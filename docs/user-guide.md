# ffgo User Guide

**ffgo** is a pure Go FFmpeg binding library. It lets you decode, encode, transcode, and process media files without requiring CGO.

---

## Table of Contents

1. [Installation](#installation)
2. [Supported Platforms](#supported-platforms)
3. [Quick Start](#quick-start)
4. [Decoding Media](#decoding-media)
5. [Encoding Media](#encoding-media)
6. [Transcoding](#transcoding)
7. [Scaling and Format Conversion](#scaling-and-format-conversion)
8. [Custom I/O](#custom-io)
9. [Error Handling](#error-handling)
10. [Logging](#logging)
11. [Low-Level API](#low-level-api)
12. [FAQ](#faq)

---

## Installation

### Prerequisites

Install FFmpeg development libraries on your system:

**Ubuntu/Debian**:
```bash
sudo apt install ffmpeg libavcodec-dev libavformat-dev libavutil-dev libswscale-dev
```

**macOS**:
```bash
brew install ffmpeg
```

**Windows (MSYS2)**:
```bash
pacman -S mingw-w64-x86_64-ffmpeg
```

### Install ffgo

```bash
go get github.com/obinnaokechukwu/ffgo
```

### Install the Helper Library

ffgo requires a small helper library for logging support:

```bash
# Download pre-built binary (recommended)
ffgo-install-shim

# Or build from source
cd $(go env GOMODCACHE)/github.com/obinnaokechukwu/ffgo@latest/shim
./build.sh
sudo cp libffshim.so /usr/local/lib/  # Linux
# or: sudo cp libffshim.dylib /usr/local/lib/  # macOS
```

---

## Supported Platforms

| OS | Architecture | Status |
|----|--------------|--------|
| Linux | amd64, arm64 | Fully supported |
| macOS | amd64 (Intel), arm64 (Apple Silicon) | Fully supported |
| Windows | amd64, arm64 | Fully supported |
| FreeBSD | amd64, arm64 | Best-effort |

**Not Supported**: iOS, Android, 32-bit systems

---

## Quick Start

### Decode a Video File

```go
package main

import (
    "fmt"
    "github.com/obinnaokechukwu/ffgo"
)

func main() {
    // Open video file
    decoder, err := ffgo.NewDecoder("input.mp4")
    if err != nil {
        panic(err)
    }
    defer decoder.Close()

    // Print stream info
    fmt.Printf("Video: %dx%d @ %v fps\n",
        decoder.VideoStream().Width,
        decoder.VideoStream().Height,
        decoder.VideoStream().FrameRate)

    // Read frames
    for {
        frame, err := decoder.ReadFrame()
        if err != nil {
            panic(err)
        }
        if frame == nil {
            break // End of file
        }

        // Process frame...
        fmt.Printf("Frame PTS: %v\n", frame.PTS())
    }
}
```

### Build Without CGO

```bash
CGO_ENABLED=0 go build -o myapp
```

### Cross-Compile

```bash
# Build for Windows from Linux
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o myapp.exe

# Build for Linux ARM64 from macOS
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o myapp
```

**Note**: The target system must have FFmpeg libraries installed.

---

## Decoding Media

### Open a File

```go
decoder, err := ffgo.NewDecoder("video.mp4")
if err != nil {
    return err
}
defer decoder.Close()
```

### Get Stream Information

```go
// Video stream
video := decoder.VideoStream()
if video != nil {
    fmt.Printf("Video: %s, %dx%d, %v fps\n",
        video.CodecName,
        video.Width,
        video.Height,
        video.FrameRate)
}

// Audio stream
audio := decoder.AudioStream()
if audio != nil {
    fmt.Printf("Audio: %s, %d Hz, %d channels\n",
        audio.CodecName,
        audio.SampleRate,
        audio.Channels)
}

// Duration
fmt.Printf("Duration: %v\n", decoder.Duration())
```

### Read Frames

```go
for {
    frame, err := decoder.ReadFrame()
    if err != nil {
        return err
    }
    if frame == nil {
        break // EOF
    }

    switch frame.MediaType() {
    case ffgo.MediaTypeVideo:
        // Process video frame
        pixels := frame.Data(0) // Y plane for YUV

    case ffgo.MediaTypeAudio:
        // Process audio frame
        samples := frame.Data(0)
    }
}
```

### Seek

```go
// Seek to 30 seconds
err := decoder.Seek(30 * time.Second)
if err != nil {
    return err
}

// Continue reading from new position
frame, err := decoder.ReadFrame()
```

### Decode Only Video or Audio

```go
// Skip audio, decode only video
decoder, err := ffgo.NewDecoder("video.mp4",
    ffgo.WithStreams(ffgo.MediaTypeVideo))
```

---

## Encoding Media

### Create an Encoder

```go
encoder, err := ffgo.NewEncoder("output.mp4", &ffgo.EncoderOptions{
    Video: &ffgo.VideoEncoderConfig{
        Codec:     ffgo.CodecH264,
        Width:     1920,
        Height:    1080,
        FrameRate: ffgo.NewRational(30, 1),
        Bitrate:   5_000_000, // 5 Mbps
    },
    Audio: &ffgo.AudioEncoderConfig{
        Codec:      ffgo.CodecAAC,
        SampleRate: 48000,
        Channels:   2,
        Bitrate:    128_000,
    },
})
if err != nil {
    return err
}
defer encoder.Close()
```

### Write Frames

```go
// Write video frame
err := encoder.WriteVideoFrame(frame)
if err != nil {
    return err
}

// Write audio frame
err = encoder.WriteAudioFrame(audioFrame)
if err != nil {
    return err
}
```

### Finalize Output

```go
// Flush encoder and write trailer
err := encoder.Close()
if err != nil {
    return err
}
```

---

## Transcoding

### Basic Transcode

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
            err = encoder.WriteVideoFrame(frame)
            if err != nil {
                return err
            }
        }
    }

    return nil
}
```

### Transcode with Scaling

```go
scaler, err := ffgo.NewScaler(
    decoder.VideoStream().Width,
    decoder.VideoStream().Height,
    decoder.VideoStream().PixelFormat,
    1280, 720, // Output size
    ffgo.PixelFormatYUV420P,
    ffgo.ScaleBilinear,
)
if err != nil {
    return err
}
defer scaler.Close()

for {
    frame, err := decoder.ReadFrame()
    if err != nil {
        return err
    }
    if frame == nil {
        break
    }

    if frame.MediaType() == ffgo.MediaTypeVideo {
        scaled, err := scaler.Scale(frame)
        if err != nil {
            return err
        }
        err = encoder.WriteVideoFrame(scaled)
        if err != nil {
            return err
        }
    }
}
```

---

## Scaling and Format Conversion

### Create a Scaler

```go
scaler, err := ffgo.NewScaler(
    srcWidth, srcHeight, srcPixelFormat,
    dstWidth, dstHeight, dstPixelFormat,
    ffgo.ScaleBicubic, // Quality flag
)
if err != nil {
    return err
}
defer scaler.Close()
```

### Scale Flags

| Flag | Description |
|------|-------------|
| `ScaleFastBilinear` | Fast, lower quality |
| `ScaleBilinear` | Good balance |
| `ScaleBicubic` | High quality |
| `ScaleLanczos` | Highest quality, slowest |

### Convert Pixel Format

```go
// Convert RGB24 to YUV420P
scaler, err := ffgo.NewScaler(
    width, height, ffgo.PixelFormatRGB24,
    width, height, ffgo.PixelFormatYUV420P,
    ffgo.ScaleBilinear,
)
```

---

## Custom I/O

### Read from io.Reader

```go
file, err := os.Open("video.mp4")
if err != nil {
    return err
}
defer file.Close()

decoder, err := ffgo.NewDecoderFromReader(file, "mp4")
if err != nil {
    return err
}
defer decoder.Close()
```

### Read from HTTP

```go
resp, err := http.Get("https://example.com/video.mp4")
if err != nil {
    return err
}
defer resp.Body.Close()

decoder, err := ffgo.NewDecoderFromReader(resp.Body, "mp4")
if err != nil {
    return err
}
defer decoder.Close()
```

### Write to io.Writer

```go
file, err := os.Create("output.mp4")
if err != nil {
    return err
}
defer file.Close()

encoder, err := ffgo.NewEncoderToWriter(file, "mp4", &ffgo.EncoderOptions{
    Video: &ffgo.VideoEncoderConfig{
        Codec: ffgo.CodecH264,
        Width: 1920,
        Height: 1080,
    },
})
if err != nil {
    return err
}
defer encoder.Close()
```

### Custom I/O Callbacks

```go
decoder, err := ffgo.NewDecoderFromIO(&ffgo.IOCallbacks{
    Read: func(buf []byte) (int, error) {
        // Read from your source
        return mySource.Read(buf)
    },
    Seek: func(offset int64, whence int) (int64, error) {
        // Seek in your source
        return mySource.Seek(offset, whence)
    },
}, "mp4")
```

---

## Error Handling

### Check Error Types

```go
frame, err := decoder.ReadFrame()
if err != nil {
    if ffgo.IsEOF(err) {
        // End of file - not an error
        return nil
    }
    return err
}
```

### FFmpeg Errors

```go
err := decoder.Seek(time.Hour) // Seek past end
if err != nil {
    var ffErr *ffgo.FFmpegError
    if errors.As(err, &ffErr) {
        fmt.Printf("FFmpeg error code: %d\n", ffErr.Code)
        fmt.Printf("FFmpeg message: %s\n", ffErr.Message)
    }
}
```

---

## Logging

### Set Log Level

```go
// Show all messages
ffgo.SetLogLevel(ffgo.LogVerbose)

// Show warnings and errors only
ffgo.SetLogLevel(ffgo.LogWarning)

// Disable logging
ffgo.SetLogLevel(ffgo.LogQuiet)
```

### Log Levels

| Level | Description |
|-------|-------------|
| `LogQuiet` | No output |
| `LogPanic` | Only panics |
| `LogFatal` | Fatal errors |
| `LogError` | All errors |
| `LogWarning` | Warnings and errors |
| `LogInfo` | Informational messages |
| `LogVerbose` | Detailed information |
| `LogDebug` | Debug output |

### Custom Log Handler

```go
ffgo.SetLogCallback(func(level ffgo.LogLevel, message string) {
    log.Printf("[FFmpeg %s] %s", level, message)
})
```

---

## Low-Level API

For advanced use cases, ffgo provides direct access to FFmpeg functions.

### Packages

| Package | Description |
|---------|-------------|
| `ffgo/avutil` | Utilities, frames, dictionaries |
| `ffgo/avcodec` | Encoding and decoding |
| `ffgo/avformat` | Container formats, I/O |
| `ffgo/swscale` | Scaling and pixel conversion |

### Example: Direct avcodec Usage

```go
import (
    "github.com/obinnaokechukwu/ffgo/avcodec"
    "github.com/obinnaokechukwu/ffgo/avformat"
    "github.com/obinnaokechukwu/ffgo/avutil"
)

func main() {
    // Open input
    var fmtCtx *avformat.Context
    err := avformat.OpenInput(&fmtCtx, "input.mp4", nil, nil)
    if err != nil {
        panic(err)
    }
    defer avformat.CloseInput(&fmtCtx)

    // Find stream info
    err = avformat.FindStreamInfo(fmtCtx, nil)
    if err != nil {
        panic(err)
    }

    // Find video stream
    videoIdx := avformat.FindBestStream(fmtCtx, avutil.MediaTypeVideo, -1, -1, nil, 0)
    if videoIdx < 0 {
        panic("no video stream")
    }

    // Get decoder
    stream := avformat.GetStream(fmtCtx, videoIdx)
    codec := avcodec.FindDecoder(stream.CodecID())
    codecCtx := avcodec.AllocContext3(codec)
    defer avcodec.FreeContext(&codecCtx)

    avcodec.ParametersToContext(codecCtx, stream.CodecPar())
    err = avcodec.Open2(codecCtx, codec, nil)
    if err != nil {
        panic(err)
    }

    // Decode frames
    frame := avutil.FrameAlloc()
    defer avutil.FrameFree(&frame)

    packet := avcodec.PacketAlloc()
    defer avcodec.PacketFree(&packet)

    for {
        ret := avformat.ReadFrame(fmtCtx, packet)
        if ret < 0 {
            break
        }

        if packet.StreamIndex() == videoIdx {
            avcodec.SendPacket(codecCtx, packet)
            for avcodec.ReceiveFrame(codecCtx, frame) == nil {
                // Process frame
                fmt.Printf("Frame: %dx%d\n", frame.Width(), frame.Height())
            }
        }

        avcodec.PacketUnref(packet)
    }
}
```

---

## FAQ

### Why do I need the shim library?

FFmpeg's logging functions use C variadics (`...`), which pure Go cannot call. The shim (~10KB) wraps these functions. Without it, logging won't work, but decoding/encoding will.

### Can I statically link FFmpeg?

No. ffgo uses dynamic linking via `dlopen`. Static linking requires CGO.

### Why doesn't iOS/Android work?

The underlying purego library requires CGO on mobile platforms. For mobile, use a CGO-based FFmpeg binding instead.

### How do I use hardware acceleration?

```go
decoder, err := ffgo.NewDecoder("input.mp4",
    ffgo.WithHWDevice("cuda"),      // NVIDIA
    // ffgo.WithHWDevice("vaapi"),  // Linux VA-API
    // ffgo.WithHWDevice("videotoolbox"), // macOS
)
```

### What FFmpeg versions are supported?

FFmpeg 4.x through 7.x are supported. The library auto-detects the installed version.

### How do I report bugs?

Open an issue at https://github.com/obinnaokechukwu/ffgo/issues with:
- Your OS and architecture
- FFmpeg version (`ffmpeg -version`)
- Minimal code to reproduce

---

## License

ffgo is licensed under the MIT License.

FFmpeg is licensed under LGPL/GPL. Ensure your usage complies with FFmpeg's license.
