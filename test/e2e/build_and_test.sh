#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")/../../.."
ROOT=$(pwd)
STAGE=/tmp/sx-e2e
rm -rf "$STAGE"; mkdir -p "$STAGE/bin"

echo "=== 1. Build linux/arm64 binaries on host ==="

# xray: pure Go, no CGO needed
cd "$ROOT/sx-core"
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "-w -s" -o "$STAGE/bin/xray-linux-arm64" ./main
echo "  xray OK"

# sx-ui: needs CGO for sqlite — compile inside Docker with host module cache
echo "  Building sx-ui inside Docker (for CGO/sqlite)..."
docker run --rm \
    -v "$ROOT/sx-core:/workspace/sx-core:ro" \
    -v "$ROOT/sx-ui:/workspace/sx-ui:ro" \
    -v "$STAGE:/out" \
    -v "$(go env GOMODCACHE):/go/pkg/mod" \
    -w /workspace/sx-ui \
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
RUN chmod +x /app/x-ui /app/bin/* && mkdir -p /etc/x-ui /var/log/x-ui
ENV XUI_MAIN_FOLDER=/app
CMD ["/app/x-ui"]
DOCK

docker build -t sx-ui-e2e "$STAGE" && echo "  image OK"

echo "=== 3. Start server ==="
docker rm -f sx-e2e-server 2>/dev/null || true
docker network rm sx-e2e 2>/dev/null || true
docker network create sx-e2e
docker run -d --name sx-e2e-server --network sx-e2e sx-ui-e2e

echo "  Waiting for panel..."
for i in $(seq 1 30); do
    docker exec sx-e2e-server curl -sf http://127.0.0.1:2053/ >/dev/null 2>&1 && echo "  Panel ready (${i}s)" && break
    sleep 1
done

echo "=== 4. Run tests ==="
docker run --rm --network sx-e2e \
    -v "$ROOT/sx-ui/test/e2e/run_tests.sh:/tests/run_tests.sh:ro" \
    -e PANEL_URL=http://sx-e2e-server:2053 \
    -e "SOCKS5_OUT=207.21.125.221:9878:uIVTyaTFkeA:vr0Pq08jEHBQ" \
    sx-ui-e2e bash /tests/run_tests.sh
RC=$?

echo "=== Cleanup ==="
docker rm -f sx-e2e-server 2>/dev/null
docker network rm sx-e2e 2>/dev/null
exit $RC
