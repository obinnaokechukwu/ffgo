# ffgo

[![Go Reference](https://pkg.go.dev/badge/github.com/nonibytes/ffgo.svg)](https://pkg.go.dev/github.com/nonibytes/ffgo)
[![Go Report Card](https://goreportcard.com/badge/github.com/nonibytes/ffgo)](https://goreportcard.com/report/github.com/nonibytes/ffgo)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

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
go get github.com/nonibytes/ffgo
```

### Decode a video

```go
package main

import (
    "fmt"
    "github.com/nonibytes/ffgo"
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
            if err := encoder.WriteVideoFrame(frame); err != nil {
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
| Windows | amd64, arm64 | ✅ Fully supported |
| FreeBSD | amd64, arm64 | ⚠️ Best-effort |

**Not supported**: iOS, Android, 32-bit systems

## Documentation

- **[User Guide](docs/user-guide.md)** - Complete API documentation with examples
- **[Internal Design](docs/internal-design.md)** - Architecture and implementation details
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

A tiny C shim library (~10KB) handles FFmpeg's variadic logging functions, which pure Go cannot call. Without it, logging won't work, but decoding/encoding will.

## Examples

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
scaledFrame, err := scaler.Scale(frame)
```

### Hardware acceleration

```go
decoder, err := ffgo.NewDecoder("input.mp4",
    ffgo.WithHWDevice("cuda"),  // NVIDIA GPU
)
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

## Roadmap

- [x] Core decode/encode/transcode
- [x] Custom I/O support
- [x] Hardware acceleration hooks
- [x] Pixel format conversion
- [ ] Audio resampling
- [ ] Filter graphs
- [ ] Subtitle support

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

MIT License - see [LICENSE](LICENSE) file.

FFmpeg is licensed under LGPL/GPL. Ensure your usage complies with FFmpeg's license.

## Credits

- Built with [purego](https://github.com/ebitengine/purego) by Hajime Hoshi
- Inspired by the FFmpeg project and community

## Support

- **Issues**: [GitHub Issues](https://github.com/nonibytes/ffgo/issues)
- **Discussions**: [GitHub Discussions](https://github.com/nonibytes/ffgo/discussions)
- **Documentation**: [docs/user-guide.md](docs/user-guide.md)
