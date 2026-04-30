#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL_SH="${ROOT_DIR}/install.sh"
UPDATE_SH="${ROOT_DIR}/update.sh"

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

test_legacy_takeover_uses_official_xui_layout() {
  export XUI_INSTANCE=main
  export XUI_ROOT_FOLDER=/usr/local/sx-ui
  export XUI_MAIN_FOLDER=
  export XUI_DB_FOLDER=
  export XUI_LOG_FOLDER=
  export XUI_ENV_FILE=
  export XUI_SERVICE_NAME=

  xui_instance="${XUI_INSTANCE}"
  apply_instance_paths
  apply_legacy_takeover_paths

  assert_eq "/usr/local/x-ui" "${xui_folder}" "legacy takeover should install into official x-ui runtime folder"
  assert_eq "/etc/x-ui" "${xui_db_folder}" "legacy takeover should keep the official x-ui db folder"
  assert_eq "/var/log/x-ui" "${xui_log_folder}" "legacy takeover should keep the official x-ui log folder"
  assert_eq "/etc/default/x-ui" "${xui_env_file}" "legacy takeover should keep the official x-ui env file"
  assert_eq "x-ui" "${xui_service_name}" "legacy takeover should keep the official x-ui service name"
}

test_legacy_takeover_layout_is_allowed_when_active() {
  export XUI_INSTANCE=main
  export XUI_ROOT_FOLDER=/usr/local/sx-ui
  export XUI_MAIN_FOLDER=
  export XUI_DB_FOLDER=
  export XUI_LOG_FOLDER=
  export XUI_ENV_FILE=
  export XUI_SERVICE_NAME=

  xui_instance="${XUI_INSTANCE}"
  apply_instance_paths
  apply_legacy_takeover_paths

  ensure_isolated_instance_layout >/dev/null 2>&1
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

test_update_prefers_free_xray_port_when_existing_is_busy() {
  local selected
  selected="$(
    # shellcheck source=/dev/null
    source "${UPDATE_SH}"
    is_port_in_use() {
      [[ "$1" == "39123" ]]
    }
    choose_existing_or_free_port "xrayApiPort" "39123" 39123 39125
  )"

  assert_eq "39124" "${selected}" "update should retreat from a busy existing xray api port"
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

test_main_legacy_takeover_does_not_prompt_for_instance() {
  local prompted="0"
  local captured_folder=""
  local captured_service=""

  install_base() { :; }
  require_root() { :; }
  detect_release() { :; }
  detect_legacy_xui_install() { return 0; }
  prompt_instance_name() { prompted="1"; }
  install_x-ui() {
    captured_folder="${xui_folder}"
    captured_service="${xui_service_name}"
  }

  export XUI_INSTANCE=
  export XUI_ROOT_FOLDER=/usr/local/sx-ui
  export XUI_MAIN_FOLDER=
  export XUI_DB_FOLDER=
  export XUI_LOG_FOLDER=
  export XUI_ENV_FILE=
  export XUI_SERVICE_NAME=
  install_version=""
  xui_instance=""

  main >/dev/null 2>&1

  assert_eq "0" "${prompted}" "legacy takeover should not prompt for an instance name"
  assert_eq "/usr/local/x-ui" "${captured_folder}" "legacy takeover should install into official x-ui folder"
  assert_eq "x-ui" "${captured_service}" "legacy takeover should keep official x-ui service name"
}

test_save_db_setting_updates_without_unique_constraint() {
  local tmpdir
  tmpdir="$(mktemp -d)"
  local db_path="${tmpdir}/x-ui.db"

  sqlite3 "${db_path}" "CREATE TABLE settings (id integer PRIMARY KEY AUTOINCREMENT, key text, value text);"

  xui_db_folder="${tmpdir}"
  save_db_setting "subPort" "20000"
  save_db_setting "subPort" "20001"

  local value
  value="$(sqlite3 "${db_path}" "SELECT value FROM settings WHERE key = 'subPort';")"
  local count
  count="$(sqlite3 "${db_path}" "SELECT COUNT(*) FROM settings WHERE key = 'subPort';")"

  assert_eq "20001" "${value}" "save_db_setting should persist the latest value"
  assert_eq "1" "${count}" "save_db_setting should not duplicate rows without a unique index"
}

test_configure_sxui_node_writes_to_node_meta_table() {
  local tmpdir
  tmpdir="$(mktemp -d)"
  local db_path="${tmpdir}/x-ui.db"

  sqlite3 "${db_path}" "CREATE TABLE node_meta (id integer PRIMARY KEY AUTOINCREMENT, key text, value text); CREATE UNIQUE INDEX idx_node_meta_key ON node_meta(key);"

  xui_db_folder="${tmpdir}"
  xui_instance="hk01"
  export SX_NODE_TYPE=dedicated
  export SX_API_KEY=test-api-key
  export SX_GEOIP_BLOCK_CN=false

  configure_sxui_node >/dev/null

  local api_key
  api_key="$(sqlite3 "${db_path}" "SELECT value FROM node_meta WHERE key = 'api_key';")"
  local node_type
  node_type="$(sqlite3 "${db_path}" "SELECT value FROM node_meta WHERE key = 'node_type';")"
  local geoip_block
  geoip_block="$(sqlite3 "${db_path}" "SELECT value FROM node_meta WHERE key = 'geoip_block_cn';")"

  assert_eq "test-api-key" "${api_key}" "configure_sxui_node should write api_key to node_meta"
  assert_eq "dedicated" "${node_type}" "configure_sxui_node should write node_type to node_meta"
  assert_eq "false" "${geoip_block}" "configure_sxui_node should write geoip_block_cn to node_meta"
}

test_source_is_safe_and_defaults_to_isolated_instance_layout
test_legacy_shared_runtime_layout_is_rejected
test_legacy_takeover_uses_official_xui_layout
test_legacy_takeover_layout_is_allowed_when_active
test_auto_port_selection_skips_busy_ports
test_explicit_busy_port_fails
test_update_prefers_free_xray_port_when_existing_is_busy
test_main_does_not_forward_empty_version
test_main_legacy_takeover_does_not_prompt_for_instance
test_save_db_setting_updates_without_unique_constraint
test_configure_sxui_node_writes_to_node_meta_table

echo "install-instance-isolation: ok"
