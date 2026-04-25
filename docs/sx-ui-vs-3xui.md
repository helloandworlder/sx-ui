# sx-ui vs upstream 3x-ui — DIFF inventory

Last refreshed: 2026-04-26 (during 3x-ui v2.9.2 rebase)
Maintainer note: regenerate after every upstream merge.

## Baseline pin

| component | sx-ui pin | corresponding upstream |
|---|---|---|
| 3x-ui main | fork of `MHSanaei/3x-ui` v2.8.12 → rebased onto **v2.9.2** | https://github.com/MHSanaei/3x-ui |
| xray-core (submodule `third_party/xray-core`) | `helloandworlder/sx-core` HEAD → rebased onto **xtls/xray-core v1.260327.0 (=v26.3.27)** | https://github.com/xtls/Xray-core |

## What sx-ui owns that upstream 3x-ui does NOT

Each row lists files that are sx-ui-only or modified by sx-ui. Rebasing
upstream means re-applying these on top of the new base.

### ① 限速增强（Enhanced rate limiting）

Depends on **sx-core** ratelimit module — upstream xray-core has no
ratelimit gRPC service.

**sx-core side (xray-core fork, new files):**
- `app/ratelimit/limiter.go` — token bucket per user, chunked Wait
- `app/ratelimit/manager.go` — User registry, set/remove/get-speed
- `app/ratelimit/wrapper.go` · `units.go` · `config_loader.go` · `grpc_service.go`
- `app/ratelimit/command/*` — gRPC service definition (.proto + generated stubs)
- `app/dispatcher/ratelimit_writer.go` — chunked WriteMultiBuffer + ActivityUpdater
- `proxy/activity.go` — idle timeout updater for throttled connections

**sx-core side (modified):**
- `app/dispatcher/default.go` — wires ratelimit_writer into stat writer chain
- `app/commander/commander.go` — registers ratelimit gRPC service
- `app/metrics/metrics.go` — exposes ratelimit counters

**sx-ui side:**
- `xray/api.go`
  - imports `app/ratelimit/command`
  - `XrayAPI.RateLimitServiceClient`
  - `SetUserRateLimit(email, egressBps, ingressBps)` `RemoveUserRateLimit(email)` `GetUserSpeed(email)`
- `web/service/rate_limit.go` (69L) — service wrapper
- `web/service/rate_limit_sync.go` (42L) — bg sync of DB → xray-core
- `web/service/xray_dynamic.go` (269L) — `DynamicSetRateLimit` / `DynamicRemoveRateLimit` (also routing/reverse)
- `web/controller/rest_api.go` — `GET/PUT/DELETE /panel/api/inbounds/rate-limits/:email`
- `database/model/model.go`:
  - Client struct: `EgressBps int64` `IngressBps int64`
  - Account struct: `EgressBps,omitempty` / `IngressBps,omitempty` / `EgressRate{value,unit}` / `IngressRate{...}`
- `web/assets/js/model/inbound.js` — UI form for per-user rate-limit (Mbps/Kbps)
- `web/html/form/client.html` — rate-limit input fields

### ② Socks5 / HTTP / Mixed 多用户 + email/auth

Upstream's HTTP/Socks/Mixed are single-account or simplified; sx-ui makes
them first-class multi-user inbounds aligned with vmess/vless API.

**sx-core side (new):**
- `proxy/http/config_ext.go` — `AccountEmails` accessor
- `proxy/socks/config_ext.go` — same
- `proxy/http/config.proto` — `+ map<string,string> account_emails = 5;`
- `proxy/socks/config.proto` — same
- `proxy/http/config.pb.go` / `proxy/socks/config.pb.go` — regenerated

**sx-core side (modified):**
- `proxy/http/server.go` — auth check uses AccountEmails
- `proxy/socks/server.go` — same + per-account routing tag

**sx-ui side:**
- `web/controller/rest_api.go`
  - `AddClientRequest` accepts `{user, pass, email}` for HTTP/Socks/Mixed
  - email = UUIDv7 line-key for sub link generation
  - `mergeShadowsocksClients` for shadowsocks merging
  - branches on `model.HTTP, model.Socks, model.Mixed`
- `database/model/model.go`:
  - Account struct: `Method,omitempty` (cipher) / `Auth,omitempty` (hysteria2 secret)

### ③ Hysteria 2 协议

> ⚠️ **Cleanup target after v2.9.2 rebase**: check whether v2.9.2 + xray-core
> v1.260327.0 register hysteria2 natively. If yes, drop sx-ui register code.

**sx-core side (modified):**
- `main/distro/all/all.go` — registers `proxy/hysteria` + `transport/internet/hysteria`

**sx-ui side:**
- `database/model/model.go` — `Hysteria` enum (= "hysteria2")
- `web/html/form/protocol/hysteria.html` — form template
- `web/html/form/inbound.html` — protocol selector includes hysteria
- `web/assets/js/model/inbound.js` — Hysteria2 model + transport wiring

### ④ Runtime 热应用（dynamic xray apply）

Avoids xray restart on routing/reverse/ratelimit config changes.

**sx-core side (new):**
- `app/reverse/command/command.go` + `.proto` + `.pb.go` + `_grpc.pb.go`
  - gRPC service: `DynamicReplaceReverse(reverseConfig)`
**sx-core side (modified):**
- `app/reverse/reverse.go` — `+62L` to support replace-in-place
- `app/commander/commander.go` — registers reverse + ratelimit command services

