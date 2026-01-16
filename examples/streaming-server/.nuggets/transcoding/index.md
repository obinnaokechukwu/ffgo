---
title: Transcoding and Encoding Architecture
keywords: [transcoding, encoding, bitrate, codec, optimization]
tags: [index]
---

# Transcoding and Encoding Architecture

This section covers how the streaming server converts incoming streams into multiple output formats and qualities. Transcoding is the CPU/GPU-intensive heart of the system.

## Core Concepts

- **Adaptive Bitrate (ABR)** - Multi-variant encoding (720p, 480p, 360p)
- **Codec Selection** - Choosing H.264, H.265, VP9, AV1 based on requirements
- **Rate Control** - VBR, CBR, CRF quality modes
- **Hardware Acceleration** - GPU encoding strategies
- **Keyframe Synchronization** - Aligned keyframes across variants

## Design Patterns

1. **Producer-Consumer for Frames** - Decoder → Frame distribution → Variant encoders

2. **Per-Variant Goroutines** - Each variant encodes independently with its own scaler/encoder

3. **Shared Input, Multiplexed Output** - One decoder feeds multiple encoders via frame reference duplication

4. **Hardware Device Pool** - Distribute encoding load across available GPUs

## Architecture

```
Input Source
     ↓
Decoder (HWACCEL)
     ↓
Frame Distribution (Reference counting)
     ↓
┌────────────────┬────────────────┬────────────────┐
│                │                │                │
Scaler (720p)  Scaler (480p)  Scaler (360p)
│                │                │
Encoder (5Mbps) Encoder (2.5Mbps) Encoder (1Mbps)
│                │                │
Output File    Output File    Output File
```

## Common Patterns

**Starting ABR transcoding:**

```go
transcoder := &ABRTranscoder{
    SourceURL: "rtmp://source.com/live",
    OutputDir: "/segments",
    Profile:   StandardABR,
}

if err := transcoder.Start(ctx); err != nil {
    log.Fatal(err)
}
```

**Custom variant profile:**

```go
customProfile := ABRProfile{
    Name: "mobile",
    Variants: []VariantProfile{
        {
            Name:    "480p",
            Width:   854,
            Height:  480,
            BitRate: 1_500_000,
            Codec:   ffgo.CodecIDH264,
        },
        {
            Name:    "240p",
            Width:   426,
            Height:  240,
            BitRate: 400_000,
        },
    },
}
```

## Key Considerations

**CPU/GPU Bottleneck:** Transcoding is the most resource-intensive part. See [encoding performance](../performance/encoding-performance.md) for optimization strategies.

**Keyframe Alignment:** All variants must insert keyframes at the same timestamps for ABR switching to work properly.

**Memory Pressure:** Encoding multiple variants simultaneously uses significant memory. See [memory efficiency](../performance/memory-efficiency.md).

**Codec Selection:** H.264 has broad compatibility but poor compression. H.265 saves 40% bitrate but has licensing issues. VP9/AV1 are open but slower to encode.

## Related Topics

- [Encoder lifecycle](../media/encoder-lifecycle.md) - Low-level encoding mechanics
- [Encoding performance](../performance/encoding-performance.md) - Optimization techniques
- [HLS segmentation](../delivery/hls-segmentation.md) - Integration with HLS output
- [Ingest](../ingest/index.md) - Input pipeline that feeds transcoding
