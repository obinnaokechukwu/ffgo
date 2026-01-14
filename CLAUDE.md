# Claude Memory - ffgo Project

## Current Focus
**IMPLEMENTATION COMPLETE** - All major FFmpeg features implemented and tested.

Latest update (2026-01-14): Completed implementation of all planned features:
- Audio resampling (swresample package)
- Filter graphs (avfilter package)
- Advanced codec options (presets, CRF, profiles, rate control)
- Stream copy mode (Remuxer)
- Metadata handling (GetMetadata, SetMetadata)
- Hardware acceleration (HWDevice, HWDecoder)
- Advanced seeking (SeekPrecise, SeekToFrame, thumbnails)
- Subtitle support (SubtitleDecoder)
- Bitstream filters (BitstreamFilter)

### Implementation Status
- ✅ All internal packages (bindings, handles, platform, shim)
- ✅ Low-level bindings (avutil, avcodec, avformat, swscale, swresample, avfilter)
- ✅ High-level API (Decoder, Encoder, Scaler, Resampler, FilterGraph, HWDecoder, SubtitleDecoder, BitstreamFilter)
- ✅ Custom I/O (NewDecoderFromReader, NewEncoderToWriter)
- ✅ Functional options (presets, CRF, profiles, hardware device, format)
- ✅ Error handling (IsEOF, IsAgain, FFmpegError)
- ✅ Logging (SetLogLevel, SetLogCallback)
- ✅ All examples working (decode, encode, transcode, custom-io)
- ✅ All 52+ tests passing with CGO_ENABLED=0

### Test Coverage
All packages tested:
- ffgo (main): 52 tests
- avcodec: Tests passing
- avfilter: Tests passing
- avformat: Tests passing
- avutil: Tests passing
- swscale: Tests passing
- internal/*: Tests passing

## Project Facts
- **Module path**: github.com/nonibytes/ffgo
- **Go version**: 1.23.0
- **Key dependency**: github.com/ebitengine/purego v0.9.1
- **FFmpeg versions supported**: 4.x - 7.x (tested with 6.x)
- **Platforms**: Linux/macOS/Windows on amd64/arm64 (no iOS/Android)

### Feature Coverage (~75-80% of FFmpeg)

**Fully Implemented**:
- Core video decode/encode/transcode
- Audio decode/encode with resampling
- Container mux/demux (MP4, MKV, AVI, MOV, etc.)
- Video scaling and pixel format conversion
- Audio resampling (sample rate, channels, format)
- Filter graphs (video and audio filters)
- Custom I/O (io.Reader/Writer + callbacks)
- Hardware acceleration (CUDA, VAAPI, VideoToolbox, DXVA2, QSV)
- Metadata handling (container and stream level)
- Advanced seeking (frame-accurate, thumbnails)
- Subtitle extraction (SRT, ASS, bitmap)
- Bitstream filters (packet transformations)
- Stream copy mode (fast remuxing)
- Advanced codec options (presets, CRF, profiles)

**Partially Implemented**:
- Network protocols (via FFmpeg, no custom helpers)

**Not Implemented**:
- Device I/O (avdevice) - webcam/screen capture
- Multi-pass encoding
- HLS/DASH segmentation

### Struct Offsets (FFmpeg 6.x/7.x)
Verified using offsetof():
- **AVFrame**: data=0, linesize=64, width=104, height=108, nb_samples=112, format=116, key_frame=120, pts=136
- **AVFormatContext**: pb=32, oformat=16, nb_streams=44, streams=48, duration=72, bit_rate=80, metadata=176
- **AVStream**: index=8, codecpar=16, time_base=32, metadata=80
- **AVCodecParameters**: codec_type=0, codec_id=4, format=28, width=56, height=60, sample_rate=116
- **AVPacket**: pts=8, dts=16, data=24, size=32, stream_index=36
- **AVCodecContext**: codec_type=12, codec_id=24, bit_rate=56, width=116, height=120, pix_fmt=136, hw_device_ctx=864
- **AVBSFContext**: par_in=24, par_out=32, time_base_in=40, time_base_out=48
- **AVSubtitle**: format=0, start_display_time=4, end_display_time=8, num_rects=12, rects=16, pts=24

### Library Loading
- Must load in order: avutil → avcodec → avformat → swscale → swresample → avfilter
- RTLD_GLOBAL flag is REQUIRED for FFmpeg cross-library references

## Decisions
- **Opaque pointers**: All FFmpeg types are unsafe.Pointer (no struct access by value on non-Darwin)
- **AVRational**: Pure Go implementation
- **Callbacks**: Limited support due to purego's 2000 callback limit
- **Shim required**: For variadic functions like av_log (libffshim.so)

## Discovered Constraints
- purego panics on struct-by-value arguments/returns on Linux (only works on Darwin)
- String lifetime: use runtime.KeepAlive() after FFmpeg calls that might retain references
- Callback limit: purego has hard limit of 2000 callbacks
- BSF function parameter order: av_bsf_alloc takes (filter, &ctx) not (&ctx, filter)

## User Preferences
None recorded yet.

## Open Questions
None currently.

## Remaining TODOs for Production Release

### 1. Multi-Platform Shim Binaries
**Location**: `shim/` directory

Only Linux amd64 shim exists. Need:
- macOS (amd64/arm64): `libffshim.dylib`
- Windows (amd64/arm64): `ffshim.dll`
- Linux arm64: `libffshim.so`

**Workaround**: Users can compile shim from source using `build.sh`.
**Impact**: Logging doesn't work on other platforms, but core functionality works.

### 2. Library Detection Improvements
**Location**: `internal/bindings/bindings.go`

Could improve with:
- Dynamic version detection via pkg-config
- Runtime version verification
- Better error messages for unsupported versions

### 3. Device I/O (Optional Enhancement)
**Not implemented**: Camera/microphone/screen capture (avdevice)
**Workaround**: Use OS-specific APIs separately

### 4. Multi-Pass Encoding (Optional Enhancement)
**Not implemented**: Two-pass VBR encoding
**Workaround**: Use CRF-based quality encoding
