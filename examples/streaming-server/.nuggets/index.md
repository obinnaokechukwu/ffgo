---
title: Streaming Server Knowledge Vault
keywords: [streaming, ffgo, server, transcoding]
tags: [index]
---

# Streaming Server Knowledge Vault

Welcome to the knowledge vault for the ffgo streaming server project. This vault documents the architecture, implementation patterns, and internal mechanics of a production-grade streaming server built on ffgo.

## What is This?

This is a collection of detailed technical documentation about how a streaming server works internally. It covers:

- **Architecture** - How components connect and exchange data
- **Implementation Details** - How specific features work under the hood
- **Design Patterns** - Proven approaches to common problems
- **Performance Tuning** - Optimization techniques and trade-offs
- **Debugging Guide** - How to troubleshoot common issues

## Organization

The vault is organized by subsystem:

### Core Subsystems

- **[Media Processing](./media/index.md)** - Decoding, encoding, frame handling with ffgo
- **[Transcoding](./transcoding/index.md)** - Adaptive bitrate encoding, quality control
- **[Delivery](./delivery/index.md)** - HLS/DASH segmentation and streaming
- **[Ingest](./ingest/index.md)** - Input protocols and stream handling

### Support Systems

- **[Performance](./performance/index.md)** - Memory, encoding optimization, tuning
- **[Patterns](./patterns/index.md)** - Design patterns and best practices
- **[Gotchas](./gotchas/index.md)** - Common pitfalls and debugging

## Quick Start by Role

**If you're implementing:**
- Input handler → Read `media/decoder-lifecycle`, `patterns/ffgo-integration-pattern`
- Transcoding → Read `transcoding/adaptive-bitrate`, `performance/encoding-performance`
- Output delivery → Read `delivery/hls-segmentation`, `media/stream-copy-optimization`

**If you're debugging:**
- Memory issues → Read `gotchas/frame-leaks`, `performance/memory-efficiency`
- Performance problems → Read `performance/encoding-performance`, `gotchas/encoder-bottlenecks`
- Encoding failures → Read `patterns/error-handling-strategy`, `media/encoder-lifecycle`

**If you're optimizing:**
- Reduce latency → Read `performance/memory-efficiency`, `transcoding/adaptive-bitrate`
- Reduce CPU → Read `media/stream-copy-optimization`, `performance/encoding-performance`
- Reduce memory → Read `performance/memory-efficiency`, `media/frame-pipeline`

## Architecture Overview

```
Ingest Layer (RTMP/RTSP/HTTP)
         ↓
    Decoder (ffgo)
         ↓
    Frame Distribution (Reference counting)
         ↓
    ┌─────────────────────────────────┐
    │  Transcoding Pipeline (per-variant)
    │  Scaler → Encoder → Segmenter
    └─────────────────────────────────┘
         ↓
    Output Layer (HLS/DASH)
         ↓
    HTTP Server → Clients
```

## Key Concepts

**Frame:** Decoded audio/video data, shared by reference across pipeline stages

**Variant:** Output encoding (720p/480p/360p), each with independent encoder

**Segment:** 6-10 second media chunk, packaged for HTTP delivery

**Manifest:** Playlist describing available segments and variants

**Stream:** Complete processing pipeline from ingest to delivery

## ffgo Basics

The server uses ffgo for all media processing:

```go
// Decoding
decoder, _ := ffgo.NewDecoder("input.mp4")
frame, _ := decoder.ReadFrame()

// Encoding
encoder, _ := ffgo.NewEncoder("output.mp4")
encoder.EncodeFrame(frame)

// Scaling
scaler, _ := ffgo.NewScaler(1920, 1080, ffgo.PixelFormatYUV420P,
    1280, 720, ffgo.PixelFormatYUV420P)
scaler.ScaleFrame(srcFrame, dstFrame)

// Remuxing (fast stream copy)
remuxer, _ := ffgo.NewRemuxer("input.mp4", "output.mkv")
remuxer.Remux()
```

## Common Tasks

**Add support for new input protocol:**
1. Implement io.Reader wrapper around protocol
2. Use `NewDecoderFromReader()` to feed ffgo
3. See [decoder lifecycle](./media/decoder-lifecycle.md)

**Optimize encoding speed:**
1. Enable GPU acceleration: `WithHWDevice("cuda")`
2. Reduce preset: `Preset: "fast"` instead of "slow"
3. Lower CRF: `CRF: 32` trades quality for speed
4. See [encoding performance](./performance/encoding-performance.md)

**Debug memory leak:**
1. Add frame accounting (count alloc/free)
2. Monitor heap size with `runtime.ReadMemStats()`
3. Check for missing `ffgo.FrameFree(&frame)` calls
4. Use Valgrind for low-level debugging
5. See [frame leaks](./gotchas/frame-leaks.md)

**Handle ABR variant synchronization:**
1. All variants share input frames via reference counting
2. Keyframes must align across variants (6-10s apart)
3. Each variant encodes independently in goroutine
4. See [adaptive bitrate](./transcoding/adaptive-bitrate.md)

## Conventions

**Naming:**
- Methods taking context: `func (x *X) Operation(ctx context.Context) error`
- Frame ownership: If function returns frame, caller must free it
- Stream indices: -1 means not found/not applicable

**Error Handling:**
- `ffgo.IsEOF(err)` - Check for end-of-file
- `ffgo.IsAgain(err)` - Check for EAGAIN (try again)
- Fatal errors returned directly

**Resource Cleanup:**
- Use `defer` for cleanup: `defer decoder.Close()`
- Frame pooling for frequently allocated frames
- Goroutine-per-stream for independent error isolation

## Common Errors and Quick Fixes

| Error | Likely Cause | Fix |
|-------|-------------|-----|
| "Rescale timestamp error" | Time base mismatch in muxer | Rescale PTS/DTS before writing packet |
| "Codec not found" | FFmpeg missing codec | Check FFmpeg build includes codec |
| "EAGAIN (try again)" | Encoder buffer full | Flush encoder with `ReceivePacket()` loop |
| "Memory growth over time" | Frame leak | Check for missing `ffgo.FrameFree(&frame)` calls |
| "Frames not processed" | Encoding stalled | Check queue depth and backpressure |
| "A/V sync issues" | Timestamp errors | Verify time bases match, rescale correctly |

## Performance Targets

Running on modern server hardware (8 cores, 1 GPU):

| Metric | Target |
|--------|--------|
| 1080p 30fps to 4 variants | 5x real-time |
| Memory per stream | 100-200 MB |
| Encoding latency | < 100ms |
| Startup latency | < 1s |

## Further Reading

- [FFmpeg Wiki](https://trac.ffmpeg.org/wiki)
- [ffgo project](https://github.com/obinnaokechukwu/ffgo)
- [HLS spec](https://datatracker.ietf.org/doc/html/draft-pantos-http-live-streaming)
- [DASH spec](https://dashif.org/specs/)

## Contributing

When adding new features:

1. Document your design in this vault
2. Add performance characteristics
3. Include error handling examples
4. Update this index if adding new sections

## Questions?

- Check the gotchas section for your symptom
- Search by keyword (memory, encoding, segment)
- Look for "Related" links at top of each nugget
