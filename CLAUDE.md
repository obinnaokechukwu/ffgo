# Claude Memory - ffgo Project

## Current Focus
**DESIGN PHASE COMPLETE** - All feature designs documented, ready for implementation:

Latest update (2026-01-14): Created comprehensive feature designs for ALL missing FFmpeg features. See:
- docs/feature-designs.md - Full specification-grade designs for 15 new features
- docs/internal-design.md - Updated with implementation details (sections 18-25, Appendix C)
- docs/user-guide.md - Updated with user-facing APIs for all new features

All designs account for purego constraints while providing clean, idiomatic Go APIs.

### Implementation Status
- ✅ All internal packages (bindings, handles, platform, shim)
- ✅ Low-level bindings (avutil, avcodec, avformat, swscale)
- ✅ High-level API (Decoder, Encoder, Scaler, Frame wrapper)
- ✅ Custom I/O (NewDecoderFromReader, NewEncoderToWriter)
- ✅ Functional options (WithStreams, WithHWDevice, WithFormat)
- ✅ Error handling (IsEOF, IsAgain, FFmpegError)
- ✅ Logging (SetLogLevel, SetLogCallback)
- ✅ All examples working (decode, encode, transcode, custom-io)
- ✅ All tests passing with CGO_ENABLED=0

### Documentation Status
- ✅ README.md - Project overview, quick start, examples
- ✅ CONTRIBUTING.md - Contribution guidelines
- ✅ LICENSE - MIT License
- ✅ docs/user-guide.md - Complete API documentation
- ✅ docs/internal-design.md - Architecture specification
- ✅ docs/gap-analysis.md - Comprehensive FFmpeg vs ffgo feature comparison
- ✅ CLAUDE.md - Project memory (this file)

### API Compliance
- Duration() now returns time.Duration (user-guide compliant)
- Codec aliases (CodecH264, CodecAAC, etc.) implemented
- EncoderOptions with Video/Audio configs available
- NewEncoderToWriterWithOptions for io.Writer API
- All user-guide.md examples verified working

## Project Facts
- **Module path**: github.com/nonibytes/ffgo
- **Go version**: 1.23.0
- **Key dependency**: github.com/ebitengine/purego v0.9.1
- **FFmpeg versions supported**: 4.x - 7.x (tested with 6.x)
- **Platforms**: Linux/macOS/Windows on amd64/arm64 (no iOS/Android)

### Feature Coverage (vs Full FFmpeg)
See docs/gap-analysis.md for comprehensive comparison.

**What works well** (~95% coverage):
- Core video decode/encode/transcode pipeline
- Format conversion (MP4, MKV, AVI, MOV, etc.)
- Resolution/scaling changes with quality control
- Custom I/O integration (io.Reader/Writer)
- Basic seeking (backward keyframe)
- Hardware acceleration hooks (CUDA, VA-API, VideoToolbox)

**Partially implemented** (basic support only):
- Hardware acceleration (hooks exist, manual setup required)
- Metadata handling (low-level dict API only)
- Multi-stream handling (can read video OR audio)
- Codec options (basic bitrate/GOP only, no presets)

**Not yet implemented** (DESIGNS COMPLETE - see docs/feature-designs.md):
- Filter graphs (avfilter) - video effects, watermarking, overlays
- Audio resampling (swresample) - sample rate/channel conversion
- Subtitle support - decode/encode/render
- Device I/O (avdevice) - camera/screen capture
- Bitstream filters - packet-level transformations
- Advanced encoding (presets, multi-pass, CRF, profile/level control)
- Stream copy mode (fast remuxing without decode)
- Network protocols configuration (RTMP, RTSP, HLS)
- Audio encoding (stubbed, returns error)
- Hardware acceleration (complete implementation)
- Multi-stream muxing
- Advanced seeking (frame-accurate, byte position)
- Metadata/chapters handling
- Image sequences
- Color space handling

**Overall estimate**: ~35-40% current coverage, targeting 100% after implementation

**Implementation Priority** (from feature-designs.md):
- P0: Audio Encoding, Audio Resampling (required for transcoding)
- P1: Advanced Codec Options, Multi-Stream Muxing, Stream Copy
- P2: Filter Graphs, Hardware Acceleration, Advanced Seeking, Metadata
- P3: Subtitles, Bitstream Filters, Device I/O, Network Protocols
- P4: Image Sequences, Color Space

### Struct Offsets (FFmpeg 7.x / avutil 59.x)
These were verified using offsetof():
- **AVFrame**: data=0, linesize=64, width=104, height=108, nb_samples=112, format=116, key_frame=120, pts=136, sample_rate=216
- **AVFormatContext**: pb=32, oformat=16, nb_streams=44, streams=48, duration=72, bit_rate=80
- **AVStream**: index=8, id=12, codecpar=16, time_base=32, avg_frame_rate=88
- **AVCodecParameters**: codec_type=0, codec_id=4, format=28, width=56, height=60, sample_rate=116, channels=148 (ch_layout.nb_channels)
- **AVPacket**: pts=8, dts=16, data=24, size=32, stream_index=36
- **AVCodecContext**: codec_type=12, codec_id=24, bit_rate=56, flags=76, time_base=100, width=116, height=120, gop_size=132, pix_fmt=136, max_b_frames=160, framerate=704
- **AVOutputFormat**: flags=44

### Library Loading
- Must load in order: avutil → avcodec → avformat → swscale
- RTLD_GLOBAL flag is REQUIRED for FFmpeg cross-library references

## Decisions
- **Opaque pointers**: All FFmpeg types are unsafe.Pointer (no struct access by value on non-Darwin)
- **AVRational**: Pure Go implementation (shim provides wrappers if needed)
- **Callbacks**: Limited support due to purego's 2000 callback limit
- **Shim required**: For variadic functions like av_log (libffshim.so)

