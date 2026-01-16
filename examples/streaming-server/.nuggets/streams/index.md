---
title: Stream Management and Control
keywords: [stream, management, lifecycle, control, monitoring]
tags: [index]
---

# Stream Management and Control

This section covers stream-level management, lifecycle, and health monitoring.

## Core Topics

- **Stream Lifecycle** - State transitions from creation through completion
- **Stream Manager** - Central management of multiple concurrent streams
- **Statistics** - Performance metrics and health indicators
- **Error Recovery** - Handling stream failures gracefully

## Architecture

Each streaming server manages multiple concurrent streams:

```
┌──────────────────┐
│  StreamManager   │
├──────────────────┤
│ Stream 1 (live1) │
│ Stream 2 (live2) │
│ Stream 3 (live3) │
└──────────────────┘
```

Each stream is independent with its own:
- Input decoder
- Output encoders (one per variant)
- Error isolation
- Lifecycle state

## Key Concepts

**Stream:** Complete processing pipeline from ingest to delivery for one source

**Variant:** Output encoding of a stream (720p, 480p, etc.)

**Segment:** Media chunk for delivery (6-10 seconds typical)

**Session:** User connection duration

## State Management

Streams follow a strict state machine to ensure clean transitions and resource management:

```
New → Starting → Running → Stopping → Stopped
        ↓                      ↑
        └─→ Error ──→ Recovery─┘
             ↓
          Failed
```

See [stream lifecycle](./stream-lifecycle.md) for implementation details.

## Common Patterns

**Starting a stream:**

```go
manager := NewStreamManager(10)

stream, _ := manager.CreateStream(StreamConfig{
    ID:        "mystream",
    SourceURL: "rtmp://source.com/app/stream",
    OutputDir: "/var/media/streams",
})
```

**Monitoring streams:**

```go
for {
    streams := manager.GetAllStreams()
    for _, stream := range streams {
        stats := stream.GetStats()
        log.Printf("Stream %s: %d frames processed", stream.ID, stats.FramesIn)
    }
}
```

**Stopping a stream:**

```go
manager.TerminateStream("mystream")
```

## Related Topics

- [Decoder lifecycle](../media/decoder-lifecycle.md) - Input handling
- [Adaptive bitrate](../transcoding/adaptive-bitrate.md) - Transcoding
- [Error handling strategy](../patterns/error-handling-strategy.md) - Error recovery
