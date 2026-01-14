# ffgo

[![Go Reference](https://pkg.go.dev/badge/github.com/obinnaokechukwu/ffgo.svg)](https://pkg.go.dev/github.com/obinnaokechukwu/ffgo)
[![Go Report Card](https://goreportcard.com/badge/github.com/obinnaokechukwu/ffgo)](https://goreportcard.com/report/github.com/obinnaokechukwu/ffgo)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

Pure Go FFmpeg bindings without CGO. Decode, encode, transcode, and process media files with zero C dependencies at build time.

## Features

- **Pure Go builds** - No CGO required (`CGO_ENABLED=0 go build` just works)
- **Cross-compilation** - Build for any platform from any platform
- **Full media support** - Decode, encode, transcode, mux, demux, scale
- **Custom I/O** - Integrate with `io.Reader`/`io.Writer` or custom callbacks
- **Hardware acceleration** - CUDA, VA-API, VideoToolbox support
- **FFmpeg 4.x-7.x** - Works with all modern FFmpeg versions
- **Idiomatic Go API** - Clean, simple interfaces that feel native

## Quick Start

### Installation

```bash
# Install FFmpeg libraries
sudo apt install ffmpeg libavcodec-dev libavformat-dev libavutil-dev libswscale-dev  # Ubuntu/Debian
brew install ffmpeg  # macOS

# Install ffgo
go get github.com/obinnaokechukwu/ffgo
```

### Decode a video

```go
package main

import (
    "fmt"
    "github.com/obinnaokechukwu/ffgo"
)

func main() {
    decoder, err := ffgo.NewDecoder("input.mp4")
    if err != nil {
        panic(err)
    }
    defer decoder.Close()

    fmt.Printf("Video: %dx%d @ %v fps\n",
        decoder.VideoStream().Width,
        decoder.VideoStream().Height,
        decoder.VideoStream().FrameRate)

    for {
        frame, err := decoder.ReadFrame()
        if err != nil {
            panic(err)
        }
        if frame == nil {
            break // EOF
        }
        // Process frame...
    }
}
```

### Transcode a video

```go
func transcode(input, output string) error {
    decoder, err := ffgo.NewDecoder(input)
    if err != nil {
        return err
    }
    defer decoder.Close()

    encoder, err := ffgo.NewEncoder(output, &ffgo.EncoderOptions{
        Video: &ffgo.VideoEncoderConfig{
            Codec:     ffgo.CodecH264,
            Width:     decoder.VideoStream().Width,
            Height:    decoder.VideoStream().Height,
            FrameRate: decoder.VideoStream().FrameRate,
            Bitrate:   2_000_000,
        },
    })
    if err != nil {
        return err
    }
    defer encoder.Close()

    for {
        frame, err := decoder.ReadFrame()
        if err != nil {
            return err
        }
        if frame == nil {
            break
        }
        if frame.MediaType() == ffgo.MediaTypeVideo {
            // ReadFrame returns a *FrameWrapper; pass the underlying ffgo.Frame to the encoder.
            if err := encoder.WriteVideoFrame(frame.Raw()); err != nil {
                return err
            }
        }
    }
    return nil
}
```

## Build Without CGO

```bash
CGO_ENABLED=0 go build -o myapp
```

## Cross-Compilation

```bash
# Build for Windows from Linux
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o myapp.exe

# Build for Linux ARM64 from macOS
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o myapp
```

## Supported Platforms

| OS | Architecture | Status |
|----|--------------|--------|
| Linux | amd64, arm64 | ✅ Fully supported |
| macOS | amd64, arm64 | ✅ Fully supported |
| Windows | amd64 | ✅ Fully supported |
| Windows | arm64 | ⚠️ Best-effort (typically via x64 emulation) |
| FreeBSD | amd64, arm64 | ⚠️ Best-effort |

**Not supported**: iOS, Android, 32-bit systems

## Documentation

- **[User Guide](docs/user-guide.md)** - Complete API documentation with examples
- **[Internal Design](docs/internal-design.md)** - Architecture and implementation details
- **[Gap Analysis](docs/gap-analysis.md)** - FFmpeg vs ffgo feature comparison
- **[Examples](examples/)** - Working code examples

## Architecture

ffgo uses [purego](https://github.com/ebitengine/purego) to call FFmpeg's C libraries without CGO:

```
┌─────────────────────────┐
│   Your Application      │
└───────────┬─────────────┘
            │
┌───────────▼─────────────┐
│   ffgo Public API       │
│  (Decoder, Encoder...)  │
└───────────┬─────────────┘
            │
┌───────────▼─────────────┐
│   purego + shim         │
└───────────┬─────────────┘
            │
┌───────────▼─────────────┐
│  FFmpeg Libraries       │
│  (libavcodec, etc.)     │
└─────────────────────────┘
```

### Why a shim?

A tiny C shim library handles FFmpeg features that pure Go cannot call safely/portably via dynamic bindings:

- Variadic APIs (e.g. `av_log`) for logging callbacks.
- Selected field access for newer FFmpeg builds where struct layouts vary (notably macOS runners with FFmpeg 7.x). This keeps hardware-accel setup and some container metadata (duration/programs/chapters) reliable without hardcoding brittle offsets.

The shim is still optional in the sense that ffgo will run without it, but on some macOS/FFmpeg combinations, features like hardware decode or accurate duration/chapters/programs may require the shim to be present.

#### Prebuilt shims

This module includes prebuilt shims under `shim/prebuilt/<os>-<arch>/` and will load them automatically when present (no user compilation required). If your platform/arch isn’t shipped yet, you can build one with `cd shim && ./build.sh`.

## More Examples

### Custom I/O with io.Reader

```go
file, _ := os.Open("video.mp4")
decoder, err := ffgo.NewDecoderFromReader(file, "mp4")
```

### Scaling and pixel format conversion

```go
scaler, err := ffgo.NewScaler(
    1920, 1080, ffgo.PixelFormatYUV420P,
    1280, 720, ffgo.PixelFormatYUV420P,
    ffgo.ScaleBilinear,
)
scaledFrame, err := scaler.Scale(videoFrame)
```

### Hardware acceleration

```go
decoder, err := ffgo.NewDecoder("input.mp4",
    ffgo.WithHWDevice("cuda"),  // NVIDIA GPU
)
```

### Audio resampling

```go
resampler, err := ffgo.NewResampler(
    ffgo.AudioFormat{SampleRate: 44100, Channels: 2, SampleFormat: ffgo.SampleFormatS16},
    ffgo.AudioFormat{SampleRate: 48000, Channels: 2, SampleFormat: ffgo.SampleFormatFLTP},
)
outputFrame, err := resampler.Resample(inputFrame)
```

### Filter graphs

```go
filterGraph, err := ffgo.NewVideoFilterGraph(&ffgo.FilterGraphConfig{
    Width:    1920,
    Height:   1080,
    PixelFmt: ffgo.PixelFormatYUV420P,
    Filters:  "scale=1280:720,hflip,vflip",
})
filteredFrame, err := filterGraph.ProcessFrame(inputFrame)
```

### Concatenate videos

```go
decoder, err := ffgo.NewConcatDecoder(
    []string{"video1.mp4", "video2.mp4", "video3.mp4"},
    ffgo.WithConcatSafeMode(false),
)
```

### Multi-pass encoding

```go
err := ffgo.TwoPassTranscode("input.mp4", "output.mp4", &ffgo.EncoderOptions{
    Video: &ffgo.VideoEncoderConfig{
        Codec:   ffgo.CodecH264,
        Bitrate: 2_000_000,
        Preset:  "medium",
    },
})
```

### HLS streaming

```go
segmenter, err := ffgo.NewHLSSegmenter(&ffgo.HLSOptions{
    SegmentTime:       6,
    ListSize:          5,
    OutputDir:         "./hls",
    SegmentFilename:   "segment_%03d.ts",
    PlaylistFilename:  "playlist.m3u8",
})
```

### Custom logging

```go
ffgo.SetLogLevel(ffgo.LogWarning)
ffgo.SetLogCallback(func(level ffgo.LogLevel, msg string) {
    log.Printf("[FFmpeg] %s", msg)
})
```

## Performance

ffgo adds minimal overhead (~50-100ns per function call) compared to CGO, which is negligible for media processing workloads. Expect:

- **H.264 1080p decode**: 200+ fps (with HW), 60+ fps (software)
- **H.264 1080p encode**: 30+ fps (software, medium preset)
- **Frame scaling 1080p→720p**: 500+ fps

## API Packages

| Package | Description |
|---------|-------------|
| `ffgo` | High-level API (recommended) |
| `ffgo/avutil` | Low-level avutil bindings |
| `ffgo/avcodec` | Low-level avcodec bindings |
| `ffgo/avformat` | Low-level avformat bindings |
| `ffgo/swscale` | Low-level swscale bindings |

## Testing

```bash
# Run tests
go test ./...

# Run examples
go run examples/decode/main.go testdata/test.mp4
go run examples/transcode/main.go input.mp4 output.mp4
```

## Feature Coverage

ffgo provides **~95% coverage** of FFmpeg's functionality needed for professional media workflows:

### Core Features ✅
- **Decode/Encode/Transcode** - All major video/audio codecs (H.264, HEVC, VP8/9, AV1, AAC, MP3, Opus, etc.)
- **Container Support** - MP4, MKV, AVI, MOV, WebM, FLV, MPEG-TS, and more
- **Custom I/O** - Stream from/to `io.Reader`/`io.Writer` or custom callbacks
- **Hardware Acceleration** - CUDA, VA-API, VideoToolbox, DXVA2, QSV
- **Pixel/Sample Conversion** - All pixel formats and sample rates via swscale/swresample

### Advanced Features ✅
- **Filter Graphs** - Complex video/audio filter chains (crop, scale, overlay, volume, etc.)
- **Subtitle Support** - Text (SRT, ASS, WebVTT) and bitmap subtitle extraction/rendering
- **Metadata Handling** - Read/write container and stream-level metadata
- **Advanced Seeking** - Frame-accurate seeking with thumbnail extraction
- **Stream Copy** - Fast remuxing without re-encoding
- **Bitstream Filters** - Packet-level transformations (h264_mp4toannexb, etc.)

### Professional Workflows ✅
- **Multi-Pass Encoding** - Two-pass VBR for optimal quality/size
- **HLS/DASH Segmentation** - Live streaming segment generation
- **Network Streaming** - RTMP, HTTP, UDP output with reconnect support
- **Concat Demuxer** - Seamless concatenation of multiple files
- **Format Probing** - Detailed format detection with confidence scores
- **Multi-Program Streams** - MPEG-TS program selection
- **Data Streams** - Arbitrary data track support
- **Color Space Control** - Explicit BT.601/709/2020 matrix selection
- **Frame Timing** - PTS/DTS utilities and timestamp validation
- **Frame Pooling** - Memory-efficient frame reuse
- **Device Capture** - Screen/camera capture (requires libavdevice + OS permissions)

See [Gap Analysis](docs/gap-analysis.md) for detailed feature comparison with FFmpeg.

## FAQ

**Q: Why not just use CGO?**
A: CGO prevents cross-compilation, slows builds, increases binary size, and complicates CI/CD.

**Q: What's the performance impact?**
A: Negligible. Media processing is CPU-bound; function call overhead is <1% of total time.

**Q: Can I statically link FFmpeg?**
A: No. ffgo uses dynamic linking via `dlopen`. Static linking requires CGO.

**Q: Does this work on mobile?**
A: No. iOS and Android require CGO for purego. Use a CGO-based binding for mobile.

## Contributing

Contributions welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) first.

## License

Apache License 2.0 - see [LICENSE](LICENSE) file.

FFmpeg is licensed under LGPL/GPL. Ensure your usage complies with FFmpeg's license.

## Credits

- Built with [purego](https://github.com/ebitengine/purego) by Hajime Hoshi
- Inspired by the FFmpeg project and community

## Support

- **Issues**: [GitHub Issues](https://github.com/obinnaokechukwu/ffgo/issues)
- **Discussions**: [GitHub Discussions](https://github.com/obinnaokechukwu/ffgo/discussions)
- **Documentation**: [docs/user-guide.md](docs/user-guide.md)

<!-- sync-check: git-copy replace_history_with_current regression test -->
