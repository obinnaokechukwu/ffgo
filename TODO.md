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

### 3. macOS FFmpeg 7.x ABI/struct-layout Issues — Resolved (2026-02-02)

**What was happening:** GitHub Actions macOS runners (FFmpeg 7.x / avcodec 62.x / avformat 62.x) exposed places where ffgo relied on hardcoded struct offsets for fields that can shift across FFmpeg versions. This caused:
- AAC encoder setup failures (`Invalid audio sample format: -1`) when `AVCodecContext.sample_fmt` did not get set.
- Hardware decode crashes on macOS (`SIGABRT`/`SIGTRAP`) when setting `AVCodecContext.hw_device_ctx` / `hw_frames_ctx` via incorrect offsets.
- Functional test failures where duration, chapters, and programs appeared as zero because `AVFormatContext` offsets differed.

**Fix in ffgo:** added shim-backed field helpers for:
- `AVCodecContext.sample_fmt`, `hw_device_ctx`, `hw_frames_ctx`
- `AVFormatContext.duration`, programs, chapters (+ `AVProgram`/`AVChapter` accessors)

and updated Go accessors to prefer shim helpers on platforms where struct offsets are unreliable (notably macOS).

**Result:** CI (including macOS Intel + macOS ARM64) is green again.

---

### 4. Prebuilt shim availability across OS/arch — Resolved (2026-02-02)

**Goal:** library users should not need to compile `ffshim` themselves.

**Result:**
- The repo now ships prebuilt shim binaries under `shim/prebuilt/<os>-<arch>/`:
  - `linux/amd64`, `linux/arm64`
  - `darwin/amd64`, `darwin/arm64`
  - `windows/amd64`
- The `Build Shim` workflow reliably builds all of the above (including `linux/arm64` via multiarch cross-build) and packages them on every run.

## Completed

- [x] Replace LICENSE throughout history with Apache 2.0
- [x] Replace README.md throughout history with current content
- [x] Remove `.claude/` from public repo
- [x] Remove `.envrc` from public repo
- [x] Add cross-platform shim build system
- [x] Add GitHub Actions CI workflows
- [x] Ship prebuilt shims for supported platforms
