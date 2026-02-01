# TODO - ffgo

## Known Issues

### 1. purego ARM64 Limitation (High Priority)

**Problem:** purego doesn't support returning `unsafe.Pointer` from C functions on ARM64 architecture.

**Symptoms:**
- Tests in `swscale/swscale_test.go` panic with: `purego: unsupported kind unsafe.Pointer`
- Affects `GetContext()` and similar functions that return FFmpeg struct pointers
- Only occurs on macOS ARM64 (Apple Silicon)

**Root Cause:**
- purego's ARM64 calling convention handler (`struct_arm64.go`) doesn't support registering functions that return `unsafe.Pointer`
- This is a fundamental limitation of how purego handles ABI differences on ARM64

**Current Workaround:**
- Tests are skipped on ARM64 with `skipOnARM64(t)`

**Potential Fixes:**
1. **Upstream fix:** Wait for purego to add ARM64 support for `unsafe.Pointer` returns
2. **API redesign:** Change functions to use output parameters instead of return values
3. **Wrapper approach:** Use shim library to wrap problematic functions

**Affected Functions:**
- `swscale.GetContext()` - returns `unsafe.Pointer` (SwsContext*)
- Any other function returning FFmpeg struct pointers

**Tracking:**
- purego issue: https://github.com/ebitengine/purego/issues

---

### 2. CI Lint Issues (Medium Priority)

**Problem:** golangci-lint reports errcheck and gosimple violations.

**Remaining Issues:**
- errcheck: Some function return values not explicitly checked
- gosimple S1009: Unnecessary nil checks before len() on maps

**Action Required:**
- Run `golangci-lint run` locally and fix all reported issues
- Add `_ =` prefix for intentionally ignored errors
- Remove `if x != nil && len(x) > 0` patterns (just use `if len(x) > 0`)

---

## Completed

- [x] Replace LICENSE throughout history with Apache 2.0
- [x] Replace README.md throughout history with current content
- [x] Remove `.claude/` from public repo
- [x] Remove `.envrc` from public repo
- [x] Add cross-platform shim build system
- [x] Add GitHub Actions CI workflows
