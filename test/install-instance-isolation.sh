#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL_SH="${ROOT_DIR}/install.sh"

assert_eq() {
  local expected="$1"
  local actual="$2"
  local message="$3"
  if [[ "${expected}" != "${actual}" ]]; then
    echo "ASSERT_EQ failed: ${message}" >&2
    echo "expected: ${expected}" >&2
    echo "actual:   ${actual}" >&2
    exit 1
  fi
}

assert_ne() {
  local left="$1"
  local right="$2"
  local message="$3"
  if [[ "${left}" == "${right}" ]]; then
    echo "ASSERT_NE failed: ${message}" >&2
    echo "both values: ${left}" >&2
    exit 1
  fi
}

test_source_is_safe_and_defaults_to_isolated_instance_layout() {
  export SX_UI_TEST_MODE=1
  export XUI_INSTANCE=
  export XUI_ROOT_FOLDER=/usr/local/sx-ui
  export XUI_MAIN_FOLDER=
  export XUI_DB_FOLDER=
  export XUI_LOG_FOLDER=
  export XUI_ENV_FILE=
  export XUI_SERVICE_NAME=
  export XUI_SERVICE=/etc/systemd/system

  # shellcheck source=/dev/null
  source "${INSTALL_SH}"

  xui_instance=""
  prompt_instance_name
  apply_instance_paths

  assert_eq "main" "${xui_instance}" "default instance name"
  assert_eq "/usr/local/sx-ui/main" "${xui_folder}" "default instance folder"
  assert_eq "/etc/sx-ui/main" "${xui_db_folder}" "default db folder"
  assert_eq "/var/log/sx-ui/main" "${xui_log_folder}" "default log folder"
  assert_eq "/etc/default/sx-ui-main" "${xui_env_file}" "default env file"
  assert_eq "sx-ui-main" "${xui_service_name}" "default service name"
}

test_legacy_shared_runtime_layout_is_rejected() {
  export XUI_INSTANCE=main
  export XUI_ROOT_FOLDER=/usr/local
  export XUI_MAIN_FOLDER=/usr/local/x-ui
  export XUI_DB_FOLDER=/etc/sx-ui/main
  export XUI_LOG_FOLDER=/var/log/sx-ui/main
  export XUI_ENV_FILE=/etc/default/sx-ui-main
  export XUI_SERVICE_NAME=sx-ui-main

  xui_instance="${XUI_INSTANCE}"
  apply_instance_paths

  if ensure_isolated_instance_layout >/dev/null 2>&1; then
    echo "expected legacy shared runtime layout to be rejected" >&2
    exit 1
  fi
}

test_auto_port_selection_skips_busy_ports() {
  is_port_in_use() {
    [[ "$1" == "29123" ]]
  }

  local selected
  selected="$(resolve_port_choice "subPort" "" 29123 29125)"

  assert_ne "29123" "${selected}" "auto-selected port should skip busy port"
}

test_explicit_busy_port_fails() {
  is_port_in_use() {
    [[ "$1" == "29124" ]]
  }

  if resolve_port_choice "subPort" "29124" 29123 29125 >/dev/null 2>&1; then
    echo "expected explicit busy port selection to fail" >&2
    exit 1
  fi
}

test_main_does_not_forward_empty_version() {
  local captured_call=""

  install_base() { :; }
  require_root() { :; }
  detect_release() { :; }
  ensure_isolated_instance_layout() { :; }
  install_x-ui() {
    if [[ $# -eq 0 ]]; then
      captured_call="no-args"
    else
      captured_call="$1"
    fi
  }

  export XUI_INSTANCE=main
  install_version=""
  xui_instance="${XUI_INSTANCE}"

  main --instance main >/dev/null 2>&1

  assert_eq "no-args" "${captured_call}" "main should call install_x-ui without an empty version argument"
}

test_source_is_safe_and_defaults_to_isolated_instance_layout
test_legacy_shared_runtime_layout_is_rejected
test_auto_port_selection_skips_busy_ports
test_explicit_busy_port_fails
test_main_does_not_forward_empty_version

echo "install-instance-isolation: ok"
