---
title: Stream Copy Optimization for Fast Remuxing
keywords: [remuxing, stream-copy, zero-copy, performance]
tags: [pattern, guide, advanced]
related: [./decoder-lifecycle.md, ./encoder-lifecycle.md, ../performance/encoding-performance.md]
source: [server/media/remuxer.go, server/media/fast_transcode.go]
---

# Stream Copy Optimization for Fast Remuxing

When the input and output formats use the same codec, decoding and re-encoding wastes CPU. Stream copy mode bypasses codec processing entirely, achieving 50-100x faster remuxing.

## When to Use Stream Copy

Stream copy is the zero-copy fast path:

```
Input Format [H.264 packets] ---> Output Format [H.264 packets]
             (no decode/encode)
```

**Ideal for:**
- Format conversion (MP4 → MKV, both H.264)
- HLS segment generation (extract GOPs from MP4)
- DASH packaging (repackage H.264 to DASH manifests)
- Metadata addition without re-encoding
- Stream forwarding/recording

**Cannot use for:**
- Changing codec (H.264 → VP9)
- Changing resolution/bitrate
- Adding filters (watermark, scaling)
- Burning subtitles

## Implementation with ffgo

ffgo provides the Remuxer type for stream copy:

```go
// Fast path: direct stream copy
remuxer, err := ffgo.NewRemuxer(inputFile, outputFile)
if err != nil {
    return err
}
defer remuxer.Close()

// Metadata is automatically preserved
if err := remuxer.Remux(); err != nil {
    return err
}
```

However, for more control (selective stream copy, metadata modification), use the manual approach:

```go
type StreamCopyRemuxer struct {
    decoder *ffgo.Decoder
    encoder *ffgo.Encoder

    // Per-stream copy flags
    copyVideo bool
    copyAudio bool

    // Stream index mapping (input -> output)
    streamMap map[int]int
}

func (r *StreamCopyRemuxer) Remux() error {
    // Open input
    if err := r.decoder.Open(r.inputURL); err != nil {
        return err
    }
    defer r.decoder.Close()

    // Initialize output encoder
    if err := r.encoder.WriteHeader(); err != nil {
        return err
    }
    defer r.encoder.Close()

    // For stream copy, use special packet forwarding mode
    return r.copyPackets()
}

func (r *StreamCopyRemuxer) copyPackets() error {
    for {
        pkt, err := r.decoder.ReadPacket()  // Raw packet, no decode
        if err != nil {
            if ffgo.IsEOF(err) {
                break
            }
            return err
        }

        // Check if this stream should be copied
        srcStreamIdx := avcodec.GetPacketStreamIndex(pkt)
        if !r.shouldCopyStream(srcStreamIdx) {
            avcodec.FreePacket(pkt)
            continue
        }

        // Map to output stream index
        dstStreamIdx := r.streamMap[srcStreamIdx]
        avcodec.SetPacketStreamIndex(pkt, int32(dstStreamIdx))

        // Timestamp rescaling (critical for stream copy)
        r.rescalePacketTimestamps(pkt, srcStreamIdx, dstStreamIdx)

        // Write to output
        if err := r.encoder.WritePacket(pkt); err != nil {
            avcodec.FreePacket(pkt)
            return err
        }

        avcodec.FreePacket(pkt)
    }

    return r.encoder.Close()
}
```

## Decoder vs Demuxer for Stream Copy

The streaming server has two paths for reading packets:

```go
// Path 1: Decoder (frame-oriented, does decode)
decoder, _ := ffgo.NewDecoder("input.mp4")
for {
    frame, _ := decoder.ReadFrame()  // Decodes to AVFrame
    // Process frame...
}

// Path 2: Demuxer (packet-oriented, no decode)
demuxer, _ := ffgo.NewDemuxer("input.mp4")
for {
    pkt, _ := demuxer.ReadPacket()  // Raw packet, no codec processing
    // Forward packet to output...
}
```

The Demuxer is the underlying transport layer. Decoder wraps it with codec access. For stream copy, use Demuxer directly.

## Timestamp Handling in Stream Copy

This is the most error-prone part of remuxing. Each stream has a time_base:

