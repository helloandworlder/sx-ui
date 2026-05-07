#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/../../.."
ROOT=$(pwd)
STAGE=/tmp/sx-e2e
rm -rf "$STAGE"; mkdir -p "$STAGE/bin"
HOST_ARCH="$(uname -m | sed 's/aarch64/arm64/;s/arm64/arm64/;s/x86_64/amd64/')"

cleanup() {
    docker rm -f sx-e2e-server sx-e2e-runner 2>/dev/null || true
    docker network rm sx-e2e 2>/dev/null || true
}
trap cleanup EXIT

echo "=== 1. Build linux/${HOST_ARCH} binaries on host ==="

# xray: pure Go, no CGO needed
cd "$ROOT/sx-core"
GOOS=linux GOARCH="$HOST_ARCH" CGO_ENABLED=0 go build -ldflags "-w -s" -o "$STAGE/bin/xray-linux-${HOST_ARCH}" ./main
echo "  xray OK"

GEO_SRC="$ROOT/sx-ui/build/bin"
if [ ! -f "$GEO_SRC/geoip.dat" ] || [ ! -f "$GEO_SRC/geosite.dat" ]; then
    GEO_SRC="$ROOT/sx-ui/dist/sx-ui/bin"
fi
cp "$GEO_SRC/geoip.dat" "$STAGE/bin/geoip.dat"
cp "$GEO_SRC/geosite.dat" "$STAGE/bin/geosite.dat"
echo "  geo data OK ($GEO_SRC)"

# sx-ui: needs CGO for sqlite — compile inside Docker with host module cache
echo "  Building sx-ui inside Docker (for CGO/sqlite)..."
docker run --rm \
    --platform "linux/${HOST_ARCH}" \
    -v "$ROOT/sx-core:/workspace/sx-core:ro" \
    -v "$ROOT/sx-ui:/workspace/sx-ui:ro" \
    -v "$STAGE:/out" \
    -v "$(go env GOMODCACHE):/go/pkg/mod" \
    -w /workspace/sx-ui \
    -e GOOS=linux \
    -e GOARCH="$HOST_ARCH" \
    -e CGO_ENABLED=1 \
    golang:1.26-bookworm \
    go build -ldflags "-w -s" -o /out/x-ui main.go
chmod +x "$STAGE/x-ui"
echo "  x-ui OK ($(ls -lh "$STAGE/x-ui" | awk '{print $5}'))"

echo "=== 2. Build Docker image (no network needed) ==="
cat > "$STAGE/Dockerfile" <<'DOCK'
FROM golang:1.26-bookworm
WORKDIR /app
COPY x-ui /app/x-ui
COPY bin/ /app/bin/
RUN apt-get update \
    && apt-get install -y --no-install-recommends curl python3 ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && chmod +x /app/x-ui /app/bin/* \
    && mkdir -p /etc/x-ui /var/log/x-ui
ENV XUI_MAIN_FOLDER=/app
CMD ["/bin/bash", "-lc", "/app/x-ui setting -port 2053 -listenIP 0.0.0.0 -xrayApiPort 62789 -xrayMetricsPort 62790 && exec /app/x-ui run"]
DOCK

docker build -t sx-ui-e2e "$STAGE" && echo "  image OK"

echo "=== 3. Start server ==="
docker rm -f sx-e2e-server 2>/dev/null || true
docker network rm sx-e2e 2>/dev/null || true
docker network create sx-e2e
docker run -d --name sx-e2e-server --network sx-e2e sx-ui-e2e

echo "  Waiting for panel..."
READY=0
for i in $(seq 1 30); do
    docker exec sx-e2e-server curl -sf http://127.0.0.1:2053/ >/dev/null 2>&1 && READY=1 && echo "  Panel ready (${i}s)" && break
    sleep 1
done
if [ "$READY" != "1" ]; then
    echo "Panel did not become ready"
    docker logs sx-e2e-server
    exit 1
fi

echo "=== 4. Run tests ==="
docker rm -f sx-e2e-runner 2>/dev/null || true
RC=0
docker run --rm --network sx-e2e \
    --name sx-e2e-runner \
    -v "$ROOT/sx-ui/test/e2e/run_tests.sh:/tests/run_tests.sh:ro" \
    -e PANEL_URL=http://sx-e2e-server:2053 \
    -e E2E_RUNNER_HOST=sx-e2e-runner \
    -e "LOCAL_SOCKS_OUT=sx-e2e-runner:19888:e2euser:e2epass" \
    sx-ui-e2e bash /tests/run_tests.sh || RC=$?

echo "=== Cleanup ==="
cleanup
exit $RC
