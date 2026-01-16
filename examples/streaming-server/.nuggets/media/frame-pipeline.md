---
title: Frame Pipeline and Buffer Management
keywords: [frame, pipeline, buffers, memory, reference-counting]
tags: [core-concept, internal, advanced]
related: [../performance/memory-efficiency.md, ./decoder-lifecycle.md, ./encoder-lifecycle.md]
source: [server/media/frame_pool.go, server/media/pipeline.go]
---

# Frame Pipeline and Buffer Management

Frames are the fundamental data unit flowing through the streaming server. Understanding frame lifetime and buffer management is critical for memory efficiency and avoiding leaks.

## ffgo Frame Semantics

ffgo frames are opaque pointers wrapping AVFrame structures from FFmpeg:

```go
// ffgo.Frame is unsafe.Pointer internally
type Frame unsafe.Pointer

// Decoding returns a frame holding video/audio data
frame, _ := decoder.ReadFrame()

// Frame can be passed to processing stages
scaler.ScaleFrame(srcFrame, dstFrame)

// Memory must be explicitly released
ffgo.FrameUnref(frame)  // Unreference internal buffers
ffgo.FrameFree(&frame)  // Free the AVFrame structure itself
```

**Critical:** Every allocated frame must be freed. The streaming server uses `defer`-based cleanup patterns to prevent leaks.

## Reference Counting

ffgo frames use FFmpeg's internal reference counting:

```go
// When you decode a frame
frame, _ := decoder.ReadFrame() // refcount = 1 (decoder owns)

// Unreference decrements count, deallocates at 0
ffgo.FrameUnref(frame)      // refcount = 0, buffers freed

// To pass frame ownership without copying buffers
ffgo.FrameRef(dstFrame, srcFrame)  // dstFrame now references same buffers, refcount++
```

**Important:** When using FrameRef to alias frame data, both frames share the same underlying buffers. Unreferencing one affects the other.

## Pipeline Architecture

The streaming server uses a pipeline pattern to flow frames through processing stages:

```go
type FramePipeline struct {
    // Input stage
    decoder chan<- *PipelineFrame

    // Processing stages (arranged as chain)
    scaler  *ScalerStage
    filters *FilterStage

    // Output stage
    encoder chan<- *PipelineFrame

    // Control
    ctx    context.Context
    cancel context.CancelFunc
}

// Wrapped frame with metadata
type PipelineFrame struct {
    Frame          ffgo.Frame
    StreamIndex    int
    PTS            int64
    Duration       int64
    TimeBase       ffgo.Rational
    KeyFrame       bool

    // Reference tracking
    refCount       int32
    mu             sync.Mutex
}

func (pf *PipelineFrame) Retain() {
    atomic.AddInt32(&pf.refCount, 1)
}

func (pf *PipelineFrame) Release() {
    if atomic.AddInt32(&pf.refCount, -1) == 0 {
        // All stages done, free frame
        ffgo.FrameUnref(pf.Frame)
        ffgo.FrameFree(&pf.Frame)
    }
}
```

## Stage Pattern

Each pipeline stage is a goroutine consuming and producing PipelineFrames:

```go
type ScalerStage struct {
    input  <-chan *PipelineFrame
    output chan<- *PipelineFrame

    scaler *ffgo.Scaler
    srcFmt ffgo.PixelFormat
    dstFmt ffgo.PixelFormat
}

func (s *ScalerStage) Run(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case pf := <-s.input:
            if pf == nil {
                // EOF signal
                close(s.output)
                return nil
            }

            // Create output frame
            outFrame := ffgo.FrameAlloc()
            ffgo.AVUtil.FrameGetBuffer(outFrame, 1)

            // Scale
            if err := s.scaler.ScaleFrame(pf.Frame, outFrame); err != nil {
                pf.Release()
                ffgo.FrameFree(&outFrame)
                return err
            }

            // Create output pipeline frame
            outPF := &PipelineFrame{
                Frame:       outFrame,
                StreamIndex: pf.StreamIndex,
                PTS:         pf.PTS,
                Duration:    pf.Duration,
                TimeBase:    pf.TimeBase,
                KeyFrame:    pf.KeyFrame,
            }
            outPF.Retain()

            select {
            case s.output <- outPF:
                pf.Release()
            case <-ctx.Done():
                pf.Release()
                ffgo.FrameFree(&outFrame)
                return ctx.Err()
            }
        }
    }
}
```

