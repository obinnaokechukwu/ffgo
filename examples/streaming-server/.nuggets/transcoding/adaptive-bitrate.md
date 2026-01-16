---
title: Adaptive Bitrate Transcoding Architecture
keywords: [abr, transcoding, variants, bitrate, multi-encoding]
tags: [core-concept, internal, advanced]
related: [../media/frame-pipeline.md, ../media/encoder-lifecycle.md, ../performance/encoding-performance.md]
source: [server/transcoding/abr_transcoder.go, server/transcoding/variant_manager.go]
---

# Adaptive Bitrate Transcoding Architecture

Adaptive bitrate (ABR) streaming allows clients to request different quality levels based on available bandwidth. The streaming server must simultaneously encode the same source into multiple variants (720p, 480p, 360p, etc.).

## Variant Definition

Each variant specifies output format and codec options:

```go
// In server/config/variants.yaml or programmatically
type VariantProfile struct {
    Name         string           // "720p", "480p", etc.
    Width        int
    Height       int
    BitRate      int64            // Target bitrate in bps
    FrameRate    ffgo.Rational
    Codec        ffgo.CodecID
    Preset       string           // "fast", "medium", "slow"
    CRF          int              // Quality (H.264/H.265)
    Profile      string           // codec profile
    HWDevice     string           // "cuda", "vaapi", etc.
}

type ABRProfile struct {
    Name     string             // "dash-standard", "hls-apple", etc.
    Variants []VariantProfile
}

// Standard ABR ladder
var StandardABR = ABRProfile{
    Name: "standard",
    Variants: []VariantProfile{
        {
            Name:    "1080p",
            Width:   1920,
            Height:  1080,
            BitRate: 5_000_000,  // 5 Mbps
            Codec:   ffgo.CodecIDH264,
        },
        {
            Name:    "720p",
            Width:   1280,
            Height:  720,
            BitRate: 2_500_000,  // 2.5 Mbps
        },
        {
            Name:    "480p",
            Width:   854,
            Height:  480,
            BitRate: 1_200_000,  // 1.2 Mbps
        },
        {
            Name:    "360p",
            Width:   640,
            Height:  360,
            BitRate: 600_000,   // 600 kbps
        },
    },
}
```

## Parallel Encoding Pipeline

Each variant is encoded independently in its own goroutine:

```go
type ABRTranscoder struct {
    SourceURL   string
    OutputDir   string
    Profile     ABRProfile

    // Shared input
    decoder     *ffgo.Decoder

    // Per-variant encoding
    variants    map[string]*VariantEncoder
    frames      chan *PipelineFrame  // Shared frame distribution
}

type VariantEncoder struct {
    name       string
    scaler     *ffgo.Scaler
    encoder    *ffgo.Encoder

    // Flow control
    pending    int32  // Frames queued
    maxPending int32
}

func (abr *ABRTranscoder) Start(ctx context.Context) error {
    // 1. Open input
    var err error
    abr.decoder, err = ffgo.NewDecoder(abr.SourceURL)
    if err != nil {
        return err
    }
    defer abr.decoder.Close()

    // 2. Initialize variant encoders
    for _, vp := range abr.Profile.Variants {
        encoder, err := abr.createVariantEncoder(vp)
        if err != nil {
            return fmt.Errorf("variant %s: %w", vp.Name, err)
        }
        abr.variants[vp.Name] = encoder
    }

    // 3. Start variant encoding goroutines
    errChan := make(chan error, len(abr.variants))
    for _, ve := range abr.variants {
        go func(ve *VariantEncoder) {
            errChan <- ve.Run(ctx)
        }(ve)
    }

    // 4. Decode and distribute frames
    if err := abr.distribute(ctx); err != nil {
        return err
    }

    // 5. Wait for all variants to complete
    for i := 0; i < len(abr.variants); i++ {
        if err := <-errChan; err != nil {
            return err
        }
    }

    return nil
}

func (abr *ABRTranscoder) distribute(ctx context.Context) error {
    for {
        // Read frame from input
        frame, err := abr.decoder.ReadFrame()
        if err != nil {
            if ffgo.IsEOF(err) {
                // Signal EOF to all variants
                for _, ve := range abr.variants {
                    ve.input <- nil
                }
                return nil
            }
            return err
        }

        // Duplicate frame reference for each variant
        for name, ve := range abr.variants {
            // Create reference (shares same buffers)
            cloned := ffgo.FrameAlloc()
            if err := ffgo.FrameRef(cloned, frame); err != nil {
                return err
            }

            // Check variant is not overwhelmed
            if atomic.LoadInt32(&ve.pending) >= ve.maxPending {
                ffgo.FrameUnref(cloned)
                return fmt.Errorf("variant %s overrun", name)
            }

            atomic.AddInt32(&ve.pending, 1)
            ve.input <- cloned
        }

        // Unreference original (other refs keep it alive)
        ffgo.FrameUnref(frame)
    }
}
```

