---
title: Ingest and Input Handling
keywords: [ingest, input, rtmp, rtsp, protocol]
tags: [index]
---

# Ingest and Input Handling

This section covers how the streaming server accepts incoming streams from broadcasters and sources.

## Input Protocols

- **RTMP (Real Time Messaging Protocol)** - Legacy but widely used for live ingest
- **RTSP (Real Time Streaming Protocol)** - IP camera standard
- **HTTP** - Progressive uploads, HLS/DASH input
- **File-based** - Local files, NFS, cloud storage
- **UDP** - Custom protocols, very low latency

## Architecture

```
Broadcaster
    ↓
Network Protocol (RTMP/RTSP/HTTP)
    ↓
Input Buffer
    ↓
Protocol Demuxer
    ↓
ffgo.Decoder
    ↓
Frame Stream → Transcoding
```

## Key Concepts

**Authentication:** User/password credentials for ingest streams

**Bitrate Adaptation:** Input can change bitrate or resolution mid-stream

**Error Recovery:** Reconnect on network failures, maintain playback continuity

**Stream Validation:** Check codec compatibility, format, metadata

## Common Patterns

**RTMP ingest server:**

```go
rtmpServer := &RTMPServer{
    listenAddr: ":1935",
    streams:    make(map[string]*LiveStream),
}

rtmpServer.Start()  // Listen for RTMP pushes
```

**Stream lifecycle:**

```go
stream := &LiveStream{
    name:      "mystream",
    startTime: time.Now(),
    source:    remoteURL,
}

stream.Start(ctx)  // Decode and transcode
stream.Stop()      // Graceful shutdown
```

## Related Topics

- [Decoder lifecycle](../media/decoder-lifecycle.md) - Input parsing with ffgo
- [Adaptive bitrate](../transcoding/adaptive-bitrate.md) - Transcoding after ingest