## Memory Pooling for Performance

Frame allocation is expensive. The server maintains pools of pre-allocated frames:

```go
type FramePool struct {
    width    int
    height   int
    pixFmt   ffgo.PixelFormat
    available chan ffgo.Frame
    size     int
}

func NewFramePool(width, height int, pixFmt ffgo.PixelFormat, size int) (*FramePool, error) {
    fp := &FramePool{
        width:     width,
        height:    height,
        pixFmt:    pixFmt,
        available: make(chan ffgo.Frame, size),
        size:      size,
    }

    // Pre-allocate frames
    for i := 0; i < size; i++ {
        frame := ffgo.FrameAlloc()
        ffgo.AVUtil.SetFrameWidth(frame, int32(width))
        ffgo.AVUtil.SetFrameHeight(frame, int32(height))
        ffgo.AVUtil.SetFrameFormat(frame, int32(pixFmt))

        if err := ffgo.AVUtil.FrameGetBuffer(frame, 1); err != nil {
            return nil, fmt.Errorf("frame buffer alloc failed: %w", err)
        }

        fp.available <- frame
    }

    return fp, nil
}

func (fp *FramePool) Acquire(ctx context.Context) (ffgo.Frame, error) {
    select {
    case frame := <-fp.available:
        return frame, nil
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
        // Pool exhausted, allocate new frame
        frame := ffgo.FrameAlloc()
        ffgo.AVUtil.SetFrameWidth(frame, int32(fp.width))
        ffgo.AVUtil.SetFrameHeight(frame, int32(fp.height))
        ffgo.AVUtil.SetFrameFormat(frame, int32(fp.pixFmt))
        if err := ffgo.AVUtil.FrameGetBuffer(frame, 1); err != nil {
            ffgo.FrameFree(&frame)
            return nil, err
        }
        return frame, nil
    }
}

func (fp *FramePool) Release(frame ffgo.Frame) error {
    // Clear frame state for reuse
    ffgo.FrameUnref(frame)

    select {
    case fp.available <- frame:
        return nil
    default:
        // Pool full, dispose
        ffgo.FrameFree(&frame)
        return nil
    }
}

func (fp *FramePool) Close() error {
    close(fp.available)
    for frame := range fp.available {
        ffgo.FrameFree(&frame)
    }
    return nil
}
```

## Handling Different Formats

In adaptive bitrate streaming, frames are processed into multiple output formats simultaneously:

```go
type AdaptiveTranscoder struct {
    decoder       *ffgo.Decoder

    // Multiple scalers for different bitrates
    scalers       map[string]*ffgo.Scaler  // "720p", "480p", "360p"
    encoders      map[string]*ffgo.Encoder

    // Per-stream output pipelines
    pipelines     map[string]*FramePipeline
}

func (at *AdaptiveTranscoder) TranscodeFrame(frame ffgo.Frame) error {
    // Clone frame for each output variant
    for variant, pipeline := range at.pipelines {
        // Create a reference to the frame
        cloned := ffgo.FrameAlloc()
        if err := ffgo.FrameRef(cloned, frame); err != nil {
            ffgo.FrameFree(&cloned)
            return err
        }

        pf := &PipelineFrame{
            Frame: cloned,
            // ... metadata
        }
        pf.Retain()

        select {
        case pipeline.decoder <- pf:
        default:
            pf.Release()
            return fmt.Errorf("pipeline %s backpressure", variant)
        }
    }

    return nil
}
```

## Key Invariants

1. **Every allocated frame must be freed.** Track ownership carefully across goroutine boundaries.

2. **FrameRef creates shared references.** Multiple frames pointing to same buffers requires careful coordination.

3. **FrameUnref is idempotent.** Calling FrameUnref multiple times is safe but unnecessary.

4. **Pool frames must be unreferenced before returning.** Always call FrameUnref before releasing to pool.

5. **Context passing.** Frame metadata (PTS, stream index) must be preserved through the pipeline.

See [memory efficiency](../performance/memory-efficiency.md) for GC tuning and [frame leaks](../gotchas/frame-leaks.md) for debugging memory issues.
