#!/bin/bash
set -euo pipefail

OWNER="${OWNER:-helloandworlder}"
SX_UI_REPO="${SX_UI_REPO:-sx-ui}"
SX_CORE_REPO="${SX_CORE_REPO:-sx-core}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SX_UI_DIR="${ROOT_DIR}/sx-ui"
SX_CORE_DIR="${ROOT_DIR}/sx-core"

require_clean_auth() {
  gh auth status >/dev/null
}

ensure_repo() {
  local repo="$1"
  if ! gh repo view "${OWNER}/${repo}" >/dev/null 2>&1; then
    gh repo create "${OWNER}/${repo}" --public
  fi
}

set_remote() {
  local dir="$1"
  local repo="$2"
  if git -C "${dir}" remote get-url origin >/dev/null 2>&1; then
    git -C "${dir}" remote set-url origin "https://github.com/${OWNER}/${repo}.git"
  else
    git -C "${dir}" remote add origin "https://github.com/${OWNER}/${repo}.git"
  fi
}

push_main() {
  local dir="$1"
  git -C "${dir}" push -u origin main
}

main() {
  require_clean_auth

  ensure_repo "${SX_CORE_REPO}"
  set_remote "${SX_CORE_DIR}" "${SX_CORE_REPO}"
  push_main "${SX_CORE_DIR}"

  git -C "${SX_UI_DIR}" submodule sync --recursive
  git -C "${SX_UI_DIR}" config submodule.third_party/xray-core.url "https://github.com/${OWNER}/${SX_CORE_REPO}.git"

  ensure_repo "${SX_UI_REPO}"
  set_remote "${SX_UI_DIR}" "${SX_UI_REPO}"
  push_main "${SX_UI_DIR}"
}

main "$@"
