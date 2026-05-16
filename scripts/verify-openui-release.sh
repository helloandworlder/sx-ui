#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

bash -n install.sh
bash -n scripts/verify-openui-release.sh

python3 - <<'PY'
import pathlib
import sys

workflow = pathlib.Path(".github/workflows/release.yml")
text = workflow.read_text()
required = [
    "name: Release OpenUI",
    "  build-linux:",
    "  build-windows:",
    "uses: actions/checkout@v6",
    "uses: actions/setup-go@v6",
    "uses: actions/setup-node@v6",
    "uses: actions/upload-artifact@v7",
    "uses: svenstaro/upload-release-action@v2",
]
missing = [item for item in required if item not in text]
if missing:
    sys.exit("release workflow missing required text: " + ", ".join(missing))
PY

required_files=(
  "open-ui.service.debian"
  "open-ui.service.arch"
  "open-ui.service.rhel"
  "open-ui.rc"
)

for file in "${required_files[@]}"; do
  test -f "${file}"
done

grep -q "open-ui-linux-\${{ matrix.platform }}.tar.gz" .github/workflows/release.yml
grep -q "open-ui-windows-amd64.zip" .github/workflows/release.yml
grep -q "open-ui/" .github/workflows/release.yml
grep -q "OPENUI_DB_FOLDER=/etc/open-ui" open-ui.service.debian
grep -q "ExecStart=/usr/local/open-ui/open-ui" open-ui.service.debian
grep -q "OPENUI_REPO" install.sh
grep -q "/usr/local/open-ui" install.sh
grep -q "/etc/open-ui" install.sh
grep -q "/usr/bin/open-ui" install.sh

if grep -q "MHSanaei/3x-ui" install.sh .github/workflows/release.yml; then
  echo "install/release files must not fetch upstream 3x-ui assets as the panel package" >&2
  exit 1
fi

echo "OpenUI release files verified."
