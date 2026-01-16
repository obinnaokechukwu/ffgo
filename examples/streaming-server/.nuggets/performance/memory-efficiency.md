---
title: Memory Efficiency in Media Processing
keywords: [memory, pooling, gc, efficiency, buffers]
tags: [guide, internal, advanced]
related: [../media/frame-pipeline.md, ./encoding-performance.md, ../gotchas/frame-leaks.md]
source: [server/performance/memory.go, server/media/pool.go]
---

# Memory Efficiency in Media Processing

Media processing is inherently memory-intensive. A single 4K frame requires 33 MB of storage. Streaming 10 concurrent streams simultaneously means managing 330 MB just for frames. Careful memory management is essential for both performance and reliability.

## Frame Buffer Allocation

Raw frame buffers must be explicitly allocated in ffgo:

```go
// Allocation
frame := ffgo.FrameAlloc()  // Creates AVFrame structure, buffers uninitialized

// To use frame for actual data, must allocate buffers
if err := ffgo.AVUtil.FrameGetBuffer(frame, 1); err != nil {
    ffgo.FrameFree(&frame)
    return nil, err
}

// Frame is now ready for decode/process
```

**Size calculation:**

```go
func frameSizeBytes(width, height int, pixFmt ffgo.PixelFormat) int {
    switch pixFmt {
    case ffgo.PixelFormatYUV420P:
        return width * height * 3 / 2  // 1.5x
    case ffgo.PixelFormatYUV422P:
        return width * height * 2      // 2x
    case ffgo.PixelFormatRGB24:
        return width * height * 3      // 3x
    case ffgo.PixelFormatRGBA:
        return width * height * 4      // 4x
    default:
        return 0
    }
}

// Example: 4K YUV420P
// 3840 * 2160 * 1.5 = 12,441,600 bytes = 12.4 MB per frame
// At 30 FPS: ~372 MB/sec allocation rate
```

## Frame Pooling Pattern

Pre-allocating frames avoids GC pressure and runtime allocation costs:

```go
type FramePool struct {
    frames    chan ffgo.Frame
    size      int

    // Configuration
    width     int
    height    int
    pixFmt    ffgo.PixelFormat
}

func NewFramePool(width, height int, pixFmt ffgo.PixelFormat, poolSize int) (*FramePool, error) {
    fp := &FramePool{
        frames: make(chan ffgo.Frame, poolSize),
        size:   poolSize,
        width:  width,
        height: height,
        pixFmt: pixFmt,
    }

    // Pre-allocate all frames
    for i := 0; i < poolSize; i++ {
        frame := ffgo.FrameAlloc()
        ffgo.AVUtil.SetFrameWidth(frame, int32(width))
        ffgo.AVUtil.SetFrameHeight(frame, int32(height))
        ffgo.AVUtil.SetFrameFormat(frame, int32(pixFmt))

        if err := ffgo.AVUtil.FrameGetBuffer(frame, 1); err != nil {
            // Cleanup on error
            for j := 0; j < i; j++ {
                f := <-fp.frames
                ffgo.FrameFree(&f)
            }
            return nil, err
        }

        fp.frames <- frame
    }

    return fp, nil
}

// Non-blocking acquire (fallback to allocation if pool exhausted)
func (fp *FramePool) Acquire(ctx context.Context) (ffgo.Frame, error) {
    select {
    case frame := <-fp.frames:
        // Clear state for reuse
        ffgo.FrameUnref(frame)
        return frame, nil
    default:
        // Pool empty, allocate new frame (will be slower)
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
    select {
    case fp.frames <- frame:
        return nil
    default:
        // Pool full, dispose
        ffgo.FrameFree(&frame)
        return nil
    }
}

func (fp *FramePool) Close() error {
    close(fp.frames)
    for frame := range fp.frames {
        ffgo.FrameFree(&frame)
    }
    return nil
}
```

**Pool sizing heuristic:**

```
Pool size = max(2, ceil(decode_latency_ms / frame_interval_ms))

Example:
  - Input: 30 FPS = 33ms per frame
  - Decoder latency: 100ms (includes buffering)
  - Pool size: ceil(100/33) = 4 frames
```

## Garbage Collection Tuning

Go's GC can cause latency spikes with high allocation rates. Configure GC for media workloads:

