# Streaming Server Knowledge Vault

A comprehensive technical reference documenting the internals of a production-grade streaming server built on the ffgo library.

## Quick Facts

- **Total Nuggets**: 25
- **Total Lines**: 5,269
- **Total Size**: 448 KB
- **Domains**: 11 (Media, Transcoding, Delivery, Ingest, Streams, Performance, Patterns, Gotchas, API, Storage, Cluster)
- **Code Examples**: 150+
- **Diagrams**: 10+

## What's Inside

This vault documents a **fictional but plausible** production streaming server like SRS, Nginx-RTMP, or Wowza, implemented entirely using ffgo for media processing.

Written from a **contributor perspective**, emphasizing:
- Internal architecture and design decisions
- Implementation patterns with real Go code
- Performance optimization techniques
- Debugging and troubleshooting strategies
- Integration with ffgo library

## Quick Navigation

### Start Here
- [Master Index](./index.md) - Architecture overview and learning paths
- [ffgo Integration Pattern](./patterns/ffgo-integration-pattern.md) - How ffgo powers the server

### Core Media Processing
- [Decoder Lifecycle](./media/decoder-lifecycle.md) - Input parsing and frame extraction
- [Encoder Lifecycle](./media/encoder-lifecycle.md) - Output encoding pipeline
- [Frame Pipeline](./media/frame-pipeline.md) - Buffer management and multi-stage processing
- [Stream Copy Optimization](./media/stream-copy-optimization.md) - Zero-copy remuxing

### Transcoding
- [Adaptive Bitrate](./transcoding/adaptive-bitrate.md) - Multi-variant encoding architecture

### Delivery
- [HLS Segmentation](./delivery/hls-segmentation.md) - Manifest and segment generation

### Performance & Optimization
- [Memory Efficiency](./performance/memory-efficiency.md) - Pooling, GC tuning, leak prevention
- [Encoding Performance](./performance/encoding-performance.md) - GPU acceleration, presets, quality tuning

### Debugging & Troubleshooting
- [Frame Leaks](./gotchas/frame-leaks.md) - Detection and prevention of memory leaks
- [Encoder Bottlenecks](./gotchas/encoder-bottlenecks.md) - Performance issues and solutions
- [Error Handling Strategy](./patterns/error-handling-strategy.md) - Error classification and recovery

### Architecture & Management
- [Stream Lifecycle](./streams/stream-lifecycle.md) - State management and control
- [Design Patterns](./patterns/index.md) - Common patterns and idioms

### Optional Topics (Index-only)
- [API & Control](./api/index.md) - REST API documentation
- [Storage & Persistence](./storage/index.md) - DVR, recording, segment storage
- [Clustering](./cluster/index.md) - Multi-node scaling and distribution
- [Ingest Protocols](./ingest/index.md) - RTMP/RTSP/HTTP input handling

## Learning Paths

### If You're Building an Encoder
1. [patterns/ffgo-integration-pattern.md](./patterns/ffgo-integration-pattern.md)
2. [media/decoder-lifecycle.md](./media/decoder-lifecycle.md)
3. [media/encoder-lifecycle.md](./media/encoder-lifecycle.md)
4. [performance/encoding-performance.md](./performance/encoding-performance.md)

### If You're Implementing ABR
1. [transcoding/adaptive-bitrate.md](./transcoding/adaptive-bitrate.md)
2. [media/frame-pipeline.md](./media/frame-pipeline.md)
3. [performance/memory-efficiency.md](./performance/memory-efficiency.md)
4. [delivery/hls-segmentation.md](./delivery/hls-segmentation.md)

### If You're Optimizing Performance
1. [performance/index.md](./performance/index.md)
2. [performance/encoding-performance.md](./performance/encoding-performance.md)
3. [performance/memory-efficiency.md](./performance/memory-efficiency.md)
4. [gotchas/encoder-bottlenecks.md](./gotchas/encoder-bottlenecks.md)

### If You're Debugging Issues
1. [gotchas/index.md](./gotchas/index.md)
2. [patterns/error-handling-strategy.md](./patterns/error-handling-strategy.md)
3. [gotchas/frame-leaks.md](./gotchas/frame-leaks.md)
4. [gotchas/encoder-bottlenecks.md](./gotchas/encoder-bottlenecks.md)

