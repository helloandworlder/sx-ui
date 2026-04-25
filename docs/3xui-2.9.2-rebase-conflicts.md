# 3x-ui v2.9.2 rebase — conflict inventory

Generated: 2026-04-26
Source comparison: `MHSanaei/3x-ui v2.9.2 tarball` ↔ `helloandworlder/sx-ui main @87d5404`

## Summary

| bucket | count | resolution strategy |
|---|---:|---|
| Files only in v2.9.2 (need to import) | 9 | copy in, wire up |
| Files only in sx-ui (preserve as-is) | 16 | nothing to do; ensure rebase doesn't drop |
| Both-sides changes (real conflicts) | 129 | per-file 3-way merge — see prioritization below |

By language:
- 49 `.go` files
- 40 `.html` files
- 6 `.js` files
- 16 `.sh / .yml / .proto / .json / .env / .md` files
- 18 other (mostly README, CI yaml, locale json)

## A. Files to import from v2.9.2 (9)

| path | purpose | sx-ui action |
|---|---|---|
| `sub/subClashService.go` | Clash subscription | copy in; wire route in `sub/sub.go` |
| `web/controller/custom_geo.go` | Custom geo data upload | copy + wire in `web/web.go` |
| `web/service/custom_geo.go` + `_test.go` | service backing `custom_geo` controller | copy in |
| `web/service/nord.go` | NordVPN data integration | copy + wire |
| `web/html/form/stream/stream_hysteria.html` | **NATIVE hysteria2 form** — supersedes sx-ui's `web/html/form/protocol/hysteria.html` | copy in; **delete** sx-ui's custom hysteria.html (Cleanup #6) |
| `web/html/modals/nord_modal.html` | Nord modal | copy in if Nord wired up |
| `web/service/xray_setting_test.go` | Tests for xray_setting controller | copy in; verify against sx-ui's `xray_setting.go` (sx-ui has 249L vs upstream — may need test adjustments) |
| `windows_files/SSL` | Windows SSL bundle | copy in |

## B. sx-ui-only files (16) — must survive rebase

| path | purpose |
|---|---|
| `.golangci.yml` | sx-ui's relaxed lint config |
| `scripts/` | sx-ui release / install helpers |
| `test/` | sx-ui e2e (`install-instance-isolation.sh`) |
| `web/controller/rest_api.go` | sx-ui REST API (1330L) — see DIFF doc § ① ② |
| `web/controller/rest_api_test.go` | tests for above |
| `web/service/config_seq.go` | sequence-aware config serialization |
| `web/service/ip_scanner.go` | IP probing |
| `web/service/node_meta.go` | sx-ui node metadata |
| `web/service/outbound_crud.go` | tag-based outbound CRUD |
| `web/service/rate_limit.go` | rate-limit service (DIFF § ①) |
| `web/service/rate_limit_sync.go` | bg sync DB→xray (DIFF § ①) |
| `web/service/routing_crud.go` | tag-based routing CRUD (DIFF § ⑤) |
| `web/service/service_test.go` | integration scaffolding |
| `web/service/setting_test.go` | multi-instance port allocation tests (DIFF § ⑥) |
| `web/service/xray_dynamic.go` | hot-apply gRPC client (DIFF § ④) |
| `web/session/session_test.go` | session tests |

Action: **none** — git merge will keep these untouched as long as v2.9.2
doesn't add same-named files (verified above, no collisions).

## C. Both-sides changes — top 30 by diff size (real conflict surface)

| diff lines | path | conflict source | priority |
|---:|---|---|---|
| 1441 | `web/html/form/outbound.html` | upstream UI evolution + sx-ui rebrand | 🟡 P2 — UI |
| 1152 | `web/assets/js/model/inbound.js` | sx-ui hysteria2 + rate-limit fields + protocol normalize | 🔴 P0 — load-bearing |
| 996 | `web/html/inbounds.html` | upstream + sx-ui rate-limit columns | 🔴 P0 |
| 990 | `web/html/modals/inbound_info_modal.html` | upstream + sx-ui rate-limit display | 🟡 P1 |
| **910** | **`install.sh`** | **upstream rewrites + sx-ui multi-instance (DIFF § ⑥)** | **🔴 P0 — biggest pain point** |
| 897 | `update.sh` | same as install.sh | 🔴 P0 |
| 595 | `x-ui.sh` | same | 🔴 P0 |
| 545 | `web/service/inbound.go` | upstream + sx-ui multi-protocol account merge | 🔴 P0 |
| 478 | `web/html/form/client.html` | sx-ui rate-limit input fields | 🔴 P0 |
| 465 | `.github/workflows/release.yml` | sx-ui amd64-only + sha256 | 🟢 P3 — CI |
| **432** | **`xray/api.go`** | **sx-ui rate-limit gRPC client (DIFF § ①)** | **🔴 P0** |
| 360 | `web/html/form/stream/stream_xhttp.html` | upstream xhttp evolution | 🟡 P2 |
| 340 | `web/html/form/stream/stream_finalmask.html` | upstream | 🟢 P3 |
| 304 | `web/service/setting.go` | sx-ui multi-instance (980L) vs upstream | 🔴 P0 |
| 292 | `web/html/modals/inbound_modal.html` | upstream + sx-ui | 🟡 P1 |
| 260 | `sub/subService.go` | upstream subscription evolution | 🟡 P2 |
| 251 | `web/html/index.html` | upstream UI + sx-ui rebrand | 🟢 P3 |
| 205 | `web/html/xray.html` | sx-ui runtime-apply button (DIFF § ④) | 🔴 P1 |
| 200 | `web/service/xray.go` | upstream + sx-ui restart hook | 🔴 P0 |
| 188 | `web/html/form/stream/stream_sockopt.html` | upstream | 🟢 P3 |

(Full 129-file list at end of this doc.)

## D. Resolution plan

### Round 1 — backend Go (P0, ~16 files)

Order matters: fix interfaces first.

1. `database/model/model.go` (small): merge sx-ui's Account fields (EgressBps, IngressBps, Method, Auth) into v2.9.2 baseline
2. `xray/api.go`: re-port sx-ui's rate-limit gRPC client onto v2.9.2's API surface
3. `web/service/setting.go`: merge multi-instance into v2.9.2 setting
4. `web/service/inbound.go`: merge sx-ui's account merge into v2.9.2 inbound CRUD
5. `web/service/xray.go`: merge sx-ui's restart/reload hooks
6. `web/controller/xray_setting.go`: ensure sx-ui's runtimeApply endpoint survives
7. `main.go`: merge XUI_INSTANCE bootstrap
8. `config/config.go`: keep sx-ui paths
9. `Dockerfile`: keep sx-ui binary names

After Round 1: `go build ./...` and `go test ./web/service/... ./xray/...` must pass.

### Round 2 — install scripts (P0, 3 files)

Reference patches/instance/README.md plan. For this rebase, may need
manual merge; refactor into helpers comes later.

### Round 3 — frontend (P0/P1, ~10 files)

10. `web/assets/js/model/inbound.js` (1152 diff): merge upstream's new fields with sx-ui's hysteria2 + rate-limit
11. `web/html/form/client.html`: rate-limit + cipher fields
12. `web/html/inbounds.html`: column order
13. `web/html/modals/inbound_modal.html` + `inbound_info_modal.html`
14. `web/html/xray.html`: runtime-apply button
15. **DELETE** `web/html/form/protocol/hysteria.html` if v2.9.2's `stream_hysteria.html` covers it (Cleanup #6)

### Round 4 — secondary (P1/P2, ~20 files)

Stream form HTML, outbound HTML, locale json, README brand strings.

### Round 5 — drop (P3)

Files where we can take upstream verbatim (CI yaml — keep sx-ui's,
ignore upstream changes there).

## E. Hysteria2 cleanup (Task #6) — confirmed feasible

v2.9.2 ships:
- `web/html/form/stream/stream_hysteria.html` (native form template)

After confirming v2.9.2 + xray-core v1.260327.0 register hysteria2 inbound
natively, the following sx-ui code can be REMOVED:

- `web/html/form/protocol/hysteria.html` (custom form, supplanted)
- The hysteria2 register block in `main/distro/all/all.go` (if upstream
  xray-core registers it natively at v1.260327.0)
- The Hysteria-specific account fields in `model.go` if upstream's
  account model already covers them

⚠️ `Auth` field on Account may still be sx-ui-only — check.

## F. Full conflicting file list (all 129)

See `/tmp/3xui-v292.conflicting.txt`. Reproduce with:

```bash
diff -rq /tmp/3xui-v292/src/ . 2>&1 \
  | grep -v "^Only in.*media\|^Only in.*third_party\|^Only in.*\.git\|^Only in.*patches\|^Only in.*docs" \
  > /tmp/3xui-v292.diffq.txt
grep "^Files " /tmp/3xui-v292.diffq.txt | sed 's/^Files \/tmp\/3xui-v292\/src\/\(.*\) and \(.*\) differ/\1/'
```

## Status

- [x] Inventory generated
- [ ] Round 1 backend merge
- [ ] Round 2 install scripts merge
- [ ] Round 3 frontend merge
- [ ] Round 4 secondary
- [ ] Round 5 drop
- [ ] `go build ./...` clean on rebase branch
- [ ] `go test ./web/service/... ./xray/...` clean
- [ ] Real e2e on staging node (Stage B)
