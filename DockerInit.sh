#!/bin/sh
# DockerInit.sh — sets up build/bin/ with the xray binary + geo data.
#
# IMPORTANT (sx-ui fork): we BUILD xray from the local third_party/xray-core
# submodule (helloandworlder/sx-core, which carries our ratelimit gRPC,
# reverse-command service, hysteria registration, and dispatcher
# wrap-with-rateLimitStatWriter patches). DO NOT download upstream
# xtls/xray-core release binaries — they don't have those features and
# rate limiting will silently no-op.
set -eu

ARCH_INPUT=${1:-amd64}
case "$ARCH_INPUT" in
    amd64)
        FNAME="amd64"
        GOARCH="amd64"
        ;;
    i386)
        FNAME="i386"
        GOARCH="386"
        ;;
    armv8 | arm64 | aarch64)
        FNAME="arm64"
        GOARCH="arm64"
        ;;
    armv7 | arm | arm32)
        FNAME="arm32"
        GOARCH="arm"
        ;;
    armv6)
        FNAME="armv6"
        GOARCH="arm"
        ;;
    *)
        FNAME="amd64"
        GOARCH="amd64"
        ;;
esac

REPO_ROOT="$(cd "$(dirname "$0")" && pwd)"
XRAY_SRC="${REPO_ROOT}/third_party/xray-core"
OUT_DIR="${REPO_ROOT}/build/bin"

if [ ! -d "${XRAY_SRC}/main" ]; then
    echo "ERROR: ${XRAY_SRC}/main not found." >&2
    echo "       Did you fetch the xray-core submodule? Run:" >&2
    echo "         git submodule update --init --recursive" >&2
    exit 1
fi

mkdir -p "${OUT_DIR}"

# Build xray from sx-core fork (not upstream).  CGO disabled keeps the
# binary statically linkable so it runs across Alpine/Debian/RHEL and
# survives the multi-stage Dockerfile copy into a non-build base image.
echo "Building xray-${FNAME} from sx-core fork at ${XRAY_SRC} ..."
cd "${XRAY_SRC}"
CGO_ENABLED=0 GOOS=linux GOARCH="${GOARCH}" \
    go build -trimpath \
    -ldflags="-s -w -X github.com/xtls/xray-core/core.build=sx-core-rebase" \
    -o "${OUT_DIR}/xray-linux-${FNAME}" \
    ./main
cd "${REPO_ROOT}"
chmod +x "${OUT_DIR}/xray-linux-${FNAME}"

# Geo data files — these are pure data, no fork required.
echo "Fetching geo data ..."
cd "${OUT_DIR}"
curl -sfLRO https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat
curl -sfLRO https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat
curl -sfLRo geoip_IR.dat https://github.com/chocolate4u/Iran-v2ray-rules/releases/latest/download/geoip.dat
curl -sfLRo geosite_IR.dat https://github.com/chocolate4u/Iran-v2ray-rules/releases/latest/download/geosite.dat
curl -sfLRo geoip_RU.dat https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geoip.dat
curl -sfLRo geosite_RU.dat https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geosite.dat
cd "${REPO_ROOT}"

echo "DockerInit.sh: built ${OUT_DIR}/xray-linux-${FNAME} ($(du -h "${OUT_DIR}/xray-linux-${FNAME}" | awk '{print $1}'))"
ls "${OUT_DIR}"