## Per-Variant Encoding

Each variant scales and encodes in its own goroutine:

```go
func (ve *VariantEncoder) Run(ctx context.Context) error {
    defer ve.encoder.Close()

    for {
        select {
        case frame := <-ve.input:
            if frame == nil {
                // EOF
                return ve.encoder.Close()
            }

            // Scale to variant resolution
            scaled := ffgo.FrameAlloc()
            if err := ve.scaler.ScaleFrame(frame, scaled); err != nil {
                ffgo.FrameUnref(frame)
                ffgo.FrameFree(&scaled)
                return err
            }

            ffgo.FrameUnref(frame)  // Done with original

            // Encode scaled frame
            if err := ve.encoder.EncodeFrame(scaled); err != nil {
                ffgo.FrameFree(&scaled)
                return err
            }

            ffgo.FrameFree(&scaled)

            atomic.AddInt32(&ve.pending, -1)

        case <-ctx.Done():
            return ctx.Err()
        }
    }
}
```

## Keyframe Synchronization

A critical feature: all variants must have keyframes at the same timing. This is essential for:
- HLS variant switching (keyframes enable mid-segment switch)
- DASH period alignment
- Thumbnail extraction at consistent times

```go
type KeyframeSync struct {
    keyframeInterval time.Duration
    lastKeyframe     int64
    timeBase         ffgo.Rational
}

func (ks *KeyframeSync) ShouldInsertKeyframe(frame *PipelineFrame) bool {
    elapsed := (frame.PTS - ks.lastKeyframe) * int64(ks.timeBase.Den) / int64(ks.timeBase.Num)

    if elapsed >= int64(ks.keyframeInterval.Milliseconds())*1000 {
        ks.lastKeyframe = frame.PTS
        return true
    }

    return false
}

// In variant encoder
if ve.keyframeSync.ShouldInsertKeyframe(pf) {
    ve.encoder.SetForceKeyframe()  // Force next encoded packet to be keyframe
}

if err := ve.encoder.EncodeFrame(pf.Frame); err != nil {
    return err
}
```

## CPU Optimization with Hardware Acceleration

When encoding multiple variants, CPU becomes bottleneck. The server distributes encoding across available GPUs:

```go
type HWAccelStrategy struct {
    availableDevices []string  // ["cuda:0", "cuda:1", "vaapi"]
    deviceIndex      int
}

func (has *HWAccelStrategy) selectDeviceForVariant(vp VariantProfile) string {
    // Assign GPU in round-robin fashion
    device := has.availableDevices[has.deviceIndex%len(has.availableDevices)]
    has.deviceIndex++
    return device
}

// In ABRTranscoder.createVariantEncoder
func (abr *ABRTranscoder) createVariantEncoder(vp VariantProfile) (*VariantEncoder, error) {
    hwDevice := abr.hwStrategy.selectDeviceForVariant(vp)

    encoder, err := ffgo.NewEncoder(abr.outputPath(vp.Name))
    if err != nil {
        return nil, err
    }

    encoder.AddVideoStream(ffgo.VideoEncoderConfig{
        Codec:     vp.Codec,
        Width:     vp.Width,
        Height:    vp.Height,
        BitRate:   vp.BitRate,
        FrameRate: vp.FrameRate,
        Preset:    vp.Preset,
        CRF:       vp.CRF,
        HWDevice:  hwDevice,  // GPU per variant
    })

    return encoder, nil
}
```

## Output Strategy

The ABR transcoder produces files or segments for each variant:

```go
// File-based output (for storage/VOD)
func (abr *ABRTranscoder) outputPath(variant string) string {
    return filepath.Join(abr.OutputDir, fmt.Sprintf("%s.mp4", variant))
}

// Segment-based output (for HLS/DASH)
type SegmentedOutput struct {
    variant    string
    segmentDir string
    encoder    *ffgo.Encoder
    currentSeg *SegmentFile
    segNum     int
}

func (so *SegmentedOutput) OnPacket(pkt ffgo.Packet) error {
    if isKeyframe(pkt) && so.shouldStartNewSegment() {
        so.closeCurrentSegment()
        so.startNewSegment()
    }

    return so.currentSeg.WritePacket(pkt)
}
```

## Performance Metrics

With proper ABR setup on modern hardware:

```
Input: 4K 30fps H.264 source

Single-GPU (NVIDIA RTX 3090):
  - 1080p + 720p + 480p: ~120 FPS (4x real-time)
  - 1080p + 720p + 480p + 360p: ~80 FPS

Dual-GPU:
  - Same variants: ~240 FPS (8x real-time)

CPU-only (8-core i7):
  - 720p + 480p only: ~30 FPS (1x real-time)
```

See [encoding performance](../performance/encoding-performance.md) for detailed optimization strategies and [HLS segmentation](../delivery/hls-segmentation.md) for how segments are generated from encoded variants.
