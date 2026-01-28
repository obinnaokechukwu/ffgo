# FFmpeg vs ffgo: Feature Gap Analysis

## ✅ Fully Implemented in ffgo

### Core Functionality
- **Video decoding** - H.264, HEVC, VP8, VP9, AV1, etc.
- **Video encoding** - H.264, HEVC (and others if the FFmpeg build provides an encoder)
- **Audio decoding** - AAC, MP3, Opus, etc.
- **Audio encoding** - AAC (other codecs are possible but encoder-specific requirements may need additional wiring)
- **Container demuxing** - MP4, MKV, AVI, MOV, etc.
- **Container muxing** - MP4, MKV, AVI, MOV, etc.
- **Pixel format conversion** - RGB ↔ YUV, format changes
- **Video scaling** - Resize, quality flags (bilinear, bicubic, lanczos)
- **Custom I/O** - io.Reader/Writer integration, custom callbacks
- **Error handling** - FFmpegError, IsEOF, IsAgain
- **Logging** - SetLogLevel, SetLogCallback (requires shim)

### Low-Level APIs Available
- `avutil` - Memory, frames, dictionaries, errors, HW device contexts
- `avcodec` - Encoding/decoding, codec contexts, bitstream filters
- `avformat` - Container formats, I/O, metadata
- `swscale` - Pixel format conversion, scaling
- `swresample` - Audio resampling, channel layout conversion
- `avfilter` - Filter graph processing

### High-Level APIs
- `Decoder` - Video/audio decoding from files or custom I/O
- `Encoder` - Video/audio encoding with full configuration
- `Muxer` / `MuxerStream` - Multi-stream muxing (encode or stream-copy)
- `Remuxer` - Stream copy/remux without re-encoding
- `Scaler` - Resolution/format conversion
- `Resampler` - Audio sample rate/format/channel conversion
- `VideoFilterGraph` / `AudioFilterGraph` - FFmpeg filter chains
- `HWDevice` / `HWDecoder` - Hardware accelerated decoding
- `SubtitleDecoder` - Text/bitmap subtitle extraction
- `BitstreamFilter` - Packet-level transformations

### Hardware Acceleration ✅
- CUDA, VAAPI, VideoToolbox, DXVA2, QSV support
- `NewHWDevice()` - Initialize hardware context
- `NewHWDecoder()` - Decode with hardware acceleration
- Automatic SW frame transfer option
- `AvailableHWDeviceTypes()` - List supported HW types

### Metadata Handling ✅
- `Decoder.GetMetadata()` / `Encoder.SetMetadata()` - Container metadata
- `GetStreamMetadata()` / `SetStreamMetadata()` - Per-stream metadata
- Common constants (MetadataTitle, MetadataArtist, etc.)

### Audio Resampling (swresample) ✅
- Sample rate conversion (44.1kHz → 48kHz, etc.)
- Channel layout changes (stereo → 5.1, mono → stereo)
- Sample format conversion (s16 → f32, planar ↔ packed)
- High-level `Resampler` type with automatic configuration

### Filter Graphs (avfilter) ✅
- `NewVideoFilterGraph()` - Video filters (scale, crop, overlay, etc.)
- `NewAudioFilterGraph()` - Audio filters (volume, equalizer, etc.)
- Complex filter chains ("scale=640:480,crop=320:240,hflip")
- Buffer source/sink abstraction

### Subtitle Support ✅
- `SubtitleDecoder` - Decode SRT, ASS, WebVTT, bitmap subtitles
- `Decoder.HasSubtitle()` / `Decoder.SubtitleStream()` - Detection
- Text and ASS subtitle parsing
- Bitmap subtitle rectangle extraction

### Bitstream Filters ✅
- `BitstreamFilter` - Packet-level transformations
- h264_mp4toannexb, hevc_mp4toannexb, aac_adtstoasc
- extract_extradata, dump_extra, remove_extra
- `BitstreamFilterExists()` - Check availability

### Advanced Seeking ✅
- `SeekPrecise()` - Frame-accurate seeking (decode from keyframe)
- `SeekToFrame()` - Seek to specific frame number
- `SeekKeyframe()` / `SeekAny()` / `SeekByBytes()` - Low-level seek options
- `ExtractThumbnail()` / `ExtractThumbnails()` - Thumbnail generation
- `TotalFrames()` - Frame count estimation

### Stream Copy ✅
- `Remuxer` - Copy streams without re-encoding (fast remuxing)
- `EncoderOptions.CopyVideo` / `EncoderOptions.CopyAudio` - Stream copy mode via encoder output
- `Muxer.AddCopyStream` - Stream copy mode in the muxer API

### Advanced Codec Options ✅
- Presets, CRF, profile/level, tune, and rate-control via `VideoEncoderConfig` fields (plus `CodecOptions` for encoder-specific knobs)

## ⚠️ Partially Implemented / Limited

