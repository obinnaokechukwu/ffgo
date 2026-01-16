---
title: Media Processing Internals
keywords: [media, ffgo, processing, pipeline]
tags: [index]
---

# Media Processing Internals

This section documents how the streaming server processes media using ffgo. It covers the core data flow from input sources through transcoding to output delivery.

## Core Concepts

- **Decoder Lifecycle** - Input format parsing, codec initialization, and frame extraction
- **Encoder Lifecycle** - Output setup, header writing, and packet muxing
- **Frame Pipeline** - Buffer management, multi-stage processing, memory pooling
- **Stream Copy Optimization** - Zero-copy remuxing for format conversion and segments

## Design Patterns

The media layer uses several key patterns:

1. **State Machines** - Decoders and encoders follow strict state transitions (Created → Opened → Streaming → Closed)

2. **Asynchronous Codec Processing** - ffgo uses send/receive semantics: send frames/packets, receive results later

3. **Custom I/O** - Network protocols (RTMP, RTSP) are abstracted through io.Reader/Writer interfaces

4. **Hardware Acceleration** - GPU devices are transparent to higher layers, selected at decoder/encoder initialization

5. **Reference Counting** - Frame buffers use FFmpeg's internal refcounting to manage shared data

## Integration Points

- **Transcoding** - Media layer feeds scalers and filters
- **Ingest** - RTMP/RTSP decoders sit on top of media decoders
- **Delivery** - Encoders produce packets for HLS/DASH segmentation

## Common Patterns

**Processing a stream:**

```go
decoder, _ := ffgo.NewDecoder(sourceURL)
defer decoder.Close()

for {
    frame, err := decoder.ReadFrame()
    if ffgo.IsEOF(err) {
        break
    }
    // Process frame...
}
```

**Encoding with options:**

```go
encoder, _ := ffgo.NewEncoder(outputFile)
defer encoder.Close()

encoder.AddVideoStream(ffgo.VideoEncoderConfig{
    Codec:   ffgo.CodecIDH264,
    Width:   1280,
    Height:  720,
    BitRate: 5000000,  // 5 Mbps
})

encoder.WriteHeader()

for frame := range inputFrames {
    encoder.EncodeFrame(frame)
}
```

**Stream copy (fast path):**

```go
remuxer, _ := ffgo.NewRemuxer(input, output)
defer remuxer.Close()

remuxer.Remux()  // No transcoding needed
```

## Memory Considerations

Media processing is memory-intensive. Key principles:

- Frames are pooled to reduce allocation overhead
- Frame references are tracked across pipeline stages
- Decoded content is immediately consumed (not buffered)
- Encoder output is streamed to disk/network (not buffered)

See [memory efficiency](../performance/memory-efficiency.md) for detailed memory optimization strategies.
