#!/usr/bin/env bash
set -euo pipefail

red='\033[0;31m'
green='\033[0;32m'
yellow='\033[0;33m'
plain='\033[0m'

OPENUI_REPO="${OPENUI_REPO:-helloandworlder/sx-ui}"
OPENUI_INSTALL_DIR="${OPENUI_INSTALL_DIR:-/usr/local/open-ui}"
OPENUI_CLI="${OPENUI_CLI:-/usr/bin/open-ui}"
OPENUI_DATA_DIR="${OPENUI_DATA_DIR:-/etc/open-ui}"
OPENUI_LOG_DIR="${OPENUI_LOG_DIR:-/var/log/open-ui}"
OPENUI_SERVICE_DIR="${OPENUI_SERVICE_DIR:-/etc/systemd/system}"
OPENUI_SERVICE_NAME="${OPENUI_SERVICE_NAME:-open-ui}"
OPENUI_ENV_FILE="${OPENUI_ENV_FILE:-/etc/default/open-ui}"
OPENUI_VERSION="${1:-${OPENUI_VERSION:-latest}}"

[[ ${EUID} -ne 0 ]] && echo -e "${red}Fatal error:${plain} Please run this script as root." && exit 1

if [[ -f /etc/os-release ]]; then
    # shellcheck disable=SC1091
    source /etc/os-release
    release="${ID}"
else
    echo -e "${red}Fatal error:${plain} Failed to detect Linux distribution." >&2
    exit 1
fi

arch() {
    case "$(uname -m)" in
        x86_64 | x64 | amd64) echo 'amd64' ;;
        i*86 | x86) echo '386' ;;
        armv8* | armv8 | arm64 | aarch64) echo 'arm64' ;;
        armv7* | armv7 | arm) echo 'armv7' ;;
        armv6* | armv6) echo 'armv6' ;;
        armv5* | armv5) echo 'armv5' ;;
        s390x) echo 's390x' ;;
        *) echo -e "${red}Unsupported CPU architecture: $(uname -m)${plain}" && exit 1 ;;
    esac
}

install_base() {
    case "${release}" in
        ubuntu | debian | armbian)
            apt-get update
            apt-get install -y -q curl tar ca-certificates
            ;;
        fedora | amzn | virtuozzo | rhel | almalinux | rocky | ol)
            dnf install -y -q curl tar ca-certificates
            ;;
        centos)
            if [[ "${VERSION_ID:-}" =~ ^7 ]]; then
                yum install -y curl tar ca-certificates
            else
                dnf install -y -q curl tar ca-certificates
            fi
            ;;
        arch | manjaro | parch)
            pacman -Sy --noconfirm curl tar ca-certificates
            ;;
        opensuse-tumbleweed | opensuse-leap)
            zypper refresh
            zypper -q install -y curl tar ca-certificates
            ;;
        alpine)
            apk update
            apk add curl tar ca-certificates openrc
            ;;
        *)
            apt-get update
            apt-get install -y -q curl tar ca-certificates
            ;;
    esac
}

latest_version() {
    curl -fsSL "https://api.github.com/repos/${OPENUI_REPO}/releases/latest" \
        | sed -nE 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/p' \
        | head -n1
}

download_release() {
    local version="$1"
    local platform
    platform="$(arch)"
    local asset="open-ui-linux-${platform}.tar.gz"
    local url="https://github.com/${OPENUI_REPO}/releases/download/${version}/${asset}"

    echo -e "${green}Downloading OpenUI ${version} for linux-${platform}...${plain}"
    curl -fL --retry 3 --retry-delay 2 -o "/tmp/${asset}" "${url}"
    echo "/tmp/${asset}"
}

stop_existing() {
    if [[ "${release}" == "alpine" ]]; then
        rc-service "${OPENUI_SERVICE_NAME}" stop >/dev/null 2>&1 || true
    else
        systemctl stop "${OPENUI_SERVICE_NAME}" >/dev/null 2>&1 || true
    fi
}

