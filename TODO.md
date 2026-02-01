# TODO - ffgo

## Status

### 1. purego ARM64 Limitation — Mitigated (2026-02-01)

**What was happening:** some purego bindings could panic on ARM64 when the Go function signature returned an `unsafe.Pointer` (example error: `purego: unsupported kind unsafe.Pointer`).

**Fix in ffgo:** internal purego bindings that *return pointers* now return `uintptr`, and wrappers convert back to `unsafe.Pointer`. This avoids registering functions with `unsafe.Pointer` return types while preserving the public API.

**Result:**
- `swscale` tests no longer skip ARM64 for this reason.
- Public types remain `unsafe.Pointer` aliases; only the internal binding signatures changed.

**Tracking:** purego upstream: https://github.com/ebitengine/purego/issues

---

### 2. CI Lint Issues — Resolved (2026-02-01)

`golangci-lint` runs clean with the repo’s current configuration (CI uses the default golangci-lint action config).

Local verification:
- `golangci-lint run ./...`
- `go vet -unsafeptr=false ./...`

---

## Completed

- [x] Replace LICENSE throughout history with Apache 2.0
- [x] Replace README.md throughout history with current content
- [x] Remove `.claude/` from public repo
- [x] Remove `.envrc` from public repo
- [x] Add cross-platform shim build system
- [x] Add GitHub Actions CI workflows