```go
// Example: H.264 in MP4 has time_base = 1/24000, H.264 in MKV = 1/1000
// Same packet's PTS=1000000 means:
// MP4: 1000000/24000 = 41.67 seconds
// MKV: 1000000/1000 = 1000 seconds (WRONG!)

func (r *StreamCopyRemuxer) rescalePacketTimestamps(pkt ffgo.Packet, srcIdx, dstIdx int) {
    srcTimeBase := r.decoder.GetStreamTimeBase(srcIdx)
    dstTimeBase := r.encoder.GetStreamTimeBase(dstIdx)

    // Rescale PTS: convert from source time base to destination time base
    pts := avcodec.GetPacketPTS(pkt)
    pts = avformat.RescaleTimestamp(pts, srcTimeBase, dstTimeBase)
    avcodec.SetPacketPTS(pkt, pts)

    // Rescale DTS (decode timestamp) similarly
    dts := avcodec.GetPacketDTS(pkt)
    dts = avformat.RescaleTimestamp(dts, srcTimeBase, dstTimeBase)
    avcodec.SetPacketDTS(pkt, dts)

    // Rescale duration
    duration := avcodec.GetPacketDuration(pkt)
    duration = avformat.RescaleTimestamp(duration, srcTimeBase, dstTimeBase)
    avcodec.SetPacketDuration(pkt, duration)
}
```

**Without rescaling:** Output will have incorrect playback speed, seeking issues, and A/V sync problems.

## Selecting Which Streams to Copy

The server might want to copy video but re-encode audio:

```go
type SelectiveRemuxer struct {
    decoder  *ffgo.Decoder
    encoder  *ffgo.Encoder

    copyStreams   map[int]bool  // Stream index -> should copy
    encodeStreams map[int]*EncodeConfig
}

func (r *SelectiveRemuxer) Remux() error {
    // First pass: setup output streams
    numInputStreams := r.decoder.GetNumStreams()
    for i := 0; i < numInputStreams; i++ {
        streamInfo := r.decoder.GetStreamInfo(i)

        if r.copyStreams[i] {
            // Stream copy: add stream without codec
            r.addStreamCopy(i, streamInfo)
        } else if encCfg, ok := r.encodeStreams[i]; ok {
            // Re-encode: add stream with encoder
            r.addStreamEncode(i, streamInfo, encCfg)
        }
        // Else: skip this stream
    }

    // Second pass: process packets/frames
    for {
        pkt, err := r.decoder.ReadPacket()
        if ffgo.IsEOF(err) {
            break
        }

        srcIdx := avcodec.GetPacketStreamIndex(pkt)

        if r.copyStreams[srcIdx] {
            // Direct copy
            r.copyPacket(pkt, srcIdx)
        } else if _, ok := r.encodeStreams[srcIdx]; ok {
            // Decode and re-encode
            frame, _ := r.decodePacket(pkt, srcIdx)
            r.encodeFrame(frame, srcIdx)
        }
    }

    return r.encoder.Close()
}
```

## HLS Segment Generation via Stream Copy

HLS requires splitting streams at keyframes:

```go
type HLSSegmenter struct {
    decoder       *ffgo.Decoder
    encoder       *ffgo.Encoder
    segmentDir    string
    segmentDur    time.Duration
    segments      []string

    currentSegment *os.File
    segmentStart   int64
    lastKeyframe   int64
}

func (hs *HLSSegmenter) Segmentize() error {
    for {
        pkt, err := hs.decoder.ReadPacket()
        if ffgo.IsEOF(err) {
            hs.closeSegment()
            return hs.writePlaylist()
        }

        pts := avcodec.GetPacketPTS(pkt)
        isKeyFrame := avcodec.IsPacketKeyFrame(pkt)

        // Check if we should start new segment
        if isKeyFrame && (pts - hs.segmentStart) > hs.segmentDur {
            hs.closeSegment()
            hs.startNewSegment()
            hs.segmentStart = pts
        }

        // Write packet to current segment (stream copy)
        if err := hs.currentSegment.WritePacket(pkt); err != nil {
            return err
        }

        hs.lastKeyframe = pts
    }
}

func (hs *HLSSegmenter) startNewSegment() error {
    segNum := len(hs.segments)
    filename := filepath.Join(hs.segmentDir, fmt.Sprintf("segment_%03d.m4s", segNum))

    file, err := os.Create(filename)
    if err != nil {
        return err
    }

    encoder := ffgo.NewEncoder(file)
    // Copy codec params from decoder
    // ... setup encoder with stream copy mode
    encoder.WriteHeader()

    hs.currentSegment = encoder
    hs.segments = append(hs.segments, filename)
    return nil
}
```

## Performance Characteristics

Benchmarks show the performance difference:

```
Input: 1-hour H.264 4K stream

Decode-Recode (full transcode):  ~15 minutes (GPU assisted)
Stream Copy (remux):              ~30 seconds (CPU < 5%)

Speedup: 30x faster
CPU Savings: 95%+
```

Stream copy is essential for:
- Real-time transcoding of multiple variants
- Recording live streams while encoding delivery copies
- Format conversion in edge CDN nodes

See [adaptive bitrate](../transcoding/adaptive-bitrate.md) for how stream copy integrates into ABR pipelines.
