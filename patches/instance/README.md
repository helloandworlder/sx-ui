# patches/instance — multi-instance install/runtime extraction plan

## Why

`install.sh`, `update.sh`, and `x-ui.sh` carry ~135 lines of multi-instance
logic (`XUI_INSTANCE`, `xui_instance`, `sx-ui-${xui_instance}.service`, per-
instance dirs `/etc/sx-ui/<inst>` etc.). Every upstream 3x-ui rebase
generates massive merge conflicts in these scripts because the multi-
instance lines are interleaved with upstream's single-instance lines.

Goal: refactor so the upstream scripts stay close-to-pristine and the
sx-ui-only behavior lives in sourced helper files.

## Target layout

```
sx-ui/
├── install.sh             # source patches/instance/sxui-instance.sh
├── update.sh              # source patches/instance/sxui-instance.sh
├── x-ui.sh                # source patches/instance/sxui-instance.sh
└── patches/instance/
    ├── README.md          # this file
    ├── sxui-instance.sh   # all helpers: prompt_instance_name, apply_instance_paths,
    │                      # ensure_isolated_instance_layout, instance_systemd_unit,
    │                      # legacy_x_ui_migration_guard, etc.
    ├── 0001-install-instance-flag.patch   # minimal hook into install.sh
    ├── 0002-update-instance-flag.patch
    └── 0003-x-ui-sh-instance-menu.patch
```

## Migration steps (deferred to future session)

1. Extract from `install.sh`:
   - `prompt_instance_name()`
   - `apply_instance_paths()`
   - `ensure_isolated_instance_layout()`
   - `--instance|-i` arg parsing
   - `XUI_INSTANCE` env propagation
   into `sxui-instance.sh` as functions, then replace inline blocks with `source patches/instance/sxui-instance.sh`.

2. Same for `update.sh` (per-instance update loop, instance discovery via
   `/etc/sx-ui/*/x-ui.db` glob).

3. Same for `x-ui.sh` (instance picker menu, `sx-ui@<instance>.service`
   start/stop/restart wrappers).

4. Verify via `test/install-instance-isolation.sh` — must still pass after
   refactor.

5. Run `git diff --stat HEAD~ HEAD` against an upstream rebase to confirm
   the script-level conflict surface drops by ~80%.

## Constraint

Do NOT change runtime behavior during the refactor. The first commit must
be a pure refactor that produces byte-identical output for `--help` and
default install. Behavior changes happen in follow-up commits.

## Status

- [ ] Step 1 (install.sh extraction)
- [ ] Step 2 (update.sh extraction)
- [ ] Step 3 (x-ui.sh extraction)
- [ ] Step 4 (isolation test passes)
- [ ] Step 5 (rebase conflict measurement)