### Network Protocols
**Status**: ✅ Supported (via FFmpeg)
- Can decode from URLs (http://, rtmp://, etc.)
- Protocol configuration is available via `NewNetworkDecoder` and `ProtocolOptions` (timeouts, reconnect, headers, TLS verify)
- Streaming output helpers are not a primary focus (use FFmpeg muxers/protocols directly when needed)

### Format-Specific Features
**Partially covered / missing**:
- Multi-program streams (MPEG-TS programs) (no dedicated high-level helpers)
- Data streams (arbitrary data tracks) (no dedicated high-level helpers)

### Concatenation/Segmentation
**Status**: ✅ Segmentation implemented; concat helpers missing
- ✅ HLS segment generation (via `NewHLSSegmenter` + `Muxer.WriteHeaderWithOptions`)
- ✅ DASH manifest/segment generation (via `NewDASHSegmenter` + `Muxer.WriteHeaderWithOptions`)
- ❌ Concat demuxer helpers (no dedicated API)

### Color Space/Range Handling
**Status**: ✅ Basic support
- `Frame.ColorSpec()` / `Frame.SetColorSpec()` for color metadata
- `Scaler.SetColorConversion(...)` supports range handling (limited/full) when swscale exposes colorspace APIs
- Explicit BT.601/BT.709/BT.2020 matrix selection is not exposed as a high-level helper

### Image Sequence Handling
**Status**: ✅ Implemented
- Supports printf-style sequence patterns (e.g. `frame_%04d.png`) via `image2`
- Supports frame timing via the `framerate` option

## ❌ Not Implemented

### Device Input/Output (avdevice)
**FFmpeg capability**: Capture from devices
- Camera capture (V4L2, AVFoundation, DirectShow)
- Screen capture (x11grab/gdigrab/etc.)
- Audio device input

**ffgo status**: ⚠️ Partially implemented
- ✅ `NewCapture` / `CaptureScreen` exist and use FFmpeg device demuxers
- ✅ Automatically loads `libavdevice` when capture APIs are used
- ✅ `ListDevices` / `ListDevicesWithOptions` implemented (requires libavdevice + shim helper)
- ⚠️ Capture is environment-dependent (requires FFmpeg built with device support + OS permissions)

### Multi-Pass Encoding
**FFmpeg capability**: Two-pass VBR for optimal quality/size
- Statistics collection in first pass
- Optimal bitrate distribution in second pass

**ffgo status**: ✅ Implemented
- `TwoPassTranscode` helper + `EncoderOptions.Pass*`
- Uses `AV_CODEC_FLAG_PASS1/PASS2` + `passlogfile`/`stats` options (x264/x265 supported on typical builds)

### Advanced Format Probing
**FFmpeg capability**: Detailed format detection
- Probe score analysis
- Multiple format attempts
- Stream-level probing options

**ffgo status**: ✅ Implemented
- Typed probe controls in `DecoderOptions` (ProbeSizeBytes, AnalyzeDuration, MaxProbePackets, whitelist fields)

## Summary Statistics

| Category | Status | Notes |
|----------|--------|-------|
| Core decode/encode | ✅ Full | Video + Audio |
| Container mux/demux | ✅ Full | All major formats |
| Scaling/conversion | ✅ Full | swscale wrapped |
| Audio resampling | ✅ Full | swresample wrapped |
| Filter graphs | ✅ Full | avfilter wrapped |
| Custom I/O | ✅ Full | Reader/Writer + callbacks |
| Hardware acceleration | ✅ Full | CUDA, VAAPI, etc. |
| Metadata | ✅ Full | Container + stream metadata |
| Subtitles | ✅ Full | Text + bitmap |
| Bitstream filters | ✅ Full | Packet transformations |
| Advanced seeking | ✅ Full | Frame-accurate + thumbnails |
| Stream copy | ✅ Full | Fast remuxing |
| Advanced encoding | ✅ Full | Presets, CRF, profiles |
| Network protocols | ✅ Full | `NewNetworkDecoder` + `ProtocolOptions` |
| Device capture | ⚠️ Partial | Requires libavdevice + OS permissions (environment-dependent) |
| Multi-pass encoding | ✅ Full | Two-pass helper (x264/x265 supported on typical builds) |

## Capability Percentages

- **Basic video transcode pipeline**: 100% ✅
- **Production encoding**: 90% ✅
- **Advanced video processing**: 85% ✅
- **Audio processing**: 85% ✅
- **Streaming/live**: 50% ⚠️ (via FFmpeg protocols)
- **Professional workflows**: 80% ✅

**Overall FFmpeg capability coverage**: ~80%+

## What Works Well in ffgo

- ✅ Decode → process → encode workflows
- ✅ Format conversion (MP4 → MKV, etc.)
- ✅ Full transcoding with quality control
- ✅ Custom I/O integration
- ✅ Resolution/scaling/filtering
- ✅ Hardware-accelerated decoding
- ✅ Audio resampling and mixing
- ✅ Filter graph processing
- ✅ Subtitle extraction
- ✅ Metadata handling
- ✅ Stream copying (fast remux)
- ✅ Pure Go builds, cross-compilation

## What Requires External Tools

- ⚠️ Device capture setup can be OS/permission dependent (FFmpeg must be built with libavdevice and the process must have permissions)
- ❌ Concat workflows (no dedicated helpers)
