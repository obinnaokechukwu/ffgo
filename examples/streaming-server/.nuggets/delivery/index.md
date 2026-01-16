---
title: Streaming Delivery Architecture
keywords: [delivery, hls, dash, manifest, playback]
tags: [index]
---

# Streaming Delivery Architecture

This section covers how the streaming server outputs encoded media to clients using HLS, DASH, and other delivery protocols.

## Delivery Protocols

- **HLS (HTTP Live Streaming)** - Apple standard, widely supported, m3u8 manifests
- **DASH (Dynamic Adaptive Streaming)** - ISO standard, very flexible, mpd manifests
- **WebRTC** - Real-time, low-latency, peer-to-peer capable
- **Progressive Download** - Simple HTTP file serving, no manifest

## Architecture

```
Transcoded Variants (H.264, H.265)
         ↓
   ┌─────────────┐
   │ Segmenter   │
   └─────────────┘
         ↓
   ┌─────────────────┬──────────────┐
   │                 │              │
Segments (.m4s)   Init (.m4i)   Manifest (.m3u8/.mpd)
   │                 │              │
   └─────────────────┴──────────────┘
         ↓
   HTTP Server
         ↓
   Client Players
```

## Key Concepts

**Segment:** Short duration media chunk (6-10s typical), independently playable

**Variant:** Encode at different bitrate/resolution for ABR switching

**Manifest:** Playlist describing available segments, variants, timing

**Playlist Rotation:** Live streams keep sliding window of segments (3-6 segments)

## Common Patterns

**Serving HLS playlist:**

```go
handler.HandleFunc("/live/stream.m3u8", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
    w.Header().Set("Cache-Control", "no-cache")
    fmt.Fprint(w, manifest.Render())
})
```

**Serving segments with caching:**

```go
handler.HandleFunc("/live/segment_{num}.m4s", func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "video/mp4")
    w.Header().Set("Cache-Control", "public, max-age=3600")
    http.ServeFile(w, r, segmentPath)
})
```

## Related Topics

- [HLS segmentation](./hls-segmentation.md) - HLS manifest and segment generation
- [Adaptive bitrate](../transcoding/adaptive-bitrate.md) - Producing variants
- [Stream copy optimization](../media/stream-copy-optimization.md) - Fast segment production