## Key Concepts

**Stream**: Complete pipeline from ingest to delivery for one source
- Independent decoder, multiple encoders (one per variant)
- Error isolation, separate lifecycle
- Statistics and monitoring per-stream

**Variant**: Output encoding (720p, 480p, 360p)
- Separate encoder, resolution, bitrate
- Synchronized keyframes across variants
- Independent scheduling per variant

**Segment**: 6-10 second media chunk
- Independently playable
- Generated via stream copy (fast)
- Delivered via HTTP (HLS/DASH)

**Frame**: Decoded audio/video data
- Shared across stages via reference counting
- Pooled to reduce allocation overhead
- Explicit lifetime management required

## Design Principles

1. **Producer-Consumer**: Decoders produce frames, encoders consume
2. **Pipeline Architecture**: Decode → Scale → Filter → Encode → Segment
3. **Resource Pooling**: Pre-allocate frames, buffers, decoders
4. **Reference Counting**: Share frame data efficiently
5. **Goroutine Per Stream**: Independent processing, error isolation
6. **State Machines**: Strict transitions (Created → Opened → Streaming → Closed)
7. **Async Codec Processing**: Send frames, receive packets separately
8. **Observable**: Expose metrics, state, errors for monitoring

## Code Examples

All nuggets include realistic Go code examples that can be adapted for production:

```go
// Decoding
decoder, _ := ffgo.NewDecoder("rtmp://source.com/live")
frame, _ := decoder.ReadFrame()

// Encoding
encoder, _ := ffgo.NewEncoder("output.mp4")
encoder.AddVideoStream(ffgo.VideoEncoderConfig{
    Codec:   ffgo.CodecIDH264,
    BitRate: 5_000_000,
    Preset:  "fast",
})
encoder.EncodeFrame(frame)

// Stream copy (fast remuxing)
remuxer, _ := ffgo.NewRemuxer("input.mp4", "output.mkv")
remuxer.Remux()
```

## Performance Targets

Typical performance on modern hardware (8 cores, 1 GPU):

| Scenario | Performance |
|----------|-------------|
| 1080p 30fps to 4 variants | 5x real-time with GPU |
| Memory per stream | 100-200 MB |
| Encoding latency | < 100ms |
| Stream copy speed | 50-100x real-time |

## Common Errors and Quick Fixes

| Symptom | Cause | Solution |
|---------|-------|----------|
| Encoder returns EAGAIN | Input buffer full | Call ReceivePacket() loop |
| Memory grows over time | Frame leak | Add `ffgo.FrameFree(&frame)` calls |
| Frames not processed | CPU bottleneck | Enable GPU, reduce preset |
| A/V sync issues | Timestamp errors | Rescale PTS to output time base |
| Encoder stalls | Output backpressure | Add async buffering |

## Related Resources

- [FFmpeg Documentation](https://ffmpeg.org/documentation.html)
- [ffgo GitHub Repository](https://github.com/obinnaokechukwu/ffgo)
- [SRS (Simple Real-time Server)](https://github.com/ossrs/srs)
- [Nginx-RTMP Module](https://github.com/arut/nginx-rtmp-module)
- [HLS Specification](https://tools.ietf.org/html/draft-pantos-http-live-streaming)
- [DASH Specification](https://dashif.org/specs/)

## Conventions

- **Naming**: Functions with context: `func (x *X) Op(ctx context.Context) error`
- **Errors**: Fatal errors returned directly, EAGAIN checked with `ffgo.IsAgain()`
- **Cleanup**: Always defer resource cleanup with `defer close()`
- **Frames**: Caller responsible for freeing returned frames
- **Threads**: Decoder/Encoder NOT thread-safe, use per-stream goroutines

## Contributing

When adding to this vault:

1. Follow the nugget format (YAML frontmatter + markdown)
2. Include practical code examples
3. Add cross-references using "related" field
4. Keep nuggets focused (one concept per file)
5. Update index files when adding sections

## License

This documentation is part of the ffgo project and follows the same license.

---

**Start with [index.md](./index.md) for complete architecture overview.**