```go
import "runtime/debug"

func init() {
    // For real-time media, reduce GC pause time
    debug.SetMaxStack(1 << 31)  // Allow large stacks for worker goroutines

    // Adjust GC trigger (default 100% growth)
    // Lower = more frequent GC (lower latency)
    // Higher = less frequent GC (lower CPU overhead)
    debug.SetGCPercent(50)  // GC when heap grows 50% from last GC
}
```

**GC Impact Example:**

```
Default GC (100% trigger):
  Heap size: 500 MB
  GC frequency: Every 1 GB allocated
  Pause time: 200-400ms (bad for live streams)

Tuned GC (50% trigger):
  Heap size: 500 MB
  GC frequency: Every 250 MB allocated
  Pause time: 50-100ms (better for live streams)

Trade-off: Slightly higher CPU (1-3%) for lower latency variance
```

## Reference Counting and Leaks

ffgo frames use reference counting internally. Improper handling causes leaks:

```go
// LEAK: Frame allocated but never freed
func badExample() error {
    frame, err := decoder.ReadFrame()
    if err != nil {
        return err
    }
    // LEAK: frame not freed!
    // Frame memory lost
    return nil
}

// CORRECT: Always free frames
func goodExample() error {
    frame, err := decoder.ReadFrame()
    if err != nil {
        return err
    }
    defer ffgo.FrameFree(&frame)

    // Frame automatically freed when function returns
    return nil
}

// Using defer with slice cleanup
func processFrames(frames []ffgo.Frame) {
    defer func() {
        for _, f := range frames {
            ffgo.FrameFree(&f)
        }
    }()

    for _, frame := range frames {
        processFrame(frame)
    }
}
```

## Memory Tracking and Metrics

Monitor memory usage to detect leaks:

```go
type MemoryMetrics struct {
    allocatedFrames  int32
    peakHeapSize     uint64
    totalAllocations int64
}

func (mm *MemoryMetrics) RecordFrameAlloc() {
    atomic.AddInt32(&mm.allocatedFrames, 1)
}

func (mm *MemoryMetrics) RecordFrameFree() {
    atomic.AddInt32(&mm.allocatedFrames, -1)
}

func (mm *MemoryMetrics) Monitor(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    var m runtime.MemStats

    for {
        select {
        case <-ticker.C:
            runtime.ReadMemStats(&m)

            frames := atomic.LoadInt32(&mm.allocatedFrames)
            log.Printf("Memory: Heap=%vMB Allocs=%v LiveFrames=%d",
                m.HeapAlloc/1024/1024,
                m.HeapAllocs,
                frames,
            )

            // Alert if frames spike
            if frames > 100 {
                log.Warnf("High frame count: %d (possible leak)", frames)
            }

        case <-ctx.Done():
            return
        }
    }
}
```

## Segment and Packet Pooling

Beyond frames, other buffers should be pooled:

```go
type BufferPool struct {
    buffers chan []byte
    size    int
}

func NewBufferPool(bufferSize, poolSize int) *BufferPool {
    bp := &BufferPool{
        buffers: make(chan []byte, poolSize),
        size:    bufferSize,
    }

    for i := 0; i < poolSize; i++ {
        bp.buffers <- make([]byte, bufferSize)
    }

    return bp
}

func (bp *BufferPool) Acquire() []byte {
    select {
    case buf := <-bp.buffers:
        return buf[:0]  // Reset but reuse underlying array
    default:
        return make([]byte, 0, bp.size)
    }
}

func (bp *BufferPool) Release(buf []byte) {
    if cap(buf) >= bp.size && cap(buf) <= bp.size*2 {  // Size sanity check
        select {
        case bp.buffers <- buf:
        default:
            // Pool full, discard
        }
    }
}
```

## Memory-Conscious Design

Key principles for memory efficiency:

1. **Stream, don't buffer** - Process frames immediately, don't keep them in queues
2. **Pool expensive allocations** - Frames, buffers, packets
3. **Reuse across stages** - Pass frame ownership through pipeline
4. **Monitor and alert** - Track heap size and frame counts
5. **Tune GC** - Balance between latency and CPU for your workload

See [frame leaks](../gotchas/frame-leaks.md) for debugging techniques and [encoding performance](./encoding-performance.md) for related optimization.
