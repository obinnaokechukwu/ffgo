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
8. [Audio Resampling](#audio-resampling)
9. [Filter Graphs](#filter-graphs)
10. [Advanced Codec Options](#advanced-codec-options)
11. [Stream Copy Mode](#stream-copy-mode)
12. [Multi-Stream Muxing](#multi-stream-muxing)
13. [Subtitles](#subtitles)
14. [Hardware Acceleration](#hardware-acceleration)
15. [Advanced Seeking](#advanced-seeking)
16. [Metadata and Chapters](#metadata-and-chapters)
17. [Device Capture](#device-capture)
18. [Network Streaming](#network-streaming)
19. [Image Sequences](#image-sequences)
20. [Custom I/O](#custom-io)
21. [Error Handling](#error-handling)
22. [Logging](#logging)
23. [Low-Level API](#low-level-api)
24. [FAQ](#faq)

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

### Frame Ownership (Important)

- `Decoder.ReadFrame()` returns a `*ffgo.FrameWrapper`. You do not free it; it is a Go wrapper around the current decoded frame.
- `Decoder.DecodeVideo()` / `DecodeAudio()` return a **borrowed** `ffgo.Frame` (owned by the decoder and reused). Do **not** free it. If you need to keep it, call `FrameClone()` / `Frame.Clone()` or use `DecodeVideoCopy()` / `ReadFrameCopy()`.
- `ffgo.FrameAlloc()` returns an **owned** `ffgo.Frame` that you must free with `FrameFree(&f)` / `f.Free()`.

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
err := encoder.WriteVideoFrame(videoFrame) // videoFrame is an ffgo.Frame
if err != nil {
    return err
}

// Write audio frame
err = encoder.WriteAudioFrame(audioFrame) // audioFrame is an ffgo.Frame
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
            err = encoder.WriteVideoFrame(frame.Raw())
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
        scaled, err := scaler.Scale(frame.Raw())
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

## Audio Resampling

Convert audio between sample rates, channel layouts, and sample formats.

### Create a Resampler

```go
resampler, err := ffgo.NewResampler(
    ffgo.AudioFormat{
        SampleRate:   44100,
        Channels:     2,
        SampleFormat: ffgo.SampleFormatS16,
    },
    ffgo.AudioFormat{
        SampleRate:   48000,
        Channels:     2,
        SampleFormat: ffgo.SampleFormatFLTP,
    },
)
if err != nil {
    return err
}
defer resampler.Close()
```

### Resample Audio Frames

```go
for {
    frame, err := decoder.ReadFrame()
    if err != nil {
        return err
    }
    if frame == nil {
        break
    }

    if frame.MediaType() == ffgo.MediaTypeAudio {
        resampled, err := resampler.Resample(frame)
        if err != nil {
            return err
        }
        if resampled != nil {
            encoder.WriteAudioFrame(resampled)
        }
    }
}

// Flush remaining samples
for {
    flushed, err := resampler.Flush()
    if err != nil || flushed == nil {
        break
    }
    encoder.WriteAudioFrame(flushed)
}
```

### Channel Layout Constants

| Layout | Description |
|--------|-------------|
| `ChannelLayoutMono` | Single channel |
| `ChannelLayoutStereo` | Left + Right |
| `ChannelLayout5Point1` | 5.1 surround |
| `ChannelLayout7Point1` | 7.1 surround |

### Sample Format Constants

| Format | Description |
|--------|-------------|
| `SampleFormatU8` | Unsigned 8-bit |
| `SampleFormatS16` | Signed 16-bit (CD quality) |
| `SampleFormatS32` | Signed 32-bit |
| `SampleFormatFLT` | Float 32-bit |
| `SampleFormatFLTP` | Float 32-bit planar (most encoders) |

---

## Filter Graphs

Apply complex video and audio effects using FFmpeg's filter system.

### Video Filters

```go
// Create a video filter graph
graph, err := ffgo.NewVideoFilterGraph(
    "scale=1280:720,hflip",  // FFmpeg filter string
    1920, 1080,               // Input dimensions
    ffgo.PixelFormatYUV420P,
)
if err != nil {
    return err
}
defer graph.Close()

// Process frames through the filter
for {
    frame, _ := decoder.ReadFrame()
    if frame == nil {
        break
    }

    if frame.MediaType() == ffgo.MediaTypeVideo {
        filtered, err := graph.Filter(frame)
        if err != nil {
            return err
        }
        for _, f := range filtered {
            encoder.WriteVideoFrame(f)
        }
    }
}

// Flush remaining frames
for _, f := range graph.Flush() {
    encoder.WriteVideoFrame(f)
}
```

### Audio Filters

```go
graph, err := ffgo.NewAudioFilterGraph(
    "volume=0.5,aresample=48000",
    44100, 2, ffgo.SampleFormatFLTP,
)
```

### Common Filter Presets

```go
// Scale video
ffgo.Scale(1280, 720)                    // "scale=1280:720"
ffgo.ScaleKeepAspect(1280, 720)          // Maintain aspect ratio

// Crop video
ffgo.Crop(640, 480, 100, 50)             // Width, Height, X, Y

// Effects
ffgo.HFlip()                              // Horizontal flip
ffgo.VFlip()                              // Vertical flip
ffgo.Blur(2.0)                            // Gaussian blur
ffgo.Sharpen(1.5)                         // Sharpen

// Fade effects
ffgo.FadeIn(0, 30)                        // Fade in first 30 frames
ffgo.FadeOut(270, 30)                     // Fade out last 30 frames

// Overlay/watermark
ffgo.Overlay(10, 10)                      // Position for overlay
ffgo.DrawText("Hello", 10, 10, 24, "white")

// Audio
ffgo.Volume(0.5)                          // Half volume
ffgo.Normalize()                          // Normalize loudness
```

### Complex Filter Chains

```go
// Combine multiple filters
filters := fmt.Sprintf("%s,%s,%s",
    ffgo.Scale(1280, 720),
    ffgo.Sharpen(1.0),
    ffgo.DrawText("Watermark", 10, 10, 16, "white"),
)
graph, _ := ffgo.NewVideoFilterGraph(filters, 1920, 1080, ffgo.PixelFormatYUV420P)
```

---

## Advanced Codec Options

Fine-tune encoding quality and performance.

### Encoder Presets

```go
encoder, err := ffgo.NewEncoder("output.mp4", &ffgo.EncoderOptions{
    Video: &ffgo.VideoEncoderConfig{
        Codec:     ffgo.CodecH264,
        Width:     1920,
        Height:    1080,
        FrameRate: ffgo.NewRational(30, 1),

        // Quality/speed tradeoff
        Preset: ffgo.PresetMedium,  // ultrafast, fast, medium, slow, veryslow

        // Content-specific tuning
        Tune: ffgo.TuneFilm,        // film, animation, grain, zerolatency
    },
})
```

### Preset Options

| Preset | Speed | Quality | Use Case |
|--------|-------|---------|----------|
| `PresetUltrafast` | Fastest | Lowest | Real-time streaming |
| `PresetVeryfast` | Very fast | Low | Live encoding |
| `PresetFast` | Fast | Medium | General purpose |
| `PresetMedium` | Medium | Good | Default balance |
| `PresetSlow` | Slow | High | Quality-focused |
| `PresetVeryslow` | Very slow | Highest | Archival |

### Tune Options

| Tune | Description |
|------|-------------|
| `TuneFilm` | Real-world footage |
| `TuneAnimation` | Animated content |
| `TuneGrain` | Preserve film grain |
| `TuneZeroLatency` | Minimal delay streaming |
| `TuneFastDecode` | Optimize for playback speed |

### Rate Control

```go
// CRF (Constant Rate Factor) - Quality-based
encoder, _ := ffgo.NewEncoder("output.mp4", &ffgo.EncoderOptions{
    Video: &ffgo.VideoEncoderConfig{
        Codec:       ffgo.CodecH264,
        RateControl: ffgo.RateControlCRF,
        CRF:         18,  // 0-51, lower = better (18-23 typical)
    },
})

// CBR (Constant Bitrate) - Streaming
encoder, _ := ffgo.NewEncoder("output.mp4", &ffgo.EncoderOptions{
    Video: &ffgo.VideoEncoderConfig{
        Codec:       ffgo.CodecH264,
        RateControl: ffgo.RateControlCBR,
        Bitrate:     5_000_000,  // 5 Mbps
    },
})

// VBR (Variable Bitrate)
encoder, _ := ffgo.NewEncoder("output.mp4", &ffgo.EncoderOptions{
    Video: &ffgo.VideoEncoderConfig{
        Codec:       ffgo.CodecH264,
        RateControl: ffgo.RateControlVBR,
        Bitrate:     5_000_000,
        MinBitrate:  3_000_000,
        MaxBitrate:  8_000_000,
    },
})
```

### Profile and Level

```go
encoder, _ := ffgo.NewEncoder("output.mp4", &ffgo.EncoderOptions{
    Video: &ffgo.VideoEncoderConfig{
        Codec:   ffgo.CodecH264,
        Profile: ffgo.ProfileHigh,     // baseline, main, high
        Level:   ffgo.Level41,         // 3.0, 4.0, 4.1, 5.1
    },
})
```

### Custom Codec Options

```go
encoder, _ := ffgo.NewEncoder("output.mp4", &ffgo.EncoderOptions{
    Video: &ffgo.VideoEncoderConfig{
        Codec: ffgo.CodecH264,
        CodecOptions: map[string]string{
            "x264-params": "aq-mode=3:psy-rd=1.0",
            "refs":        "4",
        },
    },
})
```

---

## Stream Copy Mode

Fast remuxing without re-encoding.

### Basic Stream Copy

```go
func fastRemux(input, output string) error {
    decoder, _ := ffgo.NewDecoder(input)
    defer decoder.Close()

    // Create encoder in copy mode
    encoder, _ := ffgo.NewEncoder(output, &ffgo.EncoderOptions{
        CopyVideo: true,
        CopyAudio: true,
        SourceStreams: &ffgo.StreamCopySource{
            VideoParams: decoder.VideoStream().CodecParameters(),
            AudioParams: decoder.AudioStream().CodecParameters(),
        },
    })
    defer encoder.Close()

    // Copy packets directly (no decode/encode)
    for {
        packet, _ := decoder.ReadPacket()
        if packet == nil {
            break
        }
        encoder.WritePacket(packet)
    }

    return nil
}
```

### Copy Video, Re-encode Audio

```go
encoder, _ := ffgo.NewEncoder("output.mp4", &ffgo.EncoderOptions{
    CopyVideo: true,
    SourceStreams: &ffgo.StreamCopySource{
        VideoParams: decoder.VideoStream().CodecParameters(),
    },
    Audio: &ffgo.AudioEncoderConfig{
        Codec:      ffgo.CodecAAC,
        SampleRate: 48000,
        Bitrate:    128_000,
    },
})
```

---

## Multi-Stream Muxing

Create files with multiple video, audio, or subtitle tracks.

### Multiple Audio Tracks

```go
muxer, _ := ffgo.NewMuxer("output.mkv", "matroska")

// Add video stream
videoStream, _ := muxer.AddVideoStream(&ffgo.VideoEncoderConfig{
    Codec: ffgo.CodecH264,
    Width: 1920, Height: 1080,
})

// Add English audio
audioEN, _ := muxer.AddAudioStream(&ffgo.AudioEncoderConfig{
    Codec: ffgo.CodecAAC,
    SampleRate: 48000,
    Channels: 2,
})

// Add Spanish audio
audioES, _ := muxer.AddAudioStream(&ffgo.AudioEncoderConfig{
    Codec: ffgo.CodecAAC,
    SampleRate: 48000,
    Channels: 2,
})

// Write header
muxer.WriteHeader()

// Write frames to appropriate streams
for {
    // ... read from sources
    muxer.WriteFrame(videoStream, videoFrame)
    muxer.WriteFrame(audioEN, audioENFrame)
    muxer.WriteFrame(audioES, audioESFrame)
}

muxer.WriteTrailer()
muxer.Close()
```

### Adding Subtitles

```go
// Add subtitle stream
subs, _ := muxer.AddSubtitleStream(ffgo.SubtitleFormatSRT)
```

---

## Subtitles

Decode, encode, and render subtitles.

### Extract Subtitles

```go
decoder, _ := ffgo.NewDecoder("video.mkv")

// Create subtitle decoder
subDecoder, _ := ffgo.NewSubtitleDecoder(decoder.SubtitleStream())
defer subDecoder.Close()

var subtitles []*ffgo.Subtitle
for {
    packet, _ := decoder.ReadPacket()
    if packet == nil {
        break
    }

    if packet.StreamIndex() == decoder.SubtitleStreamIndex() {
        sub, err := subDecoder.Decode(packet)
        if err == nil && sub != nil {
            fmt.Printf("%v - %v: %s\n", sub.StartTime, sub.EndTime, sub.Text)
            subtitles = append(subtitles, sub)
        }
    }
}
```

### Burn Subtitles into Video

```go
renderer, _ := ffgo.NewSubtitleRenderer("subtitles.srt", 1920, 1080)
defer renderer.Close()

for {
    frame, _ := decoder.ReadFrame()
    if frame == nil {
        break
    }

    if frame.MediaType() == ffgo.MediaTypeVideo {
        // Subtitle is burned into the frame
        rendered, _ := renderer.Render(frame)
        encoder.WriteVideoFrame(rendered)
    }
}
```

### Subtitle Types

| Type | Description | Formats |
|------|-------------|---------|
| `SubtitleTypeText` | Plain text | SRT, WebVTT |
| `SubtitleTypeASS` | Styled text | ASS/SSA |
| `SubtitleTypeBitmap` | Image-based | DVD, Blu-ray |

---

## Hardware Acceleration

Use GPU for faster encoding/decoding.

### Create Hardware Device

```go
// NVIDIA CUDA
hwDevice, err := ffgo.NewHWDevice(ffgo.HWDeviceCUDA, "")

// Linux VA-API
hwDevice, err := ffgo.NewHWDevice(ffgo.HWDeviceVAAPI, "/dev/dri/renderD128")

// macOS VideoToolbox
hwDevice, err := ffgo.NewHWDevice(ffgo.HWDeviceVideoToolbox, "")

// Intel Quick Sync
hwDevice, err := ffgo.NewHWDevice(ffgo.HWDeviceQSV, "")
```

### Hardware-Accelerated Decoding

```go
hwDevice, err := ffgo.NewHWDevice(ffgo.HWDeviceCUDA, "")
if err != nil {
    // Fallback to software
    decoder, _ = ffgo.NewDecoder(input)
} else {
    defer hwDevice.Close()
    decoder, _ = ffgo.NewHWDecoder(input, hwDevice)
}

// Frames automatically transferred to system memory
for {
    frame, _ := decoder.ReadFrame()
    if frame == nil {
        break
    }
    // Process frame...
}
```

### Keep Frames in GPU Memory

```go
hwDecoder, _ := ffgo.NewHWDecoder(input, hwDevice)

// Read frame without transfer (stays in GPU)
hwFrame, _ := hwDecoder.ReadHWFrame()

// Transfer when needed
swFrame, _ := hwDecoder.TransferToSystem(hwFrame)
```

### Available Device Types

| Type | Platform | Description |
|------|----------|-------------|
| `HWDeviceCUDA` | NVIDIA GPUs | CUDA acceleration |
| `HWDeviceVAAPI` | Linux | Video Acceleration API |
| `HWDeviceVideoToolbox` | macOS | Apple hardware decoder |
| `HWDeviceQSV` | Intel | Quick Sync Video |
| `HWDeviceD3D11VA` | Windows | DirectX 11 |
| `HWDeviceDXVA2` | Windows | DirectX Video Acceleration |
| `HWDeviceVulkan` | Cross-platform | Vulkan Video |

---

## Advanced Seeking

Navigate precisely within media files.

### Basic Seeking

```go
// Seek to 30 seconds
err := decoder.Seek(30 * time.Second)

// Seek backward to nearest keyframe
err := decoder.Seek(30 * time.Second, ffgo.WithSeekBackward())
```

### Frame-Accurate Seeking

```go
// Seeks to keyframe, then decodes forward to exact position
err := decoder.SeekPrecise(30 * time.Second)

// Next ReadFrame() returns the frame at exactly 30 seconds
frame, _ := decoder.ReadFrame()
```

### Seek by Frame Number

```go
// Seek to frame 900 (30 seconds at 30fps)
err := decoder.SeekToFrame(900)
```

### Seek by Byte Position

```go
// Seek to byte offset
err := decoder.SeekToByte(1024 * 1024 * 100) // 100 MB
```

### Generate Thumbnails

```go
// Extract thumbnails every 10 seconds, max 10 thumbnails
thumbnails, err := decoder.GenerateThumbnails(10*time.Second, 10)
for i, thumb := range thumbnails {
    ffgo.SaveFrame(thumb, fmt.Sprintf("thumb_%02d.jpg", i))
}
```

### Get Keyframe Index

```go
keyframes, _ := decoder.GetKeyframes()
for _, kf := range keyframes {
    fmt.Printf("Keyframe at %v (byte %d)\n", kf.PTS, kf.Position)
}
```

---

## Metadata and Chapters

Read and write media metadata.

### Read Metadata

```go
decoder, _ := ffgo.NewDecoder("video.mp4")

// File-level metadata
meta := decoder.GetMetadata()
fmt.Printf("Title: %s\n", meta[ffgo.MetadataTitle])
fmt.Printf("Artist: %s\n", meta[ffgo.MetadataArtist])
fmt.Printf("Album: %s\n", meta[ffgo.MetadataAlbum])

// Stream-level metadata
streamMeta := decoder.GetStreamMetadata(0)
fmt.Printf("Language: %s\n", streamMeta[ffgo.MetadataLanguage])
```

### Write Metadata

```go
encoder, _ := ffgo.NewEncoder("output.mp4", opts)

encoder.SetMetadata(ffgo.Metadata{
    ffgo.MetadataTitle:     "My Video",
    ffgo.MetadataArtist:    "John Doe",
    ffgo.MetadataDate:      "2024",
    ffgo.MetadataComment:   "Encoded with ffgo",
    ffgo.MetadataEncoder:   "ffgo v1.0",
})
```

### Common Metadata Keys

| Key | Description |
|-----|-------------|
| `MetadataTitle` | Title |
| `MetadataArtist` | Artist/Creator |
| `MetadataAlbum` | Album name |
| `MetadataAlbumArtist` | Album artist |
| `MetadataGenre` | Genre |
| `MetadataDate` | Release date |
| `MetadataTrack` | Track number |
| `MetadataComment` | Comments |
| `MetadataLanguage` | Language code |

### Read Chapters

```go
chapters := decoder.GetChapters()
for _, ch := range chapters {
    fmt.Printf("Chapter: %s (%v - %v)\n", ch.Title, ch.Start, ch.End)
}
```

### Write Chapters

```go
encoder.SetChapters([]ffgo.Chapter{
    {Start: 0, End: 60*time.Second, Title: "Introduction"},
    {Start: 60*time.Second, End: 300*time.Second, Title: "Main Content"},
    {Start: 300*time.Second, End: 360*time.Second, Title: "Conclusion"},
})
```

### Attachments (MKV)

```go
// Read attachments (fonts, images embedded in MKV)
attachments := decoder.GetAttachments()
for _, att := range attachments {
    fmt.Printf("Attachment: %s (%s)\n", att.Filename, att.MimeType)
}

// Add attachment
encoder.AddAttachment(ffgo.Attachment{
    Filename:    "cover.jpg",
    MimeType:    "image/jpeg",
    Data:        coverImageData,
})
```

---

## Device Capture

Capture from cameras, microphones, and screens.

### List Available Devices

```go
// List video devices (cameras)
videoDevices, _ := ffgo.ListDevices(ffgo.DeviceTypeVideo)
for _, dev := range videoDevices {
    fmt.Printf("Video: %s - %s\n", dev.Name, dev.Description)
}

// List audio devices (microphones)
audioDevices, _ := ffgo.ListDevices(ffgo.DeviceTypeAudio)
for _, dev := range audioDevices {
    fmt.Printf("Audio: %s - %s\n", dev.Name, dev.Description)
}
```

### Capture from Camera

```go
decoder, err := ffgo.NewCapture(ffgo.CaptureConfig{
    Device:     "/dev/video0",  // Linux
    // Device:  "0",            // macOS (device index)
    // Device:  "HD Webcam",    // Windows (device name)
    DeviceType: ffgo.DeviceTypeVideo,
    Width:      1920,
    Height:     1080,
    FrameRate:  ffgo.NewRational(30, 1),
})
if err != nil {
    return err
}
defer decoder.Close()

// Read frames from camera
for {
    frame, _ := decoder.ReadFrame()
    if frame == nil {
        break
    }
    // Process frame...
}
```

### Screen Capture

```go
decoder, err := ffgo.CaptureScreen(
    &ffgo.Rect{X: 0, Y: 0, Width: 1920, Height: 1080},
    ffgo.NewRational(30, 1),
)
if err != nil {
    return err
}
defer decoder.Close()

// Record screen
encoder, _ := ffgo.NewEncoder("screen.mp4", opts)
for {
    frame, _ := decoder.ReadFrame()
    if frame == nil {
        break
    }
    _ = encoder.WriteVideoFrame(frame.Raw())
}
```

---

## Network Streaming

Read from network sources (RTMP, HLS, HTTP).

### Open Network Stream

```go
decoder, err := ffgo.NewNetworkDecoder("rtmp://server/live/stream", &ffgo.ProtocolOptions{
    Timeout:        10 * time.Second,
    ReconnectCount: 3,
    ReconnectDelay: 2 * time.Second,
})
if err != nil {
    return err
}
defer decoder.Close()
```

### HLS Playback

```go
decoder, err := ffgo.NewNetworkDecoder("https://example.com/stream.m3u8", &ffgo.ProtocolOptions{
    Timeout: 10 * time.Second,
    HTTPHeaders: map[string]string{
        "User-Agent":    "ffgo/1.0",
        "Authorization": "Bearer token",
    },
})
```

### RTSP Camera

```go
decoder, err := ffgo.NewNetworkDecoder("rtsp://admin:pass@camera/stream1", &ffgo.ProtocolOptions{
    Timeout:   5 * time.Second,
    TLSVerify: false,  // Skip certificate verification
})
```

### Protocol Options

| Option | Description |
|--------|-------------|
| `Timeout` | Connection timeout |
| `ReconnectCount` | Auto-reconnect attempts |
| `ReconnectDelay` | Delay between reconnects |
| `BufferSize` | Network buffer size |
| `HTTPHeaders` | Custom HTTP headers |
| `TLSVerify` | Verify TLS certificates |

---

## Image Sequences

Work with image sequences as video.

### Read Image Sequence

```go
decoder, err := ffgo.NewImageSequenceDecoder(ffgo.ImageSequenceConfig{
    Pattern:     "frames/frame_%04d.png",  // frame_0001.png, frame_0002.png, ...
    StartNumber: 1,
    FrameRate:   ffgo.NewRational(30, 1),
})
if err != nil {
    return err
}
defer decoder.Close()

// Process as video
for {
    frame, _ := decoder.ReadFrame()
    if frame == nil {
        break
    }
    // frame is a video frame
}
```

### Write Image Sequence

```go
encoder, err := ffgo.NewImageSequenceEncoder(ffgo.ImageSequenceConfig{
    Pattern:   "output/frame_%04d.png",
    FrameRate: ffgo.NewRational(30, 1),
}, ffgo.PixelFormatRGB24)
```

### Extract Single Frame

```go
// Extract frame at 30 seconds
err := ffgo.ExtractFrame("video.mp4", 30*time.Second, "thumbnail.jpg")
```

### Save Frame as Image

```go
frame, _ := decoder.ReadFrame()
err := ffgo.SaveFrame(frame, "screenshot.png")
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
| `ffgo/avutil` | Utilities, frames, dictionaries, errors |
| `ffgo/avcodec` | Encoding and decoding |
| `ffgo/avformat` | Container formats, I/O |
| `ffgo/swscale` | Scaling and pixel conversion |
| `ffgo/swresample` | Audio resampling and format conversion |
| `ffgo/avfilter` | Filter graphs and effects |

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

### What FFmpeg versions are supported?

FFmpeg 4.x through 7.x are supported. The library auto-detects the installed version and adapts to API differences.

### How do I check if hardware acceleration is available?

```go
hwDevice, err := ffgo.NewHWDevice(ffgo.HWDeviceCUDA, "")
if err != nil {
    // CUDA not available, use software decoding
    decoder, _ = ffgo.NewDecoder(input)
} else {
    defer hwDevice.Close()
    decoder, _ = ffgo.NewHWDecoder(input, hwDevice)
}
```

### What's the best preset for encoding?

| Goal | Preset | Rate Control |
|------|--------|--------------|
| Live streaming | `PresetUltrafast` | CBR |
| General purpose | `PresetMedium` | CRF 23 |
| Archival quality | `PresetSlow` | CRF 18 |
| Animation/screen | `PresetMedium` + `TuneAnimation` | CRF 20 |

### How do I convert between containers without re-encoding?

Use stream copy mode for fast remuxing:

```go
encoder, _ := ffgo.NewEncoder("output.mkv", &ffgo.EncoderOptions{
    CopyVideo: true,
    CopyAudio: true,
    SourceStreams: &ffgo.StreamCopySource{
        VideoParams: decoder.VideoStream().CodecParameters(),
        AudioParams: decoder.AudioStream().CodecParameters(),
    },
})
```

### Can I apply filters without re-encoding?

No. Filters require decoded frames, so re-encoding is necessary. For simple operations like cropping or scaling, filters are very efficient.

### How do I handle audio with different sample rates?

Use the Resampler to convert audio formats:

```go
resampler, _ := ffgo.NewResampler(
    ffgo.AudioFormat{SampleRate: 44100, Channels: 2, SampleFormat: ffgo.SampleFormatS16},
    ffgo.AudioFormat{SampleRate: 48000, Channels: 2, SampleFormat: ffgo.SampleFormatFLTP},
)
```

### What's the difference between Seek and SeekPrecise?

- `Seek()` seeks to the nearest keyframe (fast, ~100ms accuracy)
- `SeekPrecise()` decodes forward to exact position (slower, frame-accurate)

### How do I extract thumbnails efficiently?

```go
thumbnails, _ := decoder.GenerateThumbnails(10*time.Second, 10)
for i, thumb := range thumbnails {
    ffgo.SaveFrame(thumb, fmt.Sprintf("thumb_%02d.jpg", i))
}
```

### How do I report bugs?

Open an issue at https://github.com/obinnaokechukwu/ffgo/issues with:
- Your OS and architecture
- FFmpeg version (`ffmpeg -version`)
- Minimal code to reproduce
- Expected vs actual behavior

---

## License

ffgo is licensed under the MIT License.

FFmpeg is licensed under LGPL/GPL. Ensure your usage complies with FFmpeg's license.
