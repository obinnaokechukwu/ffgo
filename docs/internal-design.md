# ffgo Internal Design Document

> **Audience**: Library implementers and contributors only.
>
> This document specifies the internal architecture, constraints, and implementation requirements for building ffgo. The rules and gotchas documented here are **design constraints that implementers must handle internally** so that users of the library never encounter them.
>
> For user-facing documentation, see [user-guide.md](user-guide.md).

**Version**: 1.0.0-draft
**Status**: Specification
**Last Updated**: 2025-01-14

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Goals and Non-Goals](#2-goals-and-non-goals)
3. [Architecture Overview](#3-architecture-overview)
4. [Platform Support Matrix](#4-platform-support-matrix)
5. [FFmpeg Library Dependencies](#5-ffmpeg-library-dependencies)
6. [The C Shim Layer](#6-the-c-shim-layer)
7. [Memory Management](#7-memory-management)
8. [Type System](#8-type-system)
9. [Error Handling](#9-error-handling)
10. [Callback System](#10-callback-system)
11. [Public API Design](#11-public-api-design)
12. [FFmpeg Version Compatibility](#12-ffmpeg-version-compatibility)
13. [Build and Distribution](#13-build-and-distribution)
14. [Testing Strategy](#14-testing-strategy)
15. [Implementation Gotchas](#15-implementation-gotchas)
16. [Performance Considerations](#16-performance-considerations)
17. [Package Structure](#17-package-structure)

---

## 1. Executive Summary

**ffgo** is a pure Go FFmpeg binding library that uses [purego](https://github.com/ebitengine/purego) to call FFmpeg's C libraries without CGO. This enables:

- Cross-compilation without a C toolchain (`GOOS=windows go build` on Linux)
- Faster build times (pure Go compilation)
- Smaller binaries (no CGO wrapper overhead)
- Simpler CI/CD pipelines (no gcc/clang required)

**Trade-offs accepted**:
- Requires a small C shim library (~100 lines) for variadic functions
- No iOS/Android support without CGO
- Logging requires the shim; cannot use `av_log()` directly
- AVRational math functions implemented in Go (trivial)

---

## 2. Goals and Non-Goals

### 2.1 Goals

| ID | Goal | Success Criteria |
|----|------|------------------|
| G1 | Pure Go builds | `CGO_ENABLED=0 go build` succeeds on all supported platforms |
| G2 | Core media operations | Decode, encode, transcode, mux, demux, scale all functional |
| G3 | Custom I/O | Users can provide custom read/write/seek callbacks |
| G4 | Cross-compilation | Build for Windows/Linux/macOS from any of those platforms |
| G5 | FFmpeg 4.x-7.x support | Works with FFmpeg major versions 4, 5, 6, and 7 |
| G6 | Idiomatic Go API | Feels like native Go, not a C API transliteration |
| G7 | Memory safety | No memory leaks in normal usage; clear ownership semantics |

### 2.2 Non-Goals

| ID | Non-Goal | Rationale |
|----|----------|-----------|
| NG1 | iOS support | purego requires `CGO_ENABLED=1` on iOS |
| NG2 | Android support | purego requires `CGO_ENABLED=1` on Android |
| NG3 | 32-bit support | purego has severe limitations (no floats, no structs) |
| NG4 | 100% FFmpeg API coverage | Focus on common workflows; exotic APIs can be added later |
| NG5 | Zero external dependencies | C shim required for variadic functions |
| NG6 | Static FFmpeg linking | Dynamic linking only; static requires CGO |

---

## 3. Architecture Overview

### 3.1 Component Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                         User Application                            │
└─────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────┐
│                         ffgo Public API                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐ │
│  │   Decoder   │  │   Encoder   │  │   Muxer     │  │   Scaler    │ │
│  └─────────────┘  └─────────────┘  └─────────────┘  └─────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      ffgo Internal Layer                            │
│  ┌─────────────────────────────────────────────────────────────────┐│
│  │                    Function Bindings                            ││
│  │  RegisterLibFunc(&avcodec_open2, lib, "avcodec_open2")          ││
│  └─────────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────────┐│
│  │                    Type Definitions                             ││
│  │  AVFrame, AVPacket, AVCodecContext (as unsafe.Pointer)          ││
│  └─────────────────────────────────────────────────────────────────┘│
│  ┌─────────────────────────────────────────────────────────────────┐│
│  │                    Memory Management                            ││
│  │  Prevent GC of Go objects while C holds references              ││
│  └─────────────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    ▼                              ▼
┌───────────────────────────────┐  ┌───────────────────────────────────┐
│          purego               │  │         libffshim.so              │
│  (CGO_ENABLED=0 C calls)      │  │  (Variadic function wrappers)     │
└───────────────────────────────┘  └───────────────────────────────────┘
                    │                              │
                    └──────────────┬───────────────┘
                                   ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      FFmpeg Shared Libraries                        │
│  libavcodec.so  libavformat.so  libavutil.so  libswscale.so  ...   │
└─────────────────────────────────────────────────────────────────────┘
```

### 3.2 Data Flow: Decode Operation

```
┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐
│  Input   │───▶│  Demux   │───▶│  Decode  │───▶│  Output  │
│  File    │    │ (Format) │    │ (Codec)  │    │  Frames  │
└──────────┘    └──────────┘    └──────────┘    └──────────┘
     │               │               │               │
     ▼               ▼               ▼               ▼
  avio_open    av_read_frame   avcodec_send    AVFrame with
  or custom    returns         _packet +       decoded pixels
  I/O callback AVPacket        avcodec_        or audio samples
                               receive_frame
```

---

## 4. Platform Support Matrix

### 4.1 Supported Platforms

| OS | Architecture | Pure Go | CGO Fallback | Notes |
|----|--------------|---------|--------------|-------|
| Linux | amd64 | ✅ Full | N/A | Primary development platform |
| Linux | arm64 | ✅ Full | N/A | Raspberry Pi 4+, AWS Graviton |
| macOS | amd64 | ✅ Full | N/A | Intel Macs |
| macOS | arm64 | ✅ Full | N/A | Apple Silicon; struct returns work |
| Windows | amd64 | ✅ Full | N/A | Windows 10+ |
| Windows | arm64 | ✅ Full | N/A | Windows on ARM |
| FreeBSD | amd64 | ✅ Full | N/A | Best-effort support |
| FreeBSD | arm64 | ✅ Full | N/A | Best-effort support |

### 4.2 Unsupported Platforms

| OS | Architecture | Reason | Alternative |
|----|--------------|--------|-------------|
| iOS | any | purego requires CGO | Use CGO-based bindings |
| Android | any | purego requires CGO | Use CGO-based bindings |
| Linux | 386 | No float/struct support | Upgrade to 64-bit |
| Linux | arm (32-bit) | No float/struct support | Upgrade to arm64 |
| Windows | 386 | Limited purego support | Upgrade to 64-bit |

### 4.3 Platform-Specific Behaviors

**macOS (Darwin) amd64/arm64**:
- Struct arguments work (passed in registers for ≤16 bytes)
- Struct returns work (via registers or R8 pointer)
- AVRational can be passed/returned by value

**Linux/Windows/FreeBSD amd64/arm64**:
- Struct arguments: **PANIC** - must use pointers
- Struct returns: **PANIC** - must use shim with pointer output
- AVRational functions must be implemented in Go or via shim

**Detection at Runtime**:
```go
// internal/platform/platform.go
const (
    SupportsStructByValue = runtime.GOOS == "darwin" &&
                            (runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64")
)
```

---

## 5. FFmpeg Library Dependencies

### 5.1 Required Libraries

| Library | Purpose | Required |
|---------|---------|----------|
| libavutil | Common utilities, AVRational, logging | Yes |
| libavcodec | Encoding/decoding | Yes |
| libavformat | Container muxing/demuxing | Yes |
| libswscale | Pixel format conversion, scaling | Optional |
| libswresample | Audio resampling | Optional |
| libavfilter | Filter graphs | Optional |

### 5.2 Library Loading Order

Libraries must be loaded in dependency order:

```go
// Correct order - dependencies first
libavutil := purego.Dlopen("libavutil.so", purego.RTLD_NOW|purego.RTLD_GLOBAL)
libavcodec := purego.Dlopen("libavcodec.so", purego.RTLD_NOW|purego.RTLD_GLOBAL)
libavformat := purego.Dlopen("libavformat.so", purego.RTLD_NOW|purego.RTLD_GLOBAL)
libswscale := purego.Dlopen("libswscale.so", purego.RTLD_NOW|purego.RTLD_GLOBAL)
```

**RTLD_GLOBAL is REQUIRED**: FFmpeg libraries have internal cross-references.

### 5.3 Library Path Resolution

| OS | Search Order |
|----|--------------|
| Linux | `LD_LIBRARY_PATH`, `/usr/lib/x86_64-linux-gnu/`, `/usr/local/lib/` |
| macOS | `DYLD_LIBRARY_PATH`, `/usr/local/lib/`, Homebrew paths |
| Windows | `PATH`, executable directory, system directories |

**Library naming by platform**:

| OS | Pattern | Example |
|----|---------|---------|
| Linux | `lib{name}.so.{major}` | `libavcodec.so.60` |
| macOS | `lib{name}.{major}.dylib` | `libavcodec.60.dylib` |
| Windows | `{name}-{major}.dll` | `avcodec-60.dll` |

**Resolution algorithm**:
```go
func findLibrary(name string) (string, error) {
    // Try versioned names first (more specific)
    for _, major := range []int{61, 60, 59, 58} {
        path := formatLibraryPath(name, major)
        if _, err := os.Stat(path); err == nil {
            return path, nil
        }
    }
    // Fall back to unversioned
    return formatLibraryPath(name, 0), nil
}
```

---

## 6. The C Shim Layer

### 6.1 Purpose

The shim provides wrappers for functionality that purego cannot handle:

1. **Variadic functions** - `av_log()`, `avio_printf()`, etc.
2. **Struct-by-value returns** - AVRational functions on non-Darwin
3. **Callbacks with string parameters** - Log callback formatting

### 6.2 Shim Specification

**File**: `shim/ffshim.c`

```c
#include <libavutil/log.h>
#include <libavutil/rational.h>
#include <libavutil/error.h>
#include <libavformat/avio.h>
#include <stdio.h>
#include <stdarg.h>
#include <string.h>

// ============================================================================
// LOGGING SUBSYSTEM
// ============================================================================

// Callback type that Go can implement (no va_list)
typedef void (*ffshim_log_callback_t)(void *avcl, int level, const char *msg);

// Global callback pointer - set by Go
static ffshim_log_callback_t g_log_callback = NULL;

// Internal callback that FFmpeg calls - formats the message then calls Go
static void internal_log_callback(void *avcl, int level, const char *fmt, va_list vl) {
    if (g_log_callback == NULL) {
        return;
    }

    char buf[4096];
    int len = vsnprintf(buf, sizeof(buf), fmt, vl);

    // Remove trailing newline if present (Go will add its own)
    if (len > 0 && buf[len-1] == '\n') {
        buf[len-1] = '\0';
    }

    g_log_callback(avcl, level, buf);
}

// Called by Go to set up logging
void ffshim_log_set_callback(ffshim_log_callback_t cb) {
    g_log_callback = cb;
    if (cb != NULL) {
        av_log_set_callback(internal_log_callback);
    } else {
        av_log_set_callback(av_log_default_callback);
    }
}

// Called by Go to set log level
void ffshim_log_set_level(int level) {
    av_log_set_level(level);
}

// Called by Go to log a pre-formatted message
void ffshim_log(void *avcl, int level, const char *msg) {
    av_log(avcl, level, "%s", msg);
}

// ============================================================================
// AVRATIONAL OPERATIONS (for non-Darwin platforms)
// ============================================================================

void ffshim_rational_mul(int a_num, int a_den, int b_num, int b_den, int *out_num, int *out_den) {
    AVRational a = {a_num, a_den};
    AVRational b = {b_num, b_den};
    AVRational result = av_mul_q(a, b);
    *out_num = result.num;
    *out_den = result.den;
}

void ffshim_rational_div(int a_num, int a_den, int b_num, int b_den, int *out_num, int *out_den) {
    AVRational a = {a_num, a_den};
    AVRational b = {b_num, b_den};
    AVRational result = av_div_q(a, b);
    *out_num = result.num;
    *out_den = result.den;
}

void ffshim_rational_add(int a_num, int a_den, int b_num, int b_den, int *out_num, int *out_den) {
    AVRational a = {a_num, a_den};
    AVRational b = {b_num, b_den};
    AVRational result = av_add_q(a, b);
    *out_num = result.num;
    *out_den = result.den;
}

void ffshim_rational_sub(int a_num, int a_den, int b_num, int b_den, int *out_num, int *out_den) {
    AVRational a = {a_num, a_den};
    AVRational b = {b_num, b_den};
    AVRational result = av_sub_q(a, b);
    *out_num = result.num;
    *out_den = result.den;
}

void ffshim_d2q(double d, int max_den, int *out_num, int *out_den) {
    AVRational result = av_d2q(d, max_den);
    *out_num = result.num;
    *out_den = result.den;
}

double ffshim_q2d(int num, int den) {
    AVRational q = {num, den};
    return av_q2d(q);
}

int ffshim_rational_cmp(int a_num, int a_den, int b_num, int b_den) {
    AVRational a = {a_num, a_den};
    AVRational b = {b_num, b_den};
    return av_cmp_q(a, b);
}

// ============================================================================
// ERROR HANDLING
// ============================================================================

// av_strerror is NOT variadic, but we provide a wrapper for consistency
int ffshim_strerror(int errnum, char *errbuf, size_t errbuf_size) {
    return av_strerror(errnum, errbuf, errbuf_size);
}

// ============================================================================
// AVIO HELPERS
// ============================================================================

// Write a string to AVIOContext (replaces avio_printf for simple cases)
void ffshim_avio_write_string(void *avio_ctx, const char *str) {
    avio_write((AVIOContext*)avio_ctx, (const unsigned char*)str, strlen(str));
}

// ============================================================================
// VERSION INFO
// ============================================================================

unsigned int ffshim_avutil_version(void) {
    return avutil_version();
}

unsigned int ffshim_avcodec_version(void) {
    return avcodec_version();
}

unsigned int ffshim_avformat_version(void) {
    return avformat_version();
}
```

### 6.3 Shim Build Instructions

**Linux**:
```bash
gcc -shared -fPIC -o libffshim.so ffshim.c \
    $(pkg-config --cflags --libs libavutil libavcodec libavformat)
```

**macOS**:
```bash
clang -shared -fPIC -o libffshim.dylib ffshim.c \
    $(pkg-config --cflags --libs libavutil libavcodec libavformat)
```

**Windows (MSYS2/MinGW)**:
```bash
gcc -shared -o ffshim.dll ffshim.c \
    -I/mingw64/include \
    -L/mingw64/lib -lavutil -lavcodec -lavformat
```

### 6.4 Shim Distribution

The shim is distributed as:
- Pre-built binaries for common platforms in GitHub releases
- Source code for users to build against their FFmpeg version
- Build script (`build_shim.sh`) that auto-detects FFmpeg location

**Shim file sizes** (approximate):
- Linux: ~15KB
- macOS: ~12KB
- Windows: ~20KB

---

## 7. Memory Management

### 7.1 Ownership Model

| Object | Created By | Freed By | Notes |
|--------|------------|----------|-------|
| AVFormatContext | `avformat_alloc_context()` | `avformat_close_input()` or `avformat_free_context()` | Never free manually with `av_free()` |
| AVCodecContext | `avcodec_alloc_context3()` | `avcodec_free_context()` | Must be freed even after close |
| AVFrame | `av_frame_alloc()` | `av_frame_free()` | Unref before free if holding data |
| AVPacket | `av_packet_alloc()` | `av_packet_free()` | Unref before free if holding data |
| AVDictionary | Various | `av_dict_free()` | Often modified by FFmpeg; check after calls |
| SwsContext | `sws_getContext()` | `sws_freeContext()` | Thread-safe after creation |

**High-level `ffgo.Frame` (safe wrapper)**

- **Borrowed frames**: Some APIs return frames owned by an internal component and reused (e.g. decoder/scaler). Borrowed frames **must not** be freed; `FrameFree` / `Frame.Free()` returns an error if attempted.
- **Owned frames**: APIs that allocate new frames return owned frames; the caller must free them.
- Use `FrameClone` (or `Frame.Clone`) to convert a borrowed frame into an owned one.

### 7.2 Go Object Lifetime Rules

**Rule 1: Keep Go objects alive while C holds references**

```go
// WRONG - buffer may be GC'd while FFmpeg uses it
func createIOContext() *AVIOContext {
    buffer := make([]byte, 32768)
    return avio_alloc_context(
        unsafe.Pointer(&buffer[0]),
        32768,
        // ...
    )
}

// CORRECT - buffer stored to prevent GC
type IOContext struct {
    ptr    *AVIOContext
    buffer []byte  // prevents GC
}

func NewIOContext() *IOContext {
    ctx := &IOContext{
        buffer: make([]byte, 32768),
    }
    ctx.ptr = avio_alloc_context(
        unsafe.Pointer(&ctx.buffer[0]),
        32768,
        // ...
    )
    return ctx
}
```

**Rule 2: Use runtime.KeepAlive for short-lived references**

```go
func DecodePacket(ctx *CodecContext, pkt *Packet) (*Frame, error) {
    ret := avcodec_send_packet(ctx.ptr, pkt.ptr)
    runtime.KeepAlive(pkt)  // Ensure pkt lives until C returns
    if ret < 0 {
        return nil, ffmpegError(ret)
    }
    // ...
}
```

**Rule 3: Never store Go pointers in C structs**

```go
// WRONG - Go pointer stored in C memory
ctx.opaque = unsafe.Pointer(&myGoStruct)

// CORRECT - Use handle system
handle := registerHandle(&myGoStruct)  // Returns uintptr ID
ctx.opaque = unsafe.Pointer(handle)

// In callback:
goStruct := lookupHandle(uintptr(opaque))
```

### 7.3 Handle System for Callbacks

```go
// internal/handles/handles.go

var (
    mu      sync.RWMutex
    handles = make(map[uintptr]interface{})
    nextID  uintptr = 1
)

// Register stores a Go object and returns a handle ID
func Register(v interface{}) uintptr {
    mu.Lock()
    defer mu.Unlock()
    id := nextID
    nextID++
    handles[id] = v
    return id
}

// Lookup retrieves a Go object by handle ID
func Lookup(id uintptr) interface{} {
    mu.RLock()
    defer mu.RUnlock()
    return handles[id]
}

// Unregister removes a handle (call when done with callback)
func Unregister(id uintptr) {
    mu.Lock()
    defer mu.Unlock()
    delete(handles, id)
}
```

### 7.4 String Handling

**Passing strings to C**:

| String State | purego Behavior | Safety |
|--------------|-----------------|--------|
| Ends with `\x00` | Uses original pointer | Caller must keep alive |
| No null terminator | Copies to temp buffer | Only valid during call |

```go
// SAFE - null terminated, kept alive
path := "/path/to/file\x00"
ret := avformat_open_input(&ctx, path, nil, nil)
runtime.KeepAlive(path)

// SAFE - purego copies automatically
path := "/path/to/file"  // No \x00
ret := avformat_open_input(&ctx, path, nil, nil)
// purego copied it; original can be GC'd
```

**Receiving strings from C**:

```go
// C returns char* - purego creates new Go string
name := avcodec_get_name(codecID)  // Returns Go string
// The C memory is NOT freed; FFmpeg owns it (static string)
```

### 7.5 Preventing Memory Leaks

**Decoder cleanup sequence**:
```go
func (d *Decoder) Close() error {
    if d.frame != nil {
        av_frame_free(&d.frame)
        d.frame = nil
    }
    if d.packet != nil {
        av_packet_free(&d.packet)
        d.packet = nil
    }
    if d.codecCtx != nil {
        avcodec_free_context(&d.codecCtx)
        d.codecCtx = nil
    }
    if d.formatCtx != nil {
        avformat_close_input(&d.formatCtx)
        d.formatCtx = nil
    }
    return nil
}
```

**Use finalizers as safety net (not primary cleanup)**:
```go
func NewDecoder() *Decoder {
    d := &Decoder{}
    runtime.SetFinalizer(d, func(d *Decoder) {
        if d.formatCtx != nil {
            // Log warning - should have been explicitly closed
            log.Println("WARNING: Decoder not explicitly closed")
            d.Close()
        }
    })
    return d
}
```

---

## 8. Type System

### 8.1 Opaque Pointer Types

All FFmpeg context types are represented as opaque pointers:

```go
// internal/types/types.go

// AVFormatContext is an opaque FFmpeg format context
type AVFormatContext = unsafe.Pointer

// AVCodecContext is an opaque FFmpeg codec context
type AVCodecContext = unsafe.Pointer

// AVFrame is an opaque FFmpeg frame
type AVFrame = unsafe.Pointer

// AVPacket is an opaque FFmpeg packet
type AVPacket = unsafe.Pointer

// AVCodec is an opaque FFmpeg codec descriptor
type AVCodec = unsafe.Pointer

// SwsContext is an opaque swscale context
type SwsContext = unsafe.Pointer

// AVDictionary is an opaque FFmpeg dictionary
type AVDictionary = unsafe.Pointer

// AVIOContext is an opaque FFmpeg I/O context
type AVIOContext = unsafe.Pointer
```

### 8.2 Value Types

**AVRational** - Implemented in pure Go:

```go
// Rational represents a rational number (fraction)
type Rational struct {
    Num int32  // Numerator
    Den int32  // Denominator
}

// NewRational creates a new Rational
func NewRational(num, den int32) Rational {
    return Rational{Num: num, Den: den}
}

// Float64 converts to float64
func (r Rational) Float64() float64 {
    if r.Den == 0 {
        return 0
    }
    return float64(r.Num) / float64(r.Den)
}

// Mul multiplies two rationals
func (r Rational) Mul(other Rational) Rational {
    // Use shim on non-Darwin, pure Go on Darwin
    if platform.SupportsStructByValue {
        return av_mul_q(r, other)
    }
    var outNum, outDen int32
    ffshim_rational_mul(r.Num, r.Den, other.Num, other.Den, &outNum, &outDen)
    return Rational{Num: outNum, Den: outDen}
}

// ... similar for Div, Add, Sub
```

### 8.3 Enum Types

```go
// PixelFormat represents FFmpeg pixel formats
type PixelFormat int32

const (
    PixelFormatNone PixelFormat = -1
    PixelFormatYUV420P PixelFormat = 0
    PixelFormatYUYV422 PixelFormat = 1
    PixelFormatRGB24 PixelFormat = 2
    PixelFormatBGR24 PixelFormat = 3
    // ... generated from FFmpeg headers
)

// CodecID represents FFmpeg codec identifiers
type CodecID int32

const (
    CodecIDNone CodecID = 0
    CodecIDH264 CodecID = 27
    CodecIDHEVC CodecID = 173
    CodecIDAV1  CodecID = 226
    // ... generated from FFmpeg headers
)

// MediaType represents stream types
type MediaType int32

const (
    MediaTypeUnknown MediaType = -1
    MediaTypeVideo   MediaType = 0
    MediaTypeAudio   MediaType = 1
    MediaTypeData    MediaType = 2
    MediaTypeSubtitle MediaType = 3
    MediaTypeAttachment MediaType = 4
)
```

### 8.4 Error Codes

```go
// Error codes (negative AVERROR values)
const (
    AVERROR_EOF            = -541478725  // AVERROR_EOF
    AVERROR_EAGAIN         = -11         // EAGAIN (try again)
    AVERROR_EINVAL         = -22         // Invalid argument
    AVERROR_ENOMEM         = -12         // Out of memory
    AVERROR_DECODER_NOT_FOUND = -1128613112
    AVERROR_ENCODER_NOT_FOUND = -1129203192
    // ... more as needed
)

// IsEOF returns true if the error indicates end of file
func IsEOF(err error) bool {
    var ffErr *FFmpegError
    if errors.As(err, &ffErr) {
        return ffErr.Code == AVERROR_EOF
    }
    return false
}

// IsAgain returns true if the error indicates to try again
func IsAgain(err error) bool {
    var ffErr *FFmpegError
    if errors.As(err, &ffErr) {
        return ffErr.Code == AVERROR_EAGAIN
    }
    return false
}
```

---

## 9. Error Handling

### 9.1 Error Type

```go
// FFmpegError represents an error from FFmpeg
type FFmpegError struct {
    Code    int32   // Raw FFmpeg error code
    Message string  // Human-readable message
    Op      string  // Operation that failed
}

func (e *FFmpegError) Error() string {
    return fmt.Sprintf("ffmpeg %s: %s (code %d)", e.Op, e.Message, e.Code)
}

// newError creates an FFmpegError from a return code
func newError(code int32, op string) error {
    if code >= 0 {
        return nil
    }

    // Get error message from FFmpeg
    buf := make([]byte, 256)
    av_strerror(code, unsafe.Pointer(&buf[0]), 256)
    msg := string(buf[:bytes.IndexByte(buf, 0)])

    return &FFmpegError{
        Code:    code,
        Message: msg,
        Op:      op,
    }
}
```

### 9.2 Error Handling Patterns

**Pattern 1: Simple return code check**
```go
func (d *Decoder) SendPacket(pkt *Packet) error {
    ret := avcodec_send_packet(d.codecCtx, pkt.ptr)
    if ret < 0 {
        return newError(ret, "avcodec_send_packet")
    }
    return nil
}
```

**Pattern 2: EAGAIN is not an error**
```go
func (d *Decoder) ReceiveFrame() (*Frame, error) {
    ret := avcodec_receive_frame(d.codecCtx, d.frame)
    if ret == AVERROR_EAGAIN || ret == AVERROR_EOF {
        return nil, nil  // No frame available, not an error
    }
    if ret < 0 {
        return newError(ret, "avcodec_receive_frame")
    }
    return d.frame, nil
}
```

**Pattern 3: Drain loop**
```go
func (d *Decoder) Drain() ([]*Frame, error) {
    // Send flush packet
    avcodec_send_packet(d.codecCtx, nil)

    var frames []*Frame
    for {
        frame, err := d.ReceiveFrame()
        if err != nil {
            return frames, err
        }
        if frame == nil {
            break  // Drained
        }
        frames = append(frames, frame)
    }
    return frames, nil
}
```

### 9.3 Panic vs Error

| Situation | Handling |
|-----------|----------|
| FFmpeg returns error code | Return `error` |
| Nil pointer passed to public API | Return `error` with clear message |
| Internal invariant violated | `panic` with descriptive message |
| Memory allocation failed | Return `error` (ErrOutOfMemory) |
| Library not loaded | `panic` at init time |

---

## 10. Callback System

### 10.1 Supported Callbacks

| Callback | purego Compatible | Notes |
|----------|-------------------|-------|
| `read_packet` | ✅ Yes | Custom I/O read |
| `write_packet` | ✅ Yes | Custom I/O write |
| `seek` | ✅ Yes | Custom I/O seek |
| `get_buffer2` | ✅ Yes | Custom frame allocation |
| `get_format` | ✅ Yes | Pixel format selection |
| `draw_horiz_band` | ✅ Yes | Progressive decoding |
| `execute` / `execute2` | ✅ Yes | Multithreading |
| `interrupt_callback` | ✅ Yes | Interrupt blocking operations |
| Log callback | ❌ No | Requires shim (va_list) |
| `io_open` | ❌ No | String parameter |

### 10.2 Callback Registration

```go
// internal/callback/callback.go

// Maximum callbacks (purego limit is 2000)
const MaxCallbacks = 1000  // Reserve some for internal use

var (
    callbackMu    sync.Mutex
    callbackCount int
)

// checkCallbackLimit panics if too many callbacks registered
func checkCallbackLimit() {
    callbackMu.Lock()
    defer callbackMu.Unlock()
    callbackCount++
    if callbackCount > MaxCallbacks {
        panic(fmt.Sprintf("ffgo: too many callbacks registered (%d > %d)",
            callbackCount, MaxCallbacks))
    }
}
```

### 10.3 Custom I/O Implementation

```go
// IOCallbacks provides custom I/O operations
type IOCallbacks struct {
    Read  func(buf []byte) (int, error)
    Write func(buf []byte) (int, error)
    Seek  func(offset int64, whence int) (int64, error)
}

// NewCustomIOContext creates an AVIOContext with custom callbacks
func NewCustomIOContext(bufferSize int, writable bool, cb *IOCallbacks) (*IOContext, error) {
    checkCallbackLimit()

    ctx := &IOContext{
        buffer:    make([]byte, bufferSize),
        callbacks: cb,
    }

    // Register handle for callback lookup
    ctx.handle = handles.Register(ctx)

    // Create C callbacks
    var readCb, writeCb uintptr

    if cb.Read != nil {
        readCb = purego.NewCallback(func(_ purego.CDecl, opaque unsafe.Pointer, buf *byte, bufSize int32) int32 {
            ctx := handles.Lookup(uintptr(opaque)).(*IOContext)
            goBuf := unsafe.Slice(buf, bufSize)
            n, err := ctx.callbacks.Read(goBuf)
            if err != nil {
                if err == io.EOF {
                    return AVERROR_EOF
                }
                return -1
            }
            return int32(n)
        })
    }

    if cb.Write != nil {
        writeCb = purego.NewCallback(func(_ purego.CDecl, opaque unsafe.Pointer, buf *byte, bufSize int32) int32 {
            ctx := handles.Lookup(uintptr(opaque)).(*IOContext)
            goBuf := unsafe.Slice(buf, bufSize)
            n, err := ctx.callbacks.Write(goBuf)
            if err != nil {
                return -1
            }
            return int32(n)
        })
    }

    // Similar for seek...

    ctx.ptr = avio_alloc_context(
        unsafe.Pointer(&ctx.buffer[0]),
        int32(bufferSize),
        boolToInt(writable),
        unsafe.Pointer(ctx.handle),
        readCb,
        writeCb,
        seekCb,
    )

    if ctx.ptr == nil {
        handles.Unregister(ctx.handle)
        return nil, errors.New("failed to create AVIOContext")
    }

    return ctx, nil
}
```

### 10.4 Interrupt Callback

```go
// InterruptFunc is called periodically during blocking operations
// Return true to abort the operation
type InterruptFunc func() bool

// SetInterruptCallback sets an interrupt callback on a format context
func (f *FormatContext) SetInterruptCallback(fn InterruptFunc) {
    checkCallbackLimit()

    f.interruptHandle = handles.Register(fn)

    cb := purego.NewCallback(func(_ purego.CDecl, opaque unsafe.Pointer) int32 {
        fn := handles.Lookup(uintptr(opaque)).(InterruptFunc)
        if fn() {
            return 1  // Abort
        }
        return 0  // Continue
    })

    // Set on format context's interrupt_callback field
    // This requires knowing the struct offset - use accessor function
    avformat_set_interrupt_callback(f.ptr, cb, unsafe.Pointer(f.interruptHandle))
}
```

---

## 11. Public API Design

### 11.1 High-Level API (Recommended)

```go
package ffgo

// Decoder decodes media files
type Decoder struct {
    formatCtx  *FormatContext
    codecCtx   *CodecContext
    videoStream int
    audioStream int
    // ...
}

// NewDecoder opens a media file for decoding
func NewDecoder(path string) (*Decoder, error)

// NewDecoderFromIO creates a decoder with custom I/O
func NewDecoderFromIO(io *IOContext, format string) (*Decoder, error)

// VideoStream returns information about the video stream
func (d *Decoder) VideoStream() *StreamInfo

// AudioStream returns information about the audio stream
func (d *Decoder) AudioStream() *StreamInfo

// ReadFrame reads the next frame (video or audio)
func (d *Decoder) ReadFrame() (*FrameWrapper, error)

// DecodeVideo/DecodeAudio decode the next frame of that media type.
// Returned ffgo.Frame values are borrowed (decoder-owned and reused).
func (d *Decoder) DecodeVideo() (Frame, error)
func (d *Decoder) DecodeAudio() (Frame, error)

// Seek seeks to a timestamp
func (d *Decoder) Seek(timestamp time.Duration) error

// Close releases all resources
func (d *Decoder) Close() error


// Encoder encodes media files
type Encoder struct {
    formatCtx *FormatContext
    codecCtx  *CodecContext
    // ...
}

// NewEncoder creates an encoder for the given output format
func NewEncoder(path string, opts *EncoderOptions) (*Encoder, error)

// WriteFrame encodes and writes a frame
func (e *Encoder) WriteFrame(frame Frame) error

// Close finalizes and closes the output
func (e *Encoder) Close() error


// Scaler converts between pixel formats and scales
type Scaler struct {
    ctx *SwsContext
    // ...
}

// NewScaler creates a new scaler
func NewScaler(srcW, srcH int, srcFmt PixelFormat,
               dstW, dstH int, dstFmt PixelFormat,
               flags ScaleFlags) (*Scaler, error)

// Scale converts a frame
// Returned ffgo.Frame is borrowed (owned by the scaler and reused).
func (s *Scaler) Scale(src Frame) (Frame, error)

// Close releases resources
func (s *Scaler) Close() error
```

### 11.2 Low-Level API (For Advanced Users)

```go
package ffgo/avutil
package ffgo/avcodec
package ffgo/avformat
package ffgo/swscale

// Direct bindings to FFmpeg functions
// Named exactly like C functions but with Go conventions

// avcodec package
func AllocContext3(codec *Codec) *CodecContext
func FreeContext(ctx **CodecContext)
func Open2(ctx *CodecContext, codec *Codec, options **Dictionary) error
func SendPacket(ctx *CodecContext, pkt *Packet) error
func ReceiveFrame(ctx *CodecContext, frame *Frame) error
func SendFrame(ctx *CodecContext, frame *Frame) error
func ReceivePacket(ctx *CodecContext, pkt *Packet) error
func FindDecoder(id CodecID) *Codec
func FindEncoder(id CodecID) *Codec

// avformat package
func OpenInput(ctx **FormatContext, url string, fmt *InputFormat, options **Dictionary) error
func CloseInput(ctx **FormatContext)
func FindStreamInfo(ctx *FormatContext, options **Dictionary) error
func ReadFrame(ctx *FormatContext, pkt *Packet) error
func SeekFrame(ctx *FormatContext, streamIndex int, timestamp int64, flags int) error

// avutil package
func FrameAlloc() Frame
func FrameFree(frame *Frame)
func FrameRef(dst, src Frame) error
func FrameUnref(frame Frame)
func FrameGetBufferErr(frame Frame, align int32) error
```

### 11.3 Options Pattern

```go
// DecoderOptions configures decoder behavior
type DecoderOptions struct {
    // Format hint (e.g., "mp4", "mkv") - optional
    Format string

    // Hardware acceleration device (e.g., "/dev/dri/renderD128")
    HWDevice string

    // Custom I/O callbacks - if nil, uses file I/O
    IO *IOCallbacks

    // Interrupt callback - called during blocking operations
    Interrupt InterruptFunc

    // FFmpeg options passed to avformat_open_input
    AVOptions map[string]string
}

// WithFormat sets the format hint
func WithFormat(format string) func(*DecoderOptions) {
    return func(o *DecoderOptions) {
        o.Format = format
    }
}

// Usage:
decoder, err := ffgo.NewDecoder("input.mp4",
    ffgo.WithFormat("mp4"),
    ffgo.WithHWDevice("/dev/dri/renderD128"),
)
```

---

## 12. FFmpeg Version Compatibility

### 12.1 Supported Versions

| FFmpeg Version | libavcodec | libavformat | libavutil | Status |
|----------------|------------|-------------|-----------|--------|
| FFmpeg 4.x | 58.x | 58.x | 56.x | Supported |
| FFmpeg 5.x | 59.x | 59.x | 57.x | Supported |
| FFmpeg 6.x | 60.x | 60.x | 58.x | Supported (Primary) |
| FFmpeg 7.x | 61.x | 61.x | 59.x | Supported |

### 12.2 Version Detection

```go
// internal/version/version.go

// FFmpegVersion represents FFmpeg library versions
type FFmpegVersion struct {
    AVUtil   uint32
    AVCodec  uint32
    AVFormat uint32
}

// DetectVersion returns the FFmpeg version
func DetectVersion() FFmpegVersion {
    return FFmpegVersion{
        AVUtil:   avutil_version(),
        AVCodec:  avcodec_version(),
        AVFormat: avformat_version(),
    }
}

// Major returns the major version number
func (v FFmpegVersion) AVUtilMajor() int {
    return int(v.AVUtil >> 16)
}

// String returns a human-readable version string
func (v FFmpegVersion) String() string {
    return fmt.Sprintf("FFmpeg (avutil=%d.%d.%d, avcodec=%d.%d.%d, avformat=%d.%d.%d)",
        v.AVUtil>>16, (v.AVUtil>>8)&0xFF, v.AVUtil&0xFF,
        v.AVCodec>>16, (v.AVCodec>>8)&0xFF, v.AVCodec&0xFF,
        v.AVFormat>>16, (v.AVFormat>>8)&0xFF, v.AVFormat&0xFF,
    )
}
```

### 12.3 API Differences Between Versions

| Feature | FFmpeg 4.x | FFmpeg 5.x+ | Handling |
|---------|------------|-------------|----------|
| `avcodec_decode_video2` | Exists | Removed | Don't use; use send/receive API |
| `av_register_all` | Required | No-op | Call but ignore result |
| Channel layout | `uint64` mask | `AVChannelLayout` struct | Abstract in Go API |
| `AVStream.codec` | Deprecated | Removed | Use `AVStream.codecpar` |

### 12.4 Shim Version Compatibility

The C shim uses only stable FFmpeg APIs that haven't changed across versions:
- `av_log` / `av_log_set_callback` - Stable since FFmpeg 0.x
- `AVRational` operations - Stable since FFmpeg 0.x
- `av_strerror` - Stable since FFmpeg 1.x

**Shim does NOT need version-specific builds** for supported FFmpeg versions.

---

## 13. Build and Distribution

### 13.1 Go Module Structure

```
github.com/obinnaokechukwu/ffgo
├── go.mod
├── go.sum
├── ffgo.go           # Main package, high-level API
├── decoder.go
├── encoder.go
├── scaler.go
├── frame.go
├── packet.go
├── rational.go
├── errors.go
├── options.go
│
├── avutil/           # Low-level avutil bindings
│   ├── avutil.go
│   ├── frame.go
│   ├── dict.go
│   └── log.go
│
├── avcodec/          # Low-level avcodec bindings
│   ├── avcodec.go
│   ├── codec.go
│   └── packet.go
│
├── avformat/         # Low-level avformat bindings
│   ├── avformat.go
│   ├── context.go
│   └── avio.go
│
├── swscale/          # Low-level swscale bindings
│   └── swscale.go
│
├── internal/
│   ├── bindings/     # purego function registrations
│   ├── handles/      # Go object handle system
│   ├── platform/     # Platform detection
│   └── version/      # FFmpeg version detection
│
├── shim/
│   ├── ffshim.c      # C shim source
│   ├── ffshim.h      # C shim header
│   ├── build.sh      # Build script
│   └── prebuilt/     # Pre-built shim binaries
│       ├── linux-amd64/
│       ├── linux-arm64/
│       ├── darwin-amd64/
│       ├── darwin-arm64/
│       └── windows-amd64/
│
├── examples/
│   ├── decode/
│   ├── encode/
│   ├── transcode/
│   └── custom-io/
│
└── testdata/
    ├── sample.mp4
    ├── sample.mkv
    └── sample.wav
```

### 13.2 Build Tags

```go
//go:build !ios && !android && (amd64 || arm64)
// +build !ios,!android
// +build amd64 arm64

package ffgo
```

### 13.3 Installation

**Prerequisites**:
```bash
# Ubuntu/Debian
sudo apt install ffmpeg libavcodec-dev libavformat-dev libavutil-dev libswscale-dev

# macOS
brew install ffmpeg

# Windows (MSYS2)
pacman -S mingw-w64-x86_64-ffmpeg
```

**Install ffgo**:
```bash
go get github.com/obinnaokechukwu/ffgo
```

**Install shim** (choose one):
```bash
# Option 1: Download pre-built shim
ffgo-install-shim

# Option 2: Build from source
cd $(go env GOMODCACHE)/github.com/obinnaokechukwu/ffgo@latest/shim
./build.sh
sudo cp libffshim.so /usr/local/lib/
```

### 13.4 Cross-Compilation

```bash
# Build for Windows from Linux
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o myapp.exe

# Build for Linux ARM64 from macOS
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o myapp
```

**Note**: The target system must have FFmpeg libraries and shim installed.

---

## 14. Testing Strategy

### 14.1 Unit Tests

```go
// rational_test.go
func TestRationalMul(t *testing.T) {
    a := Rational{Num: 1, Den: 2}
    b := Rational{Num: 3, Den: 4}
    result := a.Mul(b)

    if result.Num != 3 || result.Den != 8 {
        t.Errorf("expected 3/8, got %d/%d", result.Num, result.Den)
    }
}

func TestRationalFloat64(t *testing.T) {
    r := Rational{Num: 30000, Den: 1001}
    expected := 29.97002997  // ~29.97 fps

    if math.Abs(r.Float64()-expected) > 0.00001 {
        t.Errorf("expected %f, got %f", expected, r.Float64())
    }
}
```

### 14.2 Integration Tests

```go
// decoder_test.go
func TestDecodeVideo(t *testing.T) {
    decoder, err := NewDecoder("testdata/sample.mp4")
    if err != nil {
        t.Fatalf("NewDecoder: %v", err)
    }
    defer decoder.Close()

    // Read first frame
    frame, err := decoder.ReadFrame()
    if err != nil {
        t.Fatalf("ReadFrame: %v", err)
    }

    if frame.Width() != 1920 || frame.Height() != 1080 {
        t.Errorf("expected 1920x1080, got %dx%d", frame.Width(), frame.Height())
    }
}
```

### 14.3 Platform Tests

```go
// platform_test.go
func TestPlatformCapabilities(t *testing.T) {
    if runtime.GOOS == "darwin" {
        if !platform.SupportsStructByValue {
            t.Error("Darwin should support struct by value")
        }
    } else {
        if platform.SupportsStructByValue {
            t.Errorf("%s should not support struct by value", runtime.GOOS)
        }
    }
}
```

### 14.4 Test Files

| File | Purpose | Duration | Resolution |
|------|---------|----------|------------|
| `sample.mp4` | H.264 video, AAC audio | 5 seconds | 1920x1080 |
| `sample.mkv` | VP9 video, Opus audio | 5 seconds | 1280x720 |
| `sample.wav` | PCM audio | 5 seconds | N/A |
| `corrupt.mp4` | Intentionally corrupt | N/A | N/A |

---

## 15. Implementation Gotchas

### 15.1 Critical: Struct By Value

**Problem**: purego panics when passing/returning structs by value on non-Darwin platforms.

**Affected FFmpeg Functions**:
- `av_mul_q()`, `av_div_q()`, `av_add_q()`, `av_sub_q()` - Return AVRational
- `av_d2q()` - Returns AVRational
- `av_guess_frame_rate()` - Returns AVRational
- `av_guess_sample_aspect_ratio()` - Returns AVRational

**Solution**:
```go
func (r Rational) Mul(other Rational) Rational {
    if platform.SupportsStructByValue {
        // Darwin: call FFmpeg directly
        return av_mul_q(r, other)
    }
    // Other platforms: use shim or pure Go
    return Rational{
        Num: r.Num * other.Num,
        Den: r.Den * other.Den,
    }.Reduce()
}
```

### 15.2 Critical: String Lifetime

**Problem**: purego copies strings without null terminators, but only for the call duration.

**Affected Patterns**:
```go
// DANGEROUS - FFmpeg may hold reference
ctx.filename = path  // If ctx stores pointer, UB after return

// SAFE - FFmpeg copies internally
avformat_open_input(&ctx, path, nil, nil)  // FFmpeg strdup's the path
```

**Solution**: Always use `runtime.KeepAlive()` after FFmpeg calls that might retain references:
```go
ret := avformat_open_input(&ctx, path, nil, nil)
runtime.KeepAlive(path)
```

### 15.3 Critical: Callback Limit

**Problem**: purego has a hard limit of 2000 callbacks that are never freed.

**Affected Code**:
```go
// WRONG - Creates new callback each time
for i := 0; i < 10000; i++ {
    ctx := NewCustomIOContext(...)  // Panic after 2000
}

// CORRECT - Reuse callback functions
var readCallback = purego.NewCallback(...)  // Create once
for i := 0; i < 10000; i++ {
    ctx := newCustomIOContextWithCallback(readCallback, ...)
}
```

**Solution**: Create callbacks at init time, pass context via opaque pointer.

### 15.4 Critical: Library Load Order

**Problem**: FFmpeg libraries have interdependencies. Wrong order causes symbol resolution failures.

**Correct Order**:
```go
// MUST load in dependency order
avutil := purego.Dlopen("libavutil.so", RTLD_NOW|RTLD_GLOBAL)
swresample := purego.Dlopen("libswresample.so", RTLD_NOW|RTLD_GLOBAL)  // depends on avutil
avcodec := purego.Dlopen("libavcodec.so", RTLD_NOW|RTLD_GLOBAL)       // depends on avutil
avformat := purego.Dlopen("libavformat.so", RTLD_NOW|RTLD_GLOBAL)     // depends on avcodec, avutil
swscale := purego.Dlopen("libswscale.so", RTLD_NOW|RTLD_GLOBAL)       // depends on avutil
```

**RTLD_GLOBAL is REQUIRED**: Allows libraries to resolve symbols from each other.

### 15.5 Critical: Mixed Int/Float in SyscallN

**Problem**: `purego.SyscallN` does not correctly place arguments when mixing integers and floats.

**Affected Patterns**:
```go
// BROKEN with SyscallN
purego.SyscallN(fn, intArg, floatArg, intArg2)  // floatArg misplaced

// WORKS with RegisterFunc
var myFunc func(int, float64, int)
purego.RegisterLibFunc(&myFunc, lib, "my_func")
myFunc(1, 2.0, 3)  // Correct placement
```

**Solution**: Always use `RegisterFunc` for functions with float parameters.

### 15.6 Important: Null Pointer Checks

**Problem**: Passing nil to FFmpeg where it expects a valid pointer causes segfaults.

**Affected Functions**:
```go
// These require non-nil:
avcodec_send_packet(ctx, nil)      // OK - signals flush
avcodec_send_packet(nil, pkt)      // SEGFAULT

avformat_close_input(&ctx)         // OK if ctx is nil (no-op)
avformat_close_input(nil)          // SEGFAULT - needs pointer-to-pointer
```

**Solution**: Check for nil in Go wrappers:
```go
func (d *Decoder) Close() error {
    if d == nil || d.formatCtx == nil {
        return nil
    }
    avformat_close_input(&d.formatCtx)
    return nil
}
```

### 15.7 Important: Dictionary Ownership

**Problem**: FFmpeg functions may free or reallocate dictionaries passed to them.

**Affected Functions**:
```go
// Dictionary may be modified/freed by these:
avformat_open_input(&ctx, path, nil, &dict)  // dict may be freed
avcodec_open2(ctx, codec, &dict)             // dict may be modified
avformat_write_header(ctx, &dict)            // dict may be modified
```

**Solution**: Don't rely on dictionary after passing to FFmpeg:
```go
dict := createOptions()
err := avformat_open_input(&ctx, path, nil, &dict)
// dict may now be nil or different - don't use it
```

### 15.8 Important: Frame Data Pointers

**Problem**: AVFrame's data pointers point to internal buffers. Copying the struct copies pointers, not data.

**Wrong**:
```go
frameCopy := *frame  // Only copies pointers, not pixel data!
```

**Correct**:
```go
frameCopy := av_frame_alloc()
av_frame_ref(frameCopy, frame)  // Properly references data
```

### 15.9 Important: Seek Behavior

**Problem**: `av_seek_frame` behavior varies by container format and flags.

**Seek Flags**:
```go
const (
    AVSEEK_FLAG_BACKWARD = 1  // Seek to keyframe before target
    AVSEEK_FLAG_BYTE     = 2  // Seek by byte position
    AVSEEK_FLAG_ANY      = 4  // Seek to any frame (not just keyframe)
    AVSEEK_FLAG_FRAME    = 8  // Seek by frame number
)
```

**Solution**: Always seek backward to keyframe, then decode forward:
```go
func (d *Decoder) Seek(ts time.Duration) error {
    timestamp := ts.Nanoseconds() / 1000  // to microseconds
    // Seek to keyframe before target
    ret := av_seek_frame(d.formatCtx, -1, timestamp, AVSEEK_FLAG_BACKWARD)
    if ret < 0 {
        return newError(ret, "av_seek_frame")
    }
    // Flush decoder buffers
    avcodec_flush_buffers(d.codecCtx)
    return nil
}
```

### 15.10 Important: Thread Safety

**Problem**: FFmpeg contexts are not thread-safe for concurrent read/write.

**Rules**:
- Single AVFormatContext: One thread for reading
- Single AVCodecContext: One thread for decode/encode
- SwsContext: Thread-safe after creation
- Different contexts: Can be used from different threads

**Solution**: Use channels or mutexes in Go wrappers:
```go
type Decoder struct {
    mu sync.Mutex
    // ...
}

func (d *Decoder) ReadFrame() (*FrameWrapper, error) {
    d.mu.Lock()
    defer d.mu.Unlock()
    // ...
}
```

---

## 16. Performance Considerations

### 16.1 purego Overhead

| Operation | Overhead vs CGO |
|-----------|-----------------|
| Function call | ~50-100ns (reflection) |
| Callback invocation | ~100-200ns (trampoline + reflection) |
| String passing | +1 allocation if no null terminator |
| Struct passing | Similar (on Darwin) |

**Impact**: Negligible for media processing (frames take milliseconds).

### 16.2 Optimization Strategies

**1. Reuse frames and packets**:
```go
// SLOW - allocates each iteration
for {
    frame := av_frame_alloc()
    // use frame
    av_frame_free(&frame)
}

// FAST - reuse allocation
frame := av_frame_alloc()
defer av_frame_free(&frame)
for {
    av_frame_unref(frame)  // Reset without freeing
    // use frame
}
```

**2. Use pre-allocated buffers for custom I/O**:
```go
// SLOW - allocates each read
func (r *Reader) Read(p []byte) (int, error) {
    buf := make([]byte, len(p))  // Allocation!
    // ...
}

// FAST - caller provides buffer
func (r *Reader) Read(p []byte) (int, error) {
    // Use p directly, no allocation
}
```

**3. Avoid unnecessary frame copies**:
```go
// SLOW - copies frame data
decoded := decodeFrame()
scaled := scaler.Scale(decoded)
encoded := encoder.Encode(scaled)

// FAST - use frame references where possible
decoded := decodeFrame()
scaledRef := scaler.ScaleInPlace(decoded)  // Returns reference
encoder.Encode(scaledRef)
```

### 16.3 Benchmarks

Target performance (on modern hardware):

| Operation | Target | Notes |
|-----------|--------|-------|
| H.264 1080p decode | 200+ fps | With hardware assist |
| H.264 1080p decode | 60+ fps | Software only |
| H.264 1080p encode | 30+ fps | Software, medium preset |
| Frame scale 1080p→720p | 500+ fps | With SIMD |
| Function call overhead | <1% | Of total decode time |

---

## 17. Package Structure

### 17.1 Public Packages

| Package | Purpose | Stability |
|---------|---------|-----------|
| `ffgo` | High-level API | Stable |
| `ffgo/avutil` | Low-level avutil | Stable |
| `ffgo/avcodec` | Low-level avcodec | Stable |
| `ffgo/avformat` | Low-level avformat | Stable |
| `ffgo/swscale` | Low-level swscale | Stable |

### 17.2 Internal Packages

| Package | Purpose |
|---------|---------|
| `internal/bindings` | purego function registrations |
| `internal/handles` | Go object handle system |
| `internal/platform` | Platform detection and capabilities |
| `internal/version` | FFmpeg version detection |
| `internal/shim` | Shim loading and bindings |

### 17.3 Dependency Graph

```
ffgo (high-level)
├── ffgo/avutil
├── ffgo/avcodec
│   └── ffgo/avutil
├── ffgo/avformat
│   ├── ffgo/avutil
│   └── ffgo/avcodec
├── ffgo/swscale
│   └── ffgo/avutil
└── internal/*
```

---

## Appendix A: FFmpeg Function Bindings

### A.1 libavutil

```go
var (
    av_frame_alloc          func() unsafe.Pointer
    av_frame_free           func(frame *unsafe.Pointer)
    av_frame_ref            func(dst, src unsafe.Pointer) int32
    av_frame_unref          func(frame unsafe.Pointer)
    av_frame_get_buffer     func(frame unsafe.Pointer, align int32) int32
    av_frame_make_writable  func(frame unsafe.Pointer) int32

    av_malloc               func(size uintptr) unsafe.Pointer
    av_free                 func(ptr unsafe.Pointer)
    av_freep                func(ptr *unsafe.Pointer)

    av_dict_set             func(pm *unsafe.Pointer, key, value string, flags int32) int32
    av_dict_get             func(m unsafe.Pointer, key string, prev unsafe.Pointer, flags int32) unsafe.Pointer
    av_dict_free            func(pm *unsafe.Pointer)

    av_strerror             func(errnum int32, errbuf unsafe.Pointer, errbuf_size uintptr) int32
    av_log_set_level        func(level int32)
    avutil_version          func() uint32
)
```

### A.2 libavcodec

```go
var (
    avcodec_find_decoder     func(id int32) unsafe.Pointer
    avcodec_find_encoder     func(id int32) unsafe.Pointer
    avcodec_find_decoder_by_name func(name string) unsafe.Pointer
    avcodec_find_encoder_by_name func(name string) unsafe.Pointer
    avcodec_alloc_context3   func(codec unsafe.Pointer) unsafe.Pointer
    avcodec_free_context     func(ctx *unsafe.Pointer)
    avcodec_open2            func(ctx, codec unsafe.Pointer, options *unsafe.Pointer) int32
    avcodec_close            func(ctx unsafe.Pointer) int32
    avcodec_send_packet      func(ctx, pkt unsafe.Pointer) int32
    avcodec_receive_frame    func(ctx, frame unsafe.Pointer) int32
    avcodec_send_frame       func(ctx, frame unsafe.Pointer) int32
    avcodec_receive_packet   func(ctx, pkt unsafe.Pointer) int32
    avcodec_flush_buffers    func(ctx unsafe.Pointer)
    avcodec_parameters_to_context func(ctx, par unsafe.Pointer) int32
    avcodec_parameters_from_context func(par, ctx unsafe.Pointer) int32

    av_packet_alloc          func() unsafe.Pointer
    av_packet_free           func(pkt *unsafe.Pointer)
    av_packet_ref            func(dst, src unsafe.Pointer) int32
    av_packet_unref          func(pkt unsafe.Pointer)

    avcodec_version          func() uint32
)
```

### A.3 libavformat

```go
var (
    avformat_open_input      func(ctx *unsafe.Pointer, url string, fmt, options unsafe.Pointer) int32
    avformat_close_input     func(ctx *unsafe.Pointer)
    avformat_find_stream_info func(ctx unsafe.Pointer, options *unsafe.Pointer) int32
    avformat_alloc_context   func() unsafe.Pointer
    avformat_free_context    func(ctx unsafe.Pointer)
    avformat_alloc_output_context2 func(ctx *unsafe.Pointer, oformat unsafe.Pointer, format_name, filename string) int32
    avformat_new_stream      func(ctx, codec unsafe.Pointer) unsafe.Pointer
    avformat_write_header    func(ctx unsafe.Pointer, options *unsafe.Pointer) int32
    av_write_trailer         func(ctx unsafe.Pointer) int32

    av_read_frame            func(ctx, pkt unsafe.Pointer) int32
    av_write_frame           func(ctx, pkt unsafe.Pointer) int32
    av_interleaved_write_frame func(ctx, pkt unsafe.Pointer) int32
    av_seek_frame            func(ctx unsafe.Pointer, stream_index int32, timestamp int64, flags int32) int32

    av_find_best_stream      func(ctx unsafe.Pointer, mediaType, wanted, related int32, decoder *unsafe.Pointer, flags int32) int32

    avio_open                func(ctx *unsafe.Pointer, url string, flags int32) int32
    avio_close               func(ctx unsafe.Pointer) int32
    avio_alloc_context       func(buffer unsafe.Pointer, buffer_size, write_flag int32, opaque unsafe.Pointer, read_packet, write_packet, seek uintptr) unsafe.Pointer

    avformat_version         func() uint32
)
```

### A.4 libswscale

```go
var (
    sws_getContext           func(srcW, srcH int32, srcFormat int32, dstW, dstH int32, dstFormat int32, flags int32, srcFilter, dstFilter, param unsafe.Pointer) unsafe.Pointer
    sws_scale                func(ctx unsafe.Pointer, srcSlice, srcStride unsafe.Pointer, srcSliceY, srcSliceH int32, dst, dstStride unsafe.Pointer) int32
    sws_freeContext          func(ctx unsafe.Pointer)
    sws_scale_frame          func(ctx, dst, src unsafe.Pointer) int32

    swscale_version          func() uint32
)
```

---

## Appendix B: Shim Function Bindings

```go
var (
    ffshim_log_set_callback  func(cb uintptr)
    ffshim_log_set_level     func(level int32)
    ffshim_log               func(avcl unsafe.Pointer, level int32, msg string)

    ffshim_rational_mul      func(a_num, a_den, b_num, b_den int32, out_num, out_den *int32)
    ffshim_rational_div      func(a_num, a_den, b_num, b_den int32, out_num, out_den *int32)
    ffshim_rational_add      func(a_num, a_den, b_num, b_den int32, out_num, out_den *int32)
    ffshim_rational_sub      func(a_num, a_den, b_num, b_den int32, out_num, out_den *int32)
    ffshim_d2q               func(d float64, max_den int32, out_num, out_den *int32)
    ffshim_q2d               func(num, den int32) float64
    ffshim_rational_cmp      func(a_num, a_den, b_num, b_den int32) int32

    ffshim_strerror          func(errnum int32, errbuf unsafe.Pointer, errbuf_size uintptr) int32
    ffshim_avio_write_string func(ctx unsafe.Pointer, str string)

    ffshim_avutil_version    func() uint32
    ffshim_avcodec_version   func() uint32
    ffshim_avformat_version  func() uint32
)
```

---

## 18. Audio Resampling (swresample)

### 18.1 Library Dependencies

| Library | Required | Notes |
|---------|----------|-------|
| libswresample | Yes | Must be loaded after libavutil |

**Loading order**: avutil → swresample → avcodec → avformat → swscale

### 18.2 Function Bindings

```go
// swresample/swresample.go
var (
    swr_alloc              func() unsafe.Pointer
    swr_alloc_set_opts2    func(ps *unsafe.Pointer, outChLayout unsafe.Pointer, outFmt int32, outRate int32,
                                inChLayout unsafe.Pointer, inFmt int32, inRate int32,
                                logOffset int32, logCtx unsafe.Pointer) int32
    swr_init               func(s unsafe.Pointer) int32
    swr_free               func(s *unsafe.Pointer)
    swr_convert            func(s unsafe.Pointer, out, in unsafe.Pointer, outCount, inCount int32) int32
    swr_convert_frame      func(s unsafe.Pointer, output, input unsafe.Pointer) int32
    swr_get_delay          func(s unsafe.Pointer, base int64) int64
    swr_get_out_samples    func(s unsafe.Pointer, inSamples int32) int32
    swr_is_initialized     func(s unsafe.Pointer) int32

    swresample_version     func() uint32
)
```

**purego Compatibility**: All functions ✓ (no variadics, no struct returns)

### 18.3 AVChannelLayout Handling

FFmpeg 5.1+ uses `AVChannelLayout` struct (24 bytes) instead of `uint64` channel mask.

```go
// AVChannelLayout struct layout (FFmpeg 5.1+)
// Offset 0:  order (AVChannelOrder enum, 4 bytes)
// Offset 4:  nb_channels (int, 4 bytes)
// Offset 8:  u.mask (uint64) or u.map (pointer)
// Offset 16: opaque (pointer)

// For purego: Always pass by pointer, never by value
type AVChannelLayout struct {
    Order      int32
    NbChannels int32
    Mask       uint64  // Union with map pointer
    Opaque     unsafe.Pointer
}

// Helper to create layout from mask
func channelLayoutFromMask(mask uint64, channels int) []byte {
    buf := make([]byte, 24)
    binary.LittleEndian.PutUint32(buf[0:4], 0)        // AV_CHANNEL_ORDER_NATIVE
    binary.LittleEndian.PutUint32(buf[4:8], uint32(channels))
    binary.LittleEndian.PutUint64(buf[8:16], mask)
    return buf
}
```

### 18.4 High-Level Resampler Implementation

```go
// Resampler wraps SwrContext for audio format conversion
type Resampler struct {
    ctx         unsafe.Pointer  // SwrContext*
    srcFormat   AudioFormat
    dstFormat   AudioFormat
    srcLayout   []byte          // AVChannelLayout buffer (kept alive)
    dstLayout   []byte          // AVChannelLayout buffer (kept alive)
    outputFrame unsafe.Pointer  // Reusable output frame
    closed      bool
}

func NewResampler(src, dst AudioFormat) (*Resampler, error) {
    r := &Resampler{
        srcFormat: src,
        dstFormat: dst,
    }

    // Create channel layouts
    r.srcLayout = channelLayoutFromMask(uint64(src.ChannelLayout), src.Channels)
    r.dstLayout = channelLayoutFromMask(uint64(dst.ChannelLayout), dst.Channels)

    // Allocate context
    var ctx unsafe.Pointer
    ret := swr_alloc_set_opts2(
        &ctx,
        unsafe.Pointer(&r.dstLayout[0]), int32(dst.SampleFormat), int32(dst.SampleRate),
        unsafe.Pointer(&r.srcLayout[0]), int32(src.SampleFormat), int32(src.SampleRate),
        0, nil,
    )
    if ret < 0 {
        return nil, newError(ret, "swr_alloc_set_opts2")
    }

    runtime.KeepAlive(r.srcLayout)
    runtime.KeepAlive(r.dstLayout)

    ret = swr_init(ctx)
    if ret < 0 {
        swr_free(&ctx)
        return nil, newError(ret, "swr_init")
    }

    r.ctx = ctx
    r.outputFrame = av_frame_alloc()

    runtime.SetFinalizer(r, (*Resampler).cleanup)
    return r, nil
}

func (r *Resampler) Resample(frame *Frame) (*Frame, error) {
    if r.closed {
        return nil, ErrClosed
    }

    av_frame_unref(r.outputFrame)

    ret := swr_convert_frame(r.ctx, r.outputFrame, frame.ptr)
    if ret < 0 {
        return nil, newError(ret, "swr_convert_frame")
    }

    // Check if output has samples
    nbSamples := getFrameNbSamples(r.outputFrame)
    if nbSamples == 0 {
        return nil, nil  // Need more input
    }

    // Create new frame with copy of data
    outFrame := &Frame{ptr: av_frame_alloc()}
    av_frame_ref(outFrame.ptr, r.outputFrame)
    return outFrame, nil
}

func (r *Resampler) Close() error {
    if r.closed {
        return nil
    }
    r.closed = true
    r.cleanup()
    return nil
}

func (r *Resampler) cleanup() {
    if r.outputFrame != nil {
        av_frame_free(&r.outputFrame)
    }
    if r.ctx != nil {
        swr_free(&r.ctx)
    }
}
```

---

## 19. Filter Graphs (avfilter)

### 19.1 Library Dependencies

| Library | Required | Notes |
|---------|----------|-------|
| libavfilter | Yes | Must be loaded after avcodec |

**Loading order**: avutil → swresample → avcodec → avformat → swscale → avfilter

### 19.2 Function Bindings

```go
// avfilter/avfilter.go
var (
    // Graph management
    avfilter_graph_alloc           func() unsafe.Pointer
    avfilter_graph_free            func(graph *unsafe.Pointer)
    avfilter_graph_config          func(graphctx unsafe.Pointer, log_ctx unsafe.Pointer) int32
    avfilter_graph_parse2          func(graph unsafe.Pointer, filters string,
                                        inputs, outputs *unsafe.Pointer) int32

    // Filter lookup
    avfilter_get_by_name           func(name string) unsafe.Pointer

    // Filter creation
    avfilter_graph_create_filter   func(filt_ctx *unsafe.Pointer, filt unsafe.Pointer,
                                        name, args string, opaque, graph_ctx unsafe.Pointer) int32

    // Filter linking
    avfilter_link                  func(src unsafe.Pointer, srcpad uint32,
                                        dst unsafe.Pointer, dstpad uint32) int32

    // Buffer source/sink
    av_buffersrc_add_frame_flags   func(ctx unsafe.Pointer, frame unsafe.Pointer, flags int32) int32
    av_buffersink_get_frame_flags  func(ctx unsafe.Pointer, frame unsafe.Pointer, flags int32) int32

    // InOut management
    avfilter_inout_alloc           func() unsafe.Pointer
    avfilter_inout_free            func(inout *unsafe.Pointer)

    avfilter_version               func() uint32
)

// Buffer source flags
const (
    AV_BUFFERSRC_FLAG_KEEP_REF = 8  // Keep reference to frame
    AV_BUFFERSRC_FLAG_PUSH     = 4  // Push frame immediately
)

// Buffer sink flags
const (
    AV_BUFFERSINK_FLAG_PEEK      = 1  // Peek without consuming
    AV_BUFFERSINK_FLAG_NO_REQUEST = 2  // Don't request frame
)
```

**purego Compatibility**: All functions ✓

**Note**: `avfilter_graph_parse_ptr` is variadic - use `avfilter_graph_parse2` instead.

### 19.3 Filter Graph Implementation

```go
// FilterGraph represents a filter processing pipeline
type FilterGraph struct {
    graph        unsafe.Pointer  // AVFilterGraph*
    bufferSrc    unsafe.Pointer  // AVFilterContext* for buffersrc
    bufferSink   unsafe.Pointer  // AVFilterContext* for buffersink
    inputFrame   unsafe.Pointer  // Reusable input frame reference
    outputFrame  unsafe.Pointer  // Reusable output frame
    isVideo      bool
    srcFormat    FilterFormat
    closed       bool
}

// FilterFormat describes filter input/output format
type FilterFormat struct {
    // Video
    Width       int
    Height      int
    PixelFormat PixelFormat
    TimeBase    Rational
    FrameRate   Rational
    SAR         Rational  // Sample aspect ratio

    // Audio
    SampleRate    int
    Channels      int
    ChannelLayout ChannelLayout
    SampleFormat  SampleFormat
}

func NewVideoFilterGraph(filters string, width, height int, pixFmt PixelFormat) (*FilterGraph, error) {
    g := &FilterGraph{
        isVideo: true,
        srcFormat: FilterFormat{
            Width:       width,
            Height:      height,
            PixelFormat: pixFmt,
            TimeBase:    NewRational(1, 90000),
        },
    }

    // Allocate graph
    g.graph = avfilter_graph_alloc()
    if g.graph == nil {
        return nil, ErrOutOfMemory
    }

    // Create buffer source args
    srcArgs := fmt.Sprintf("video_size=%dx%d:pix_fmt=%d:time_base=%d/%d:pixel_aspect=%d/%d",
        width, height, int(pixFmt), 1, 90000, 1, 1)

    // Create buffersrc
    buffersrc := avfilter_get_by_name("buffer")
    if buffersrc == nil {
        avfilter_graph_free(&g.graph)
        return nil, errors.New("buffer filter not found")
    }

    ret := avfilter_graph_create_filter(&g.bufferSrc, buffersrc, "in", srcArgs, nil, g.graph)
    if ret < 0 {
        avfilter_graph_free(&g.graph)
        return nil, newError(ret, "create buffersrc")
    }

    // Create buffersink
    buffersink := avfilter_get_by_name("buffersink")
    if buffersink == nil {
        avfilter_graph_free(&g.graph)
        return nil, errors.New("buffersink filter not found")
    }

    ret = avfilter_graph_create_filter(&g.bufferSink, buffersink, "out", "", nil, g.graph)
    if ret < 0 {
        avfilter_graph_free(&g.graph)
        return nil, newError(ret, "create buffersink")
    }

    // Parse filter string
    var inputs, outputs unsafe.Pointer
    ret = avfilter_graph_parse2(g.graph, filters, &inputs, &outputs)
    if ret < 0 {
        avfilter_graph_free(&g.graph)
        return nil, newError(ret, "parse filter graph")
    }

    // Link buffersrc to inputs, outputs to buffersink
    // (simplified - actual implementation needs to handle AVFilterInOut chain)

    // Configure graph
    ret = avfilter_graph_config(g.graph, nil)
    if ret < 0 {
        avfilter_graph_free(&g.graph)
        return nil, newError(ret, "configure filter graph")
    }

    g.outputFrame = av_frame_alloc()
    runtime.SetFinalizer(g, (*FilterGraph).cleanup)
    return g, nil
}

func (g *FilterGraph) Filter(frame *Frame) ([]*Frame, error) {
    if g.closed {
        return nil, ErrClosed
    }

    // Push frame to buffersrc
    ret := av_buffersrc_add_frame_flags(g.bufferSrc, frame.ptr, AV_BUFFERSRC_FLAG_KEEP_REF)
    if ret < 0 {
        return nil, newError(ret, "buffersrc add frame")
    }
    runtime.KeepAlive(frame)

    // Pull frames from buffersink
    var frames []*Frame
    for {
        av_frame_unref(g.outputFrame)
        ret = av_buffersink_get_frame_flags(g.bufferSink, g.outputFrame, 0)
        if ret == AVERROR_EAGAIN || ret == AVERROR_EOF {
            break
        }
        if ret < 0 {
            return frames, newError(ret, "buffersink get frame")
        }

        outFrame := &Frame{ptr: av_frame_alloc()}
        av_frame_ref(outFrame.ptr, g.outputFrame)
        frames = append(frames, outFrame)
    }

    return frames, nil
}

func (g *FilterGraph) Close() error {
    if g.closed {
        return nil
    }
    g.closed = true
    g.cleanup()
    return nil
}

func (g *FilterGraph) cleanup() {
    if g.outputFrame != nil {
        av_frame_free(&g.outputFrame)
    }
    if g.graph != nil {
        avfilter_graph_free(&g.graph)
    }
}
```

---

## 20. Advanced Codec Options

### 20.1 Option Application via av_opt_set

FFmpeg codec options are set through the AVOptions API.

```go
// av_opt_set functions
var (
    av_opt_set        func(obj unsafe.Pointer, name, val string, search_flags int32) int32
    av_opt_set_int    func(obj unsafe.Pointer, name string, val int64, search_flags int32) int32
    av_opt_set_double func(obj unsafe.Pointer, name string, val float64, search_flags int32) int32
)

const (
    AV_OPT_SEARCH_CHILDREN = 1  // Search in child objects
)
```

### 20.2 VideoEncoderConfig Extended

```go
type VideoEncoderConfig struct {
    // Existing fields
    Codec     CodecID
    Width     int
    Height    int
    FrameRate Rational
    Bitrate   int64

    // NEW: Preset and tuning
    Preset    EncoderPreset
    Tune      EncoderTune

    // NEW: Profile and level
    Profile   VideoProfile
    Level     VideoLevel

    // NEW: Rate control
    RateControl RateControlMode
    CRF         int    // 0-51 for x264, 0-63 for x265
    CQP         int    // Constant QP
    MinBitrate  int64
    MaxBitrate  int64
    BufferSize  int64  // VBV buffer

    // NEW: GOP structure
    GOPSize        int
    MaxBFrames     int
    BFrameStrategy int
    RefFrames      int

    // NEW: Quality
    Threads int

    // NEW: Codec-specific options
    CodecOptions map[string]string
}

// Apply options to codec context
func applyVideoOptions(ctx unsafe.Pointer, config *VideoEncoderConfig) error {
    // Basic settings applied via struct field writes
    setCodecContextInt(ctx, offsetWidth, config.Width)
    setCodecContextInt(ctx, offsetHeight, config.Height)
    setCodecContextInt64(ctx, offsetBitRate, config.Bitrate)
    setCodecContextInt(ctx, offsetGOPSize, config.GOPSize)
    setCodecContextInt(ctx, offsetMaxBFrames, config.MaxBFrames)

    // Advanced options via av_opt_set
    if config.Preset != "" {
        av_opt_set(ctx, "preset", string(config.Preset), AV_OPT_SEARCH_CHILDREN)
    }
    if config.Tune != "" {
        av_opt_set(ctx, "tune", string(config.Tune), AV_OPT_SEARCH_CHILDREN)
    }
    if config.Profile != "" {
        av_opt_set(ctx, "profile", string(config.Profile), AV_OPT_SEARCH_CHILDREN)
    }
    if config.Level != "" {
        av_opt_set(ctx, "level", string(config.Level), AV_OPT_SEARCH_CHILDREN)
    }

    // Rate control
    switch config.RateControl {
    case RateControlCRF:
        av_opt_set_int(ctx, "crf", int64(config.CRF), AV_OPT_SEARCH_CHILDREN)
    case RateControlCQP:
        av_opt_set_int(ctx, "qp", int64(config.CQP), AV_OPT_SEARCH_CHILDREN)
    }

    // Threading
    if config.Threads > 0 {
        setCodecContextInt(ctx, offsetThreadCount, config.Threads)
    }

    // Custom options
    for key, value := range config.CodecOptions {
        av_opt_set(ctx, key, value, AV_OPT_SEARCH_CHILDREN)
    }

    return nil
}
```

---

## 21. Audio Encoding

### 21.1 Implementation

Audio encoding follows the same pattern as video but with audio-specific frame handling.

```go
// Complete audio encoder implementation
func (e *Encoder) WriteAudioFrame(frame *Frame) error {
    if e.audioCodecCtx == nil {
        return ErrNoAudioStream
    }

    // Send frame to encoder
    ret := avcodec_send_frame(e.audioCodecCtx, frame.ptr)
    if ret < 0 && ret != AVERROR_EAGAIN {
        return newError(ret, "avcodec_send_frame (audio)")
    }
    runtime.KeepAlive(frame)

    // Receive and write packets
    for {
        ret := avcodec_receive_packet(e.audioCodecCtx, e.audioPacket)
        if ret == AVERROR_EAGAIN || ret == AVERROR_EOF {
            break
        }
        if ret < 0 {
            return newError(ret, "avcodec_receive_packet (audio)")
        }

        // Rescale timestamps
        av_packet_rescale_ts(e.audioPacket, e.audioTimeBase, e.audioStreamTimeBase)

        // Set stream index
        setPacketStreamIndex(e.audioPacket, e.audioStreamIndex)

        // Write packet
        ret = av_interleaved_write_frame(e.formatCtx, e.audioPacket)
        if ret < 0 {
            return newError(ret, "av_interleaved_write_frame (audio)")
        }

        av_packet_unref(e.audioPacket)
    }

    return nil
}
```

### 21.2 Frame Size Handling

Many audio encoders (AAC, MP3, Opus) require fixed frame sizes.

```go
// AudioFrameBuffer handles frame size requirements
type AudioFrameBuffer struct {
    encoder      *Encoder
    sampleBuffer []byte
    sampleCount  int
    frameSize    int        // Required samples per frame
    format       SampleFormat
    channels     int
    pts          int64
}

func NewAudioFrameBuffer(encoder *Encoder, frameSize int, format SampleFormat, channels int) *AudioFrameBuffer {
    bytesPerSample := getSampleFormatSize(format) * channels
    return &AudioFrameBuffer{
        encoder:      encoder,
        sampleBuffer: make([]byte, frameSize*bytesPerSample),
        frameSize:    frameSize,
        format:       format,
        channels:     channels,
    }
}

func (b *AudioFrameBuffer) Write(frame *Frame) error {
    // Copy samples to buffer
    samples := frame.Data(0)
    nb := frame.NbSamples()

    // ... accumulate samples, encode when frameSize reached
    return nil
}
```

---

## 22. Stream Copy Mode

### 22.1 Implementation

Stream copy writes packets directly without decoding/encoding.

```go
// Encoder with stream copy support
type Encoder struct {
    // ... existing fields

    copyVideoStream bool
    copyAudioStream bool
    videoTimeBase   Rational  // Source video time base
    audioTimeBase   Rational  // Source audio time base
}

// WritePacket writes a packet directly (for stream copy)
func (e *Encoder) WritePacket(packet *Packet) error {
    streamIndex := packet.StreamIndex()

    var timeBase Rational
    if streamIndex == e.videoStreamIndex {
        timeBase = e.videoTimeBase
    } else if streamIndex == e.audioStreamIndex {
        timeBase = e.audioTimeBase
    } else {
        return ErrInvalidStream
    }

    // Rescale timestamps to output stream time base
    outputTimeBase := e.getStreamTimeBase(streamIndex)
    av_packet_rescale_ts(packet.ptr, timeBase.toAVRational(), outputTimeBase.toAVRational())

    // Write interleaved
    ret := av_interleaved_write_frame(e.formatCtx, packet.ptr)
    if ret < 0 {
        return newError(ret, "av_interleaved_write_frame")
    }

    return nil
}
```

---

## 23. Bitstream Filters

### 23.1 Function Bindings

```go
var (
    av_bsf_get_by_name      func(name string) unsafe.Pointer
    av_bsf_alloc            func(filter unsafe.Pointer, ctx *unsafe.Pointer) int32
    av_bsf_init             func(ctx unsafe.Pointer) int32
    av_bsf_send_packet      func(ctx unsafe.Pointer, pkt unsafe.Pointer) int32
    av_bsf_receive_packet   func(ctx unsafe.Pointer, pkt unsafe.Pointer) int32
    av_bsf_free             func(ctx *unsafe.Pointer)
)
```

### 23.2 Implementation

```go
type BitstreamFilter struct {
    ctx      unsafe.Pointer  // AVBSFContext*
    outPkt   unsafe.Pointer  // Output packet
    closed   bool
}

func NewBitstreamFilter(name string, codecPar *CodecParameters) (*BitstreamFilter, error) {
    filter := av_bsf_get_by_name(name)
    if filter == nil {
        return nil, fmt.Errorf("bitstream filter not found: %s", name)
    }

    var ctx unsafe.Pointer
    ret := av_bsf_alloc(filter, &ctx)
    if ret < 0 {
        return nil, newError(ret, "av_bsf_alloc")
    }

    // Copy codec parameters to BSF context
    avcodec_parameters_copy(getBSFCodecPar(ctx), codecPar.ptr)

    ret = av_bsf_init(ctx)
    if ret < 0 {
        av_bsf_free(&ctx)
        return nil, newError(ret, "av_bsf_init")
    }

    return &BitstreamFilter{
        ctx:    ctx,
        outPkt: av_packet_alloc(),
    }, nil
}

func (f *BitstreamFilter) Filter(packet *Packet) ([]*Packet, error) {
    ret := av_bsf_send_packet(f.ctx, packet.ptr)
    if ret < 0 {
        return nil, newError(ret, "av_bsf_send_packet")
    }

    var packets []*Packet
    for {
        av_packet_unref(f.outPkt)
        ret := av_bsf_receive_packet(f.ctx, f.outPkt)
        if ret == AVERROR_EAGAIN || ret == AVERROR_EOF {
            break
        }
        if ret < 0 {
            return packets, newError(ret, "av_bsf_receive_packet")
        }

        pkt := &Packet{ptr: av_packet_alloc()}
        av_packet_ref(pkt.ptr, f.outPkt)
        packets = append(packets, pkt)
    }

    return packets, nil
}

func (f *BitstreamFilter) Close() error {
    if f.closed {
        return nil
    }
    f.closed = true
    av_packet_free(&f.outPkt)
    av_bsf_free(&f.ctx)
    return nil
}
```

---

## 24. Hardware Acceleration (Complete)

### 24.1 Function Bindings

```go
var (
    av_hwdevice_ctx_create    func(device_ctx *unsafe.Pointer, dtype int32,
                                   device string, opts unsafe.Pointer, flags int32) int32
    av_hwdevice_ctx_alloc     func(dtype int32) unsafe.Pointer
    av_hwframe_ctx_alloc      func(device_ctx unsafe.Pointer) unsafe.Pointer
    av_hwframe_ctx_init       func(ref unsafe.Pointer) int32
    av_hwframe_transfer_data  func(dst, src unsafe.Pointer, flags int32) int32
    av_hwframe_get_buffer     func(hwframe_ctx unsafe.Pointer, frame unsafe.Pointer, flags int32) int32
    av_buffer_ref             func(buf unsafe.Pointer) unsafe.Pointer
    av_buffer_unref           func(buf *unsafe.Pointer)
)

// Hardware device types
const (
    AV_HWDEVICE_TYPE_NONE         = 0
    AV_HWDEVICE_TYPE_VDPAU        = 1
    AV_HWDEVICE_TYPE_CUDA         = 2
    AV_HWDEVICE_TYPE_VAAPI        = 3
    AV_HWDEVICE_TYPE_DXVA2        = 4
    AV_HWDEVICE_TYPE_QSV          = 5
    AV_HWDEVICE_TYPE_VIDEOTOOLBOX = 6
    AV_HWDEVICE_TYPE_D3D11VA      = 7
    AV_HWDEVICE_TYPE_DRM          = 8
    AV_HWDEVICE_TYPE_OPENCL       = 9
    AV_HWDEVICE_TYPE_MEDIACODEC   = 10
    AV_HWDEVICE_TYPE_VULKAN       = 11
)
```

### 24.2 HWDevice Implementation

```go
type HWDevice struct {
    deviceType int32
    deviceCtx  unsafe.Pointer  // AVBufferRef*
    devicePath string
}

func NewHWDevice(deviceType HWDeviceType, device string) (*HWDevice, error) {
    var deviceCtx unsafe.Pointer

    ret := av_hwdevice_ctx_create(&deviceCtx, int32(deviceType), device, nil, 0)
    if ret < 0 {
        return nil, newError(ret, "av_hwdevice_ctx_create")
    }

    return &HWDevice{
        deviceType: int32(deviceType),
        deviceCtx:  deviceCtx,
        devicePath: device,
    }, nil
}

func (d *HWDevice) Close() error {
    if d.deviceCtx != nil {
        av_buffer_unref(&d.deviceCtx)
        d.deviceCtx = nil
    }
    return nil
}
```

### 24.3 Hardware Frame Transfer

```go
// TransferFrameToSystem copies HW frame to system memory
func TransferFrameToSystem(hwFrame *Frame) (*Frame, error) {
    swFrame := av_frame_alloc()
    if swFrame == nil {
        return nil, ErrOutOfMemory
    }

    ret := av_hwframe_transfer_data(swFrame, hwFrame.ptr, 0)
    if ret < 0 {
        av_frame_free(&swFrame)
        return nil, newError(ret, "av_hwframe_transfer_data")
    }

    // Copy metadata
    av_frame_copy_props(swFrame, hwFrame.ptr)

    return &Frame{ptr: swFrame}, nil
}
```

---

## 25. Subtitle Support

### 25.1 Function Bindings

```go
var (
    avcodec_decode_subtitle2  func(avctx unsafe.Pointer, sub unsafe.Pointer,
                                   got_sub *int32, pkt unsafe.Pointer) int32
    avcodec_encode_subtitle   func(avctx unsafe.Pointer, buf unsafe.Pointer,
                                   buf_size int32, sub unsafe.Pointer) int32
    avsubtitle_free           func(sub unsafe.Pointer)
)

// AVSubtitle structure offsets
const (
    offsetSubFormat    = 0   // uint16
    offsetSubStartTime = 4   // uint32
    offsetSubEndTime   = 8   // uint32
    offsetSubNumRects  = 12  // uint32
    offsetSubRects     = 16  // AVSubtitleRect**
    offsetSubPTS       = 24  // int64
)
```

### 25.2 Subtitle Implementation

```go
type Subtitle struct {
    Type      SubtitleType
    StartTime time.Duration
    EndTime   time.Duration
    Text      string
    ASS       string
    Rects     []SubtitleRect
}

type SubtitleRect struct {
    X, Y          int
    Width, Height int
    Data          []byte
}

type SubtitleDecoder struct {
    codecCtx unsafe.Pointer
    subtitle []byte  // AVSubtitle buffer
}

func NewSubtitleDecoder(stream *StreamInfo) (*SubtitleDecoder, error) {
    codec := avcodec_find_decoder(stream.CodecID())
    if codec == nil {
        return nil, ErrDecoderNotFound
    }

    ctx := avcodec_alloc_context3(codec)
    if ctx == nil {
        return nil, ErrOutOfMemory
    }

    avcodec_parameters_to_context(ctx, stream.CodecParameters())

    ret := avcodec_open2(ctx, codec, nil)
    if ret < 0 {
        avcodec_free_context(&ctx)
        return nil, newError(ret, "avcodec_open2")
    }

    // Allocate AVSubtitle (sizeof varies by FFmpeg version, use safe size)
    return &SubtitleDecoder{
        codecCtx: ctx,
        subtitle: make([]byte, 256),
    }, nil
}

func (d *SubtitleDecoder) Decode(packet *Packet) (*Subtitle, error) {
    var gotSub int32

    ret := avcodec_decode_subtitle2(d.codecCtx, unsafe.Pointer(&d.subtitle[0]),
        &gotSub, packet.ptr)
    if ret < 0 {
        return nil, newError(ret, "avcodec_decode_subtitle2")
    }

    if gotSub == 0 {
        return nil, nil
    }

    // Parse AVSubtitle structure
    sub := d.parseSubtitle()

    // Free FFmpeg's internal data
    avsubtitle_free(unsafe.Pointer(&d.subtitle[0]))

    return sub, nil
}

func (d *SubtitleDecoder) parseSubtitle() *Subtitle {
    ptr := unsafe.Pointer(&d.subtitle[0])

    format := *(*uint16)(unsafe.Add(ptr, offsetSubFormat))
    startTime := *(*uint32)(unsafe.Add(ptr, offsetSubStartTime))
    endTime := *(*uint32)(unsafe.Add(ptr, offsetSubEndTime))

    return &Subtitle{
        Type:      SubtitleType(format),
        StartTime: time.Duration(startTime) * time.Millisecond,
        EndTime:   time.Duration(endTime) * time.Millisecond,
        // Parse rects for bitmap, text for ASS/SRT
    }
}
```

---

## Appendix C: New Library Function Bindings

### C.1 libswresample

```go
var (
    swr_alloc              func() unsafe.Pointer
    swr_alloc_set_opts2    func(ps *unsafe.Pointer, outChLayout, inChLayout unsafe.Pointer,
                                outFmt, inFmt int32, outRate, inRate int32,
                                logOffset int32, logCtx unsafe.Pointer) int32
    swr_init               func(s unsafe.Pointer) int32
    swr_free               func(s *unsafe.Pointer)
    swr_convert            func(s unsafe.Pointer, out, in unsafe.Pointer, outCount, inCount int32) int32
    swr_convert_frame      func(s unsafe.Pointer, output, input unsafe.Pointer) int32
    swr_get_delay          func(s unsafe.Pointer, base int64) int64
    swr_get_out_samples    func(s unsafe.Pointer, inSamples int32) int32
    swresample_version     func() uint32
)
```

### C.2 libavfilter

```go
var (
    avfilter_graph_alloc         func() unsafe.Pointer
    avfilter_graph_free          func(graph *unsafe.Pointer)
    avfilter_graph_config        func(graphctx, log_ctx unsafe.Pointer) int32
    avfilter_graph_parse2        func(graph unsafe.Pointer, filters string,
                                      inputs, outputs *unsafe.Pointer) int32
    avfilter_get_by_name         func(name string) unsafe.Pointer
    avfilter_graph_create_filter func(filt_ctx *unsafe.Pointer, filt unsafe.Pointer,
                                      name, args string, opaque, graph_ctx unsafe.Pointer) int32
    avfilter_link                func(src unsafe.Pointer, srcpad uint32,
                                      dst unsafe.Pointer, dstpad uint32) int32
    av_buffersrc_add_frame_flags func(ctx, frame unsafe.Pointer, flags int32) int32
    av_buffersink_get_frame_flags func(ctx, frame unsafe.Pointer, flags int32) int32
    avfilter_inout_alloc         func() unsafe.Pointer
    avfilter_inout_free          func(inout *unsafe.Pointer)
    avfilter_version             func() uint32
)
```

### C.3 Hardware Acceleration

```go
var (
    av_hwdevice_ctx_create   func(device_ctx *unsafe.Pointer, dtype int32,
                                  device string, opts unsafe.Pointer, flags int32) int32
    av_hwframe_transfer_data func(dst, src unsafe.Pointer, flags int32) int32
    av_buffer_ref            func(buf unsafe.Pointer) unsafe.Pointer
    av_buffer_unref          func(buf *unsafe.Pointer)
)
```

### C.4 Bitstream Filters

```go
var (
    av_bsf_get_by_name    func(name string) unsafe.Pointer
    av_bsf_alloc          func(filter unsafe.Pointer, ctx *unsafe.Pointer) int32
    av_bsf_init           func(ctx unsafe.Pointer) int32
    av_bsf_send_packet    func(ctx, pkt unsafe.Pointer) int32
    av_bsf_receive_packet func(ctx, pkt unsafe.Pointer) int32
    av_bsf_free           func(ctx *unsafe.Pointer)
)
```

### C.5 AV Options

```go
var (
    av_opt_set        func(obj unsafe.Pointer, name, val string, search_flags int32) int32
    av_opt_set_int    func(obj unsafe.Pointer, name string, val int64, search_flags int32) int32
    av_opt_set_double func(obj unsafe.Pointer, name string, val float64, search_flags int32) int32
    av_opt_get        func(obj unsafe.Pointer, name string, search_flags int32, out *unsafe.Pointer) int32
)
```

---

## Revision History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0.0-draft | 2025-01-14 | Claude | Initial specification |
| 2.0.0-draft | 2026-01-14 | Claude | Added swresample, avfilter, advanced codec options, audio encoding, stream copy, bitstream filters, hardware acceleration, subtitles |
