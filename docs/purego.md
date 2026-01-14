# Notes on the PureGo Library

Pure Go Version: [68d977deec7](https://github.com/ebitengine/purego/tree/68d977deec745157f1ff09e576819db62cdf8dcf)

## 1. How PureGo Works Internally

### Core Mechanism

PureGo enables calling C functions from Go without CGO by leveraging Go's internal runtime mechanisms. The core mechanism involves:

**1.1 Using `runtime.cgocall` via `go:linkname`**

At `./purego/go_runtime.go:12-13`:
```go
//go:linkname runtime_cgocall runtime.cgocall
func runtime_cgocall(fn uintptr, arg unsafe.Pointer) int32
```

This links to Go's internal `runtime.cgocall` function, which handles the transition from Go to C code, including setting up the proper goroutine state and stack.

**1.2 Assembly Trampolines**

PureGo uses architecture-specific assembly to:
- Set up registers according to the C calling convention (SysV ABI on Unix, Windows x64 ABI on Windows)
- Pass arguments in the correct registers (integer and float registers)
- Handle stack arguments for calls with many parameters

For example, at `./purego/sys_amd64.s:41-98`, the `syscall15X` function:
- Loads float arguments into XMM0-XMM7
- Loads integer arguments into RDI, RSI, RDX, RCX, R8, R9 (SysV ABI)
- Pushes additional arguments onto the stack
- Calls the C function pointer
- Captures return values from RAX/RDX and XMM0/XMM1

**1.3 Dynamic Library Loading**

At `./purego/dlfcn.go:80-99`, PureGo uses `go:linkname` to access dlopen/dlsym/dlclose directly:
```go
//go:linkname dlopen dlopen
var dlopen uint8
var dlopenABI0 = uintptr(unsafe.Pointer(&dlopen))
```

**1.4 RegisterFunc Reflection**

At `./purego/func.go:122-400`, `RegisterFunc` uses reflection to:
- Validate the function signature
- Create a `reflect.MakeFunc` closure that marshals Go values to C calling convention
- Handle type conversions (strings to char*, slices to void*, etc.)

**1.5 Callback Support**

At `./purego/syscall_sysv.go:31-48`, `NewCallback` creates C-callable function pointers:
- Pre-generated assembly stubs in `zcallback_*.s` provide entry points
- Each callback entry jumps to `callbackasm1`, which invokes `callbackWrap`
- `callbackWrap` unpacks arguments and calls the Go function via reflection

---

## 2. Platform Support

### Tier 1 Platforms (Full Support)
From `./purego/README.md:29-37`:
| OS | Architectures |
|-----|---------------|
| Android | amd64, arm64 |
| iOS | amd64, arm64 |
| Linux | amd64, arm64 |
| macOS | amd64, arm64 |
| Windows | amd64, arm64 |

### Tier 2 Platforms (Best-Effort)
From `./purego/README.md:39-48`:
| OS | Architectures | Notes |
|-----|---------------|-------|
| Android | 386, arm | - |
| FreeBSD | amd64, arm64 | - |
| Linux | 386, arm, loong64 | - |
| Windows | 386, arm | Only `SyscallN` and `NewCallback` |

### Platform Differences

**1. Build Constraints Analysis**

From `./purego/syscall_sysv.go:4`:
```go
//go:build darwin || freebsd || (linux && (amd64 || arm64 || loong64)) || netbsd
```
The pure-Go syscall implementation only works on these platforms.

**2. Windows Uses syscall.Syscall15**

At `./purego/syscall_windows.go:13-16`:
```go
func syscall_syscall15X(...) (r1, r2, err uintptr) {
    r1, r2, errno := syscall.Syscall15(fn, 15, ...)
    return r1, r2, uintptr(errno)
}
```
Windows piggybacks on Go's existing `syscall.Syscall15`.

**3. CGO Fallback for Unsupported Platforms**

At `./purego/internal/cgo/syscall_cgo_unix.go:5`:
```go
//go:build freebsd || (linux && !(arm64 || amd64 || loong64)) || netbsd
```
Platforms without native assembly use CGO-based C code for syscalls.

**4. Float Register Handling Differences**

From `./purego/syscall.go:33-38`:
```go
// NOTE: SyscallN does not properly call functions that have both integer and float parameters.
// On amd64, if there are more than 8 floats the 9th and so on will be placed incorrectly on the stack.
```

**5. Android Requires CGO**

At `./purego/dlfcn_android.go`, Android must use CGO for dlopen.

**6. iOS Requires CGO**

At `./purego/is_ios.go:8-13`:
```go
// purego does not support this mode yet.
// the fix is to set CGO_ENABLED=1
var _ = _PUREGO_REQUIRES_CGO_ON_IOS
```

---

## 3. Limitations and Failure Cases (CRITICAL)

### 3.1 Maximum Arguments Limit

At `./purego/syscall.go:16`:
```go
const maxArgs = 15
```
At `./purego/syscall.go:49-51`:
```go
if len(args) > maxArgs {
    panic("purego: too many arguments to SyscallN")
}
```

### 3.2 Single Return Value Only

At `./purego/func.go:128-130`:
```go
if ty.NumOut() > 1 {
    panic("purego: function can only return zero or one values")
}
```

### 3.3 Callback Limitations

At `./purego/syscall_sysv.go:53`:
```go
const maxCB = 2000
```
At `./purego/syscall_sysv.go:115-117`:
```go
if cbs.numFn >= maxCB {
    panic("purego: the maximum number of callbacks has been reached")
}
```

**Callback memory is NEVER freed** (from `./purego/syscall_sysv.go:35`):
> "Only a limited number of callbacks may be created in a single Go process, and any memory allocated for these callbacks is never released."

**Unsupported callback argument types** at `./purego/syscall_sysv.go:88-98`:
```go
case reflect.Struct:
    if i == 0 && in.AssignableTo(reflect.TypeOf(CDecl{})) {
        continue
    }
    fallthrough
case reflect.Interface, reflect.Func, reflect.Slice,
    reflect.Chan, reflect.Complex64, reflect.Complex128,
    reflect.String, reflect.Map, reflect.Invalid:
    panic("purego: unsupported argument type: " + in.Kind().String())
```

**Unsupported callback return types** at `./purego/syscall_sysv.go:101-112`:
```go
switch ty.Out(0).Kind() {
case reflect.Pointer, reflect.Int, reflect.Int8, ... reflect.UnsafePointer:
    break output
}
panic("purego: unsupported return type: " + ty.String())
```
Floats and structs CANNOT be returned from callbacks!

### 3.4 Float Support Restrictions

At `./purego/func.go:134-137`:
```go
if ty.NumOut() == 1 && (ty.Out(0).Kind() == reflect.Float32 || ty.Out(0).Kind() == reflect.Float64) &&
    runtime.GOARCH != "arm64" && runtime.GOARCH != "amd64" && runtime.GOARCH != "loong64" {
    panic("purego: float returns are not supported")
}
```

At `./purego/func.go:169-172`:
```go
const is32bit = unsafe.Sizeof(uintptr(0)) == 4
if is32bit {
    panic("purego: floats only supported on 64bit platforms")
}
```

### 3.5 Struct Support Restrictions

**Struct Arguments Only on Darwin amd64/arm64**

At `./purego/func.go:179-181`:
```go
if runtime.GOOS != "darwin" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
    panic("purego: struct arguments are only supported on darwin amd64 & arm64")
}
```

**Struct Returns Only on Darwin**

At `./purego/func.go:200-202`:
```go
if runtime.GOOS != "darwin" {
    panic("purego: struct return values only supported on darwin arm64 & amd64")
}
```

**No Padding Verification**

At `./purego/func.go:98-100`:
> "it does not support aligning fields properly. It is therefore the responsibility of the caller to ensure that all padding is added to the Go struct to match the C one."

Example from `./purego/struct_test.go:415-418`:
```go
type BoolFloat struct {
    b bool
    _ [3]byte // purego won't do padding for you so make sure it aligns properly
    f float32
}
```

**Unsupported Struct Field Types**

At `./purego/func.go:477-483`:
```go
switch f.Kind() {
case reflect.Int, reflect.Int8, ... reflect.Float32:
default:
    panic(fmt.Sprintf("purego: struct field type %s is not supported", f))
}
```
Strings, slices, maps, channels, interfaces in struct fields are NOT supported.

### 3.6 32-bit Platform Limitations

At `./purego/struct_386.go:8-14`:
```go
func addStruct(...) []any {
    panic("purego: struct arguments are not supported")
}
func getStruct(...) (v reflect.Value) {
    panic("purego: struct returns are not supported")
}
```

At `./purego/struct_arm.go:8-14`:
Same panics for 32-bit ARM.

### 3.7 Mixed Integer/Float Calls

At `./purego/syscall.go:33-34`:
```go
// NOTE: SyscallN does not properly call functions that have both integer and float parameters.
```

### 3.8 CDecl Positioning

At `./purego/func.go:156-158`:
```go
if j != 0 {
    panic("purego: CDecl must be the first argument")
}
```

### 3.9 Too Many Stack Arguments

At `./purego/func.go:217-222`:
```go
if stack > sizeOfStack {
    panic("purego: too many stack arguments")
}
```

### 3.10 Nil Function Pointer

At `./purego/func.go:131-133`:
```go
if cfn == 0 {
    panic("purego: cfn is nil")
}
```

### 3.11 Function Type Validation

At `./purego/func.go:125-127`:
```go
if ty.Kind() != reflect.Func {
    panic("purego: fptr must be a function pointer")
}
```

### 3.12 Unsupported Argument Kinds

At `./purego/func.go:195-197`:
```go
default:
    panic("purego: unsupported kind " + arg.Kind().String())
```
Channels, maps, complex numbers, and interfaces are NOT supported as arguments.

### 3.13 Variadic Expansion Position

At `./purego/func.go:299-301`:
```go
if i != len(args)-1 {
    panic("purego: can only expand last parameter")
}
```

### 3.14 Pointer in Struct (AMD64 Register Allocation)

At `./purego/struct_amd64.go:172-174`:
```go
case reflect.Pointer:
    ok = false
    return
```
Pointers in structs cause fallback to stack allocation on amd64.

### 3.15 fakecgo Limitations

From `./purego/internal/fakecgo/doc.go:17-20`:
```go
// Currently, fakecgo only supports macOS on amd64 & arm64. It also cannot
// be used with -buildmode=c-archive because that requires special initialization
// that fakecgo does not implement at the moment.
```

---

## 4. What PureGo CAN'T Do That CGO Can

| Feature | CGO | PureGo |
|---------|-----|--------|
| Multiple return values | Yes | No (max 1) |
| Max arguments | Unlimited | 15 |
| Struct args (non-Darwin) | Yes | No |
| Struct returns (non-Darwin) | Yes | No |
| Float args on 32-bit | Yes | No |
| Callbacks returning floats | Yes | No |
| Callbacks returning structs | Yes | No |
| Callbacks with string args | Yes | No |
| Callbacks with slice args | Yes | No |
| Variadic C functions | Yes | Partial (no true variadic) |
| long double support | Yes | No |
| Complex number types | Yes | No |
| Union types | Yes | No |
| Bitfields | Yes | No |
| Static linking | Yes | Possible |
| -buildmode=c-archive | Yes | Not with fakecgo |
| Unlimited callbacks | Yes | Max 2000 |
| Callback cleanup | Yes | Never freed |
| iOS without CGO | Yes | Not supported |
| Android without CGO | N/A | Not supported |
| Mixed int/float in SyscallN | Yes | Broken |

---

## 5. Memory Management Concerns

### 5.1 String Handling

From `./purego/func.go:77-94` and `./purego/internal/strings/strings.go:15-23`:

**When passing strings to C:**
- If string ends with `\x00`, the original pointer is used (caller must keep alive)
- Otherwise, a copy is made that is only valid for the duration of the call
- If C holds a reference beyond the call, undefined behavior occurs

**When receiving strings from C:**
- A new Go string is allocated and data is copied
- The C string is NOT freed by purego

### 5.2 Pointer Lifetime

From `./purego/func.go:77-82`:
> "In general it is not possible for purego to guarantee the lifetimes of objects returned or received from calling functions using RegisterFunc. For arguments to a C function it is important that the C function doesn't hold onto a reference to Go memory."

### 5.3 Slice Handling

At `./purego/func.go:412-414`:
```go
case reflect.Ptr, reflect.UnsafePointer, reflect.Slice:
    addInt(v.Pointer())
```
Slices pass the data pointer only - no length/capacity information. The backing array must be kept alive.

### 5.4 Callback Memory Leak

From `./purego/syscall_sysv.go:35`:
> "any memory allocated for these callbacks is never released"

Each call to `NewCallback` permanently allocates an entry in a fixed-size array.

### 5.5 Struct Stack Allocation

At `./purego/struct_arm64.go:215-223`:
```go
func placeStack(v reflect.Value, keepAlive []any, addInt func(uintptr)) []any {
    ptrStruct := reflect.New(v.Type())
    ptrStruct.Elem().Set(v)
    ptr := ptrStruct.Elem().Addr().UnsafePointer()
    keepAlive = append(keepAlive, ptr)
    addInt(uintptr(ptr))
    return keepAlive
}
```
Large structs are heap-allocated and kept alive via the keepAlive slice.

---

## 6. Performance Considerations

### 6.1 Reflection Overhead

Every call through `RegisterFunc` involves:
- `reflect.MakeFunc` closure execution
- Argument marshalling via reflection
- Type checking at runtime
- keepAlive slice allocations

### 6.2 sync.Pool Usage

At `./purego/func.go:25-27`:
```go
var thePool = sync.Pool{New: func() any {
    return new(syscall15Args)
}}
```
This reduces allocation pressure but adds pool overhead.

### 6.3 String Conversion

At `./purego/internal/strings/strings.go:15-23`:
```go
func CString(name string) *byte {
    if hasSuffix(name, "\x00") {
        return &(*(*[]byte)(unsafe.Pointer(&name)))[0]
    }
    b := make([]byte, len(name)+1)
    copy(b, name)
    return &b[0]
}
```
Non-null-terminated strings require allocation and copy.

### 6.4 Callback Overhead

Each callback invocation:
1. Goes through assembly trampoline
2. Calls `cgocallback`
3. Uses mutex to lookup function: `./purego/syscall_sysv.go:142-144`
4. Reflects to unpack arguments and call Go function

### 6.5 CGO vs PureGo Tradeoffs

**PureGo Advantages:**
- No C compiler needed
- Faster build times
- Smaller binaries (no C wrapper functions)
- Works with CGO_ENABLED=0

**CGO Advantages:**
- Better performance for hot paths (no reflection)
- Full calling convention support
- No callback limits
- Better memory management

### 6.6 Direct SyscallN vs RegisterFunc

`SyscallN` is faster than `RegisterFunc` when:
- You don't need type conversion
- You're doing low-level system calls
- Performance is critical

However, `SyscallN` has the integer/float mixing bug.

---

## Summary of Critical Points

1. **Architecture-specific bugs**: Mixed int/float parameters in `SyscallN` are broken
2. **Platform restrictions**: Struct handling only works on Darwin amd64/arm64
3. **Fixed limits**: 15 max arguments, 2000 max callbacks (never freed)
4. **No padding**: Struct field alignment must be manually handled
5. **Callback restrictions**: Cannot return floats/structs, cannot accept strings/slices
6. **Memory leaks**: Callbacks are never freed
7. **String lifetime**: Careful management required for C strings
8. **iOS/Android**: Require CGO_ENABLED=1
9. **32-bit**: No float support, no struct support
10. **TODOs indicate incomplete features**: Callback tight packing, try/catch support