**sx-ui side:**
- `web/service/xray_dynamic.go` (269L)
  - `DynamicReplaceRouting(routingJson)`
  - `DynamicReplaceReverse(reverseJson)`
  - `DynamicReplaceRateLimit(rules)`
- `web/controller/xray_setting.go` (249L) — `POST /api/setting/runtimeApply`

### ⑤ RoutingCrudService

- `web/service/routing_crud.go` (123L)
  - `SaveRuleJson(tag, ruleJson)` — upsert by tag
  - `EnsureRuleTag(tag)` — generate tag if missing
- `web/controller/xray_setting.go` — wires endpoints

### ⑥ 多实例隔离（Multi-instance）

sx-ui supports running N panels on one host with non-colliding ports +
isolated sqlite + isolated xray internal API/metrics ports.

**install/runtime side:**
- `XUI_INSTANCE` env var (default `default`)
- `web/service/setting.go` (980L; +700L vs upstream)
  - `currentXUIInstance()` — reads env
  - `instancePortOffset(instance)` — fnv32 hash modulo 10000
  - `defaultPortsForInstance` — web 10000+, sub 20000+, xray-API 30000+, metrics 40000+
  - `chooseAvailableInstancePort` — bind probe + offset retry
  - `buildInstanceScopedXrayTemplateConfig` — injects per-instance API ports into xray template
  - `defaultXrayInternalPorts` — used by xray bootstrap
- `web/service/setting_test.go` (80L) — port allocation tests
- `main.go` — propagates `XUI_INSTANCE` to db path, log path, panel name
- DB path: `/etc/sx-ui/<instance>/x-ui.db` (was `/etc/x-ui/x-ui.db`)
- Log path: `/var/log/sx-ui/<instance>/`

**install scripts:**
- `install.sh` — supports `--instance hk01`; multi-instance setup; per-instance systemd unit `sx-ui@<instance>.service`
- `update.sh` — per-instance update flow
- `x-ui.sh` — instance-aware management menu
- `test/install-instance-isolation.sh` — dual-instance smoke

> 🛠 **Cleanup target after rebase**: extract instance-related diff into
> a separate `patches/instance/*.patch` set sourced by base scripts so
> future upstream merges of install.sh/x-ui.sh don't drown in conflicts.

### ⑦ sx-ui rebrand

- `Dockerfile` — binary `sx-ui`, paths `/etc/sx-ui` `/usr/bin/sx-ui`
- `config/config.go`:
  - `getDBFolder()` returns `/etc/sx-ui` (with legacy `/etc/x-ui` migration)
  - `getLogFolder()` returns `/var/log/sx-ui`
  - go module path `github.com/helloandworlder/sx-ui/v2`
- `main.go` imports use `helloandworlder/sx-ui/v2/...`
- `x-ui.rc` `x-ui.service.{arch,debian,rhel}` — systemd unit names → `sx-ui*`
- README brand strings

### ⑧ CI / Release

- `.github/workflows/release.yml` (vs upstream): amd64-only, sha256 checksums, multi-instance bundle, artifact pattern fix `sx-ui-linux-*` to skip dockerbuild blob

### ⑨ Other small patches

- `web/service/config_seq.go` (190L) — sequence-aware config serialization for stable diffing
- `web/service/inbound.go` — multi-protocol account merge logic (referenced by rate_limit_sync)
- `web/service/ip_scanner.go` — small fix
- `web/service/tgbot.go` — small UI text patch + tests in `tgbot_test.go`
- `web/session/session.go` + `session_test.go` — added tests
- `web/service/service_test.go` (136L) — service-level integration scaffolding
- `web/locale/locale.go` — added strings (WIP, see stash)
- `database/model/model.go` `Protocol.Normalize()` — accepts "socks5" → Socks (WIP, see stash)

## Risks / things that can break on rebase

| risk | mitigation |
|---|---|
| Upstream xray-core renames `command.Service` registration API → sx-core ratelimit/reverse gRPC won't register | re-run `go build ./...` after each xray-core bump |
| Upstream 3x-ui v2.9.x adds first-class hysteria2 → sx-ui's custom registration becomes redundant | Cleanup task #6 (drop sx-ui code, use upstream) |
| Upstream 3x-ui changes inbound JSON schema → sx-ui's account_emails / EgressBps fields may not roundtrip | rerun smoke `node --test GoSea/tests/smoke/...` and m07 e2e |
| install.sh / x-ui.sh frequently change upstream → giant conflict every rebase | Cleanup task #5 (extract patches/instance/*) |

## Rebase procedure (per upstream bump)

```bash
# in sx-ui
git fetch upstream
git checkout -b rebase/3xui-vX.Y.Z
git merge vX.Y.Z   # resolve, document conflicts in docs/3xui-X.Y.Z-rebase-conflicts.md

# in third_party/xray-core (sx-core)
git fetch upstream
git checkout -b rebase/xray-vN
# port app/ratelimit/, app/reverse/command/, proxy/{http,socks}/config_ext.go,
# proxy/{http,socks}/config.proto+pb, proxy/activity.go, app/dispatcher/ratelimit_writer.go
go build ./...
go test ./app/ratelimit/... ./app/dispatcher/... ./app/reverse/...

# back in sx-ui
git add third_party/xray-core
go build ./...
go test ./web/service/... ./xray/...

# real validation: install on a node, run GoSea CLAUDE.md §7.3 A flow
```
