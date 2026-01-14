# Claude Memory - ffgo Project

## Current Focus
Core library implementation is complete. All features working:
- Decoder: NewDecoder, ReadFrame(), DecodeVideo(), DecodeAudio(), Seek(), SeekTime()
- Encoder: NewEncoder, WriteFrame(), Close()
- Scaler: NewScaler, Scale(), Close()
- Custom I/O: NewDecoderFromReader, NewDecoderFromIO, NewEncoderToWriter, NewEncoderFromIO
- Logging: SetLogLevel(), SetLogCallback()
- All examples working: decode, encode, transcode, custom-io

All tests pass with CGO_ENABLED=0.

## Project Facts
- **Module path**: github.com/nonibytes/ffgo
- **Go version**: 1.23.0
- **Key dependency**: github.com/ebitengine/purego v0.9.1
- **FFmpeg versions supported**: 4.x - 7.x (tested with 6.x)
- **Platforms**: Linux/macOS/Windows on amd64/arm64 (no iOS/Android)

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