## Discovered Constraints
- purego panics on struct-by-value arguments/returns on Linux (only works on Darwin)
- String lifetime: use runtime.KeepAlive() after FFmpeg calls that might retain references
- Callback limit: purego has hard limit of 2000 callbacks that are never freed

## User Preferences
None recorded yet.

## Open Questions
None currently.

## TODOs for Production Release

### 1. Library Detection Improvements (CRITICAL)
**Location**: `internal/bindings/bindings.go` lines 70-91

**Issue**: Hardcoded version numbers are brittle and will fail with:
- FFmpeg 8.x (would need version 62+)
- Older FFmpeg 3.x (versions 55 or lower)
- Custom/alternative FFmpeg builds with different version schemes

**Current approach**:
```go
libAVUtil, err = loadLibrary("avutil", []int{59, 58, 57, 56})
libAVCodec, err = loadLibrary("avcodec", []int{61, 60, 59, 58})
libAVFormat, err = loadLibrary("avformat", []int{61, 60, 59, 58})
libSWScale, _ = loadLibrary("swscale", []int{8, 7, 6, 5})
```

**Proposed solutions**:
1. **Dynamic version detection**: Use `pkg-config` or similar to detect installed versions
2. **Wider version range**: Expand version arrays to include FFmpeg 3.x-8.x (versions 55-62+)
3. **Better fallback**: Improve unversioned library search (already implemented but could be prioritized)
4. **Runtime version check**: After loading, verify loaded version is compatible via `avutil_version()`

**Impact**: Users with non-standard FFmpeg installations will get "library not found" errors.

**Similar issue**: `internal/shim/shim.go` is more flexible (searches multiple names/paths) but could benefit from similar improvements.

---

### 2. Feature Implementation - All Designs Complete
**Location**: docs/feature-designs.md, docs/internal-design.md (sections 18-25)

**Status**: All feature designs are complete with specification-grade detail. Ready for implementation.

**Designed features** (see docs/feature-designs.md):
1. ✅ Audio Resampling (swresample) - package structure, API, error handling
2. ✅ Filter Graphs (avfilter) - graph API, common presets, complex filters
3. ✅ Audio Encoding - frame buffer, codec options
4. ✅ Advanced Codec Options - presets, CRF, profiles, rate control
5. ✅ Subtitle Support - decode, encode, burn-in rendering
6. ✅ Bitstream Filters - packet transformations
7. ✅ Stream Copy Mode - fast remuxing
8. ✅ Hardware Acceleration - device management, frame transfer
9. ✅ Multi-Stream Muxing - multiple audio/subtitle tracks
10. ✅ Advanced Seeking - frame-accurate, byte position, thumbnails
11. ✅ Metadata/Chapters - read/write metadata and chapter markers
12. ✅ Device I/O - camera/microphone/screen capture
13. ✅ Network Protocols - RTMP, HLS, RTSP configuration
14. ✅ Image Sequences - read/write image sequences
15. ✅ Color Space Handling - color space conversion

**Implementation priority**: See feature-designs.md for P0-P4 priority rankings

**Action**: Implement features following designs. All purego constraints documented.

---

### 3. Multi-Platform Shim Binaries (CRITICAL FOR RELEASE)
**Location**: `shim/` directory

**Issue**: Only one shim binary exists (`libffshim.so` for Linux amd64). This will NOT work for:
- **macOS**: Needs `.dylib` (Mach-O format, not ELF)
- **Windows**: Needs `.dll` (PE format)
- **ARM64**: Needs ARM64-specific binaries
- **Cross-platform distribution**: Users on other platforms cannot use logging features

**Current state**:
```
shim/
├── ffshim.c
├── ffshim.h
├── build.sh
└── libffshim.so          # Linux amd64 ONLY
```

**Required for full release**:
```
shim/
├── ffshim.c
├── ffshim.h
├── build.sh
└── prebuilt/
    ├── linux-amd64/
    │   └── libffshim.so
    ├── linux-arm64/
    │   └── libffshim.so
    ├── darwin-amd64/
    │   └── libffshim.dylib
    ├── darwin-arm64/
    │   └── libffshim.dylib
    ├── windows-amd64/
    │   └── ffshim.dll
    └── windows-arm64/
        └── ffshim.dll
```

**Solution approaches**:
1. **CI/CD builds**: Use GitHub Actions with matrix builds to compile for all platforms
2. **Cross-compilation**: Build all binaries from single host (complex, needs cross-compilers)
3. **User builds**: Provide clear build instructions for users to compile on their platform (current fallback)

**Code changes needed**:
- Update `internal/shim/shim.go` to search in `prebuilt/<os>-<arch>/` directories
- Update `build.sh` to support multiple platforms or provide per-platform build scripts
- Add installation instructions for placing shim in correct location

**Impact**:
- **Without multi-platform shims**: Logging doesn't work on macOS/Windows/ARM64
- **Shim is optional**: Core decode/encode/transcode still works without it
- **Priority**: HIGH for production release, MEDIUM for current development

**Workaround**: Users on non-Linux platforms must compile shim themselves from source using `build.sh`.

---

### 4. Version Detection Enhancement
**Location**: `internal/bindings/bindings.go`

**Related to TODO #1**: After loading libraries, should verify compatibility:

```go
func verifyFFmpegVersion() error {
    ver := avutilVersion()
    major := ver >> 16
    if major < 56 || major > 62 {
        return fmt.Errorf("unsupported FFmpeg version: avutil %d.x (need 56-62)", major)
    }
    return nil
}
```

This provides better error messages than "symbol not found" crashes.