install_files() {
    local archive="$1"
    local tmpdir
    tmpdir="$(mktemp -d)"
    trap 'rm -rf "${tmpdir}"' RETURN

    tar -zxf "${archive}" -C "${tmpdir}"
    if [[ ! -d "${tmpdir}/open-ui" ]]; then
        echo -e "${red}Fatal error:${plain} Release archive does not contain open-ui/." >&2
        exit 1
    fi

    mkdir -p "${OPENUI_INSTALL_DIR}" "${OPENUI_DATA_DIR}" "${OPENUI_LOG_DIR}"
    cp -a "${tmpdir}/open-ui/." "${OPENUI_INSTALL_DIR}/"

    if [[ -f "${OPENUI_INSTALL_DIR}/open-ui.sh" ]]; then
        install -m 0755 "${OPENUI_INSTALL_DIR}/open-ui.sh" "${OPENUI_CLI}"
    else
        cat > "${OPENUI_CLI}" <<'EOF'
#!/usr/bin/env bash
exec /usr/local/open-ui/open-ui "$@"
EOF
        chmod 0755 "${OPENUI_CLI}"
    fi

    chmod 0755 "${OPENUI_INSTALL_DIR}/open-ui"
    find "${OPENUI_INSTALL_DIR}/bin" -type f -name 'xray-*' -exec chmod 0755 {} \; 2>/dev/null || true
}

install_service() {
    if [[ "${release}" == "alpine" ]]; then
        if [[ -f "${OPENUI_INSTALL_DIR}/open-ui.rc" ]]; then
            install -m 0755 "${OPENUI_INSTALL_DIR}/open-ui.rc" "/etc/init.d/${OPENUI_SERVICE_NAME}"
        else
            echo -e "${red}Fatal error:${plain} open-ui.rc not found in release archive." >&2
            exit 1
        fi
        rc-update add "${OPENUI_SERVICE_NAME}" default
        rc-service "${OPENUI_SERVICE_NAME}" start
        return
    fi

    mkdir -p "$(dirname "${OPENUI_ENV_FILE}")"
    cat > "${OPENUI_ENV_FILE}" <<EOF
OPENUI_DB_FOLDER=${OPENUI_DATA_DIR}
OPENUI_LOG_FOLDER=${OPENUI_LOG_DIR}
OPENUI_BIN_FOLDER=${OPENUI_INSTALL_DIR}/bin
XRAY_VMESS_AEAD_FORCED=false
EOF

    local service_source=""
    case "${release}" in
        arch | manjaro | parch) service_source="${OPENUI_INSTALL_DIR}/open-ui.service.arch" ;;
        fedora | amzn | virtuozzo | rhel | almalinux | rocky | ol | centos) service_source="${OPENUI_INSTALL_DIR}/open-ui.service.rhel" ;;
        *) service_source="${OPENUI_INSTALL_DIR}/open-ui.service.debian" ;;
    esac

    if [[ ! -f "${service_source}" ]]; then
        echo -e "${red}Fatal error:${plain} $(basename "${service_source}") not found in release archive." >&2
        exit 1
    fi

    install -m 0644 "${service_source}" "${OPENUI_SERVICE_DIR}/${OPENUI_SERVICE_NAME}.service"
    systemctl daemon-reload
    systemctl enable "${OPENUI_SERVICE_NAME}"
    systemctl start "${OPENUI_SERVICE_NAME}"
}

main() {
    echo -e "${green}Installing OpenUI...${plain}"
    echo "Repository: ${OPENUI_REPO}"
    echo "Detected OS: ${release}"
    echo "Detected arch: $(arch)"

    install_base

    local version="${OPENUI_VERSION}"
    if [[ "${version}" == "latest" ]]; then
        version="$(latest_version)"
        if [[ -z "${version}" ]]; then
            echo -e "${red}Fatal error:${plain} Failed to resolve latest OpenUI release." >&2
            exit 1
        fi
    fi

    local archive
    archive="$(download_release "${version}")"
    stop_existing
    install_files "${archive}"
    install_service
    rm -f "${archive}"

    echo -e "${green}OpenUI ${version} installation finished.${plain}"
    echo "CLI: ${OPENUI_CLI}"
    echo "Install dir: ${OPENUI_INSTALL_DIR}"
    echo "Data dir: ${OPENUI_DATA_DIR}"
    echo "Log dir: ${OPENUI_LOG_DIR}"
    echo "Service: ${OPENUI_SERVICE_NAME}"
}

main "$@"
