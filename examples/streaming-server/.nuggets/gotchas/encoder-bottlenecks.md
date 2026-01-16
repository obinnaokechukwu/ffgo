---
title: Encoder Performance Bottlenecks and Solutions
keywords: [encoder, bottleneck, performance, stall, latency]
tags: [gotcha, guide, advanced]
related: [../performance/encoding-performance.md, ../performance/memory-efficiency.md]
source: [server/transcoding/bottleneck_detector.go]
---

# Encoder Performance Bottlenecks and Solutions

Encoding is the primary bottleneck in transcoding pipelines. Understanding why encoders stall and how to fix it is critical for maintaining real-time performance.

## Common Bottleneck Scenarios

### Scenario 1: Encoder Input Buffer Full (EAGAIN)

The encoder has a fixed internal buffer for pending frames. Once full, it returns EAGAIN:

```go
// SYMPTOM: EncodeFrame() returns "EAGAIN" repeatedly
func (e *Encoder) EncodeFrame(frame ffgo.Frame) error {
    if err := e.avcodec.SendFrame(frame); err != nil {
        if ffgo.IsAgain(err) {
            // Buffer full! Must flush packets first
            return fmt.Errorf("encoder buffer full")
        }
        return err
    }
    return e.flushPackets()
}

// ROOT CAUSE: Output packets not being retrieved fast enough
// FIX: Always loop to receive all available packets
for {
    pkt, err := encoder.ReceivePacket()
    if ffgo.IsAgain(err) {
        break  // No more packets available
    }
    if err != nil {
        return err
    }
    // Write packet to output immediately
}
```

**Detection:**

```go
type BottleneckDetector struct {
    eagainCount int32
    frameCount  int32
}

func (bd *BottleneckDetector) OnEAGAIN() {
    atomic.AddInt32(&bd.eagainCount, 1)
}

func (bd *BottleneckDetector) OnFrameProcessed() {
    atomic.AddInt32(&bd.frameCount, 1)
}

func (bd *BottleneckDetector) Report() {
    eagain := atomic.LoadInt32(&bd.eagainCount)
    frames := atomic.LoadInt32(&bd.frameCount)

    if eagain > frames/10 {
        log.Warnf("High EAGAIN rate: %d/%d (%.1f%%)",
            eagain, frames, float64(eagain)*100/float64(frames))
    }
}
```

### Scenario 2: CPU Bound (Encoding Too Slow)

The preset/codec combination is too slow for real-time:

```
Input: 1080p 30fps
Required encoding: 30 FPS
Encoder speed: 20 FPS (with "slow" preset)
Result: 10 FPS frame drop, accumulating input queue
```

**Symptoms:**
- `top` shows 100% CPU on encoder thread
- Frame queue grows unboundedly
- Output latency increases linearly over time

**Fix:**

```go
// Option 1: Faster preset
encoder.AddVideoStream(ffgo.VideoEncoderConfig{
    Preset: "fast",  // was "slow"
})

// Option 2: Lower quality (CRF)
encoder.AddVideoStream(ffgo.VideoEncoderConfig{
    CRF: 35,  // was 24 (lower = lower quality, faster encoding)
})

// Option 3: GPU acceleration
encoder.AddVideoStream(ffgo.VideoEncoderConfig{
    HWDevice: "cuda",  // 5-10x speedup
})

// Option 4: Reduce resolution
encoder.AddVideoStream(ffgo.VideoEncoderConfig{
    Width:  960,   // was 1920
    Height: 540,   // was 1080
})
```

### Scenario 3: GPU Memory Exhaustion

Multiple encoders sharing GPU memory can exceed VRAM:

```
4x 1080p H.264 encoders on NVIDIA RTX 3080 (10GB):
  - Context overhead: 100-200 MB per encoder = 400-800 MB
  - Frame buffers: 12 MB per encoder = 48 MB
  - Workspace: 500 MB-1 GB
  Total: ~2-2.5 GB (safe)

But with 8 encoders:
  - Context/workspace: ~4 GB
  - Frame buffers: ~96 MB
  Total: ~4+ GB (still OK)

However, with 4K encoding or very large bframes:
  - Can easily exceed 10GB available VRAM
  - GPU returns "out of memory" error
```

**Symptoms:**
- Encoder creation fails: "cuda: out of memory"
- Encoding stalls after some frames
- GPU memory near 100% (nvidia-smi)

**Detection and Fix:**

```go
func createEncoderWithFallback(cfg VideoEncoderConfig) (*Encoder, error) {
    // Try GPU first
    encoder, err := ffgo.NewEncoder(output)
    if err == nil {
        config := cfg
        config.HWDevice = "cuda"
        if err := encoder.AddVideoStream(config); err == nil {
            return encoder, nil
        }
    }

    log.Warnf("GPU encoding failed: %v, falling back to CPU", err)

    // Fall back to CPU
    return ffgo.NewEncoder(output)
}

// Monitor GPU memory
func monitorGPU(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            memUsed := getGPUMemoryUsed()  // nvidia-smi
            memTotal := getGPUMemoryTotal()

            if float64(memUsed)*100/float64(memTotal) > 90 {
                log.Warnf("GPU memory near limit: %d/%d MB",
                    memUsed, memTotal)

                // Could trigger fallback to CPU
                // Or reduce resolution
            }

        case <-ctx.Done():
            return
        }
    }
}
```

### Scenario 4: I/O Bound (Output Too Slow)

Writing output (segments, files) slower than encoding:

```
Encoding speed: 120 FPS
File write speed: 30 FPS (slow disk, network)
Result: Output buffer fills, encoder blocks
```

**Symptoms:**
- Disk/network saturated (iostat, iotop)
- WriteFrame() calls block
- CPU encoding thread idle while waiting for I/O

**Fix:**

```go
// Async output with buffering
type BufferedOutput struct {
    frames chan *OutputFrame
    writer io.Writer
}

func (bo *BufferedOutput) Run(ctx context.Context) error {
    for {
        select {
        case frame := <-bo.frames:
            // Write in separate goroutine
            if _, err := bo.writer.Write(frame.Data); err != nil {
                return err
            }

        case <-ctx.Done():
            return ctx.Err()
        }
    }
}

// Encoder doesn't block on I/O
func (encoder *Encoder) EncodeWithAsyncOutput(frame ffgo.Frame) error {
    if err := encoder.EncodeFrame(frame); err != nil {
        return err
    }

    // Packets go to async output (non-blocking)
    for {
        pkt, err := encoder.ReceivePacket()
        if ffgo.IsAgain(err) {
            break
        }
        if err != nil {
            return err
        }

        // Send to async writer (non-blocking if buffered)
        select {
        case asyncOutput.packets <- pkt:
        default:
            // Buffer full, wait
            asyncOutput.packets <- pkt
        }
    }

    return nil
}
```

### Scenario 5: Keyframe Interval Too Short

Forcing keyframes too frequently wastes bitrate and CPU:

```
Keyframe every 30 frames (1 second at 30 FPS):
  - H.264 IDR frame: ~50% larger than P-frame
  - Bitrate impact: +30-40%
  - CPU impact: +15-20%

Keyframe every 300 frames (10 seconds):
  - Better compression
  - Lower bitrate
  - Harder to seek, switch ABR variants
```

**Configuration:**

```go
encoder.AddVideoStream(ffgo.VideoEncoderConfig{
    GOPSize: 300,  // frames between keyframes
    // At 30 FPS, keyframe every 10 seconds
})

// Or in seconds
func setKeyframeInterval(encoder *ffgo.Encoder, seconds int, fps int) {
    gopSize := seconds * fps
    encoder.SetGOPSize(gopSize)
}
```

## Diagnostic Tools

### Tool 1: Frame Timing Profiler

```go
type FrameTimingProfile struct {
    mu              sync.RWMutex
    frameTimings    []time.Duration
    slowFrameCount  int
    slowFrameThresh time.Duration
}

func (ftp *FrameTimingProfile) RecordFrame(elapsed time.Duration) {
    ftp.mu.Lock()
    defer ftp.mu.Unlock()

    ftp.frameTimings = append(ftp.frameTimings, elapsed)

    if elapsed > ftp.slowFrameThresh {
        ftp.slowFrameCount++
        log.Warnf("Slow frame: %v (threshold: %v)", elapsed, ftp.slowFrameThresh)
    }

    // Keep rolling window
    if len(ftp.frameTimings) > 1000 {
        ftp.frameTimings = ftp.frameTimings[1:]
    }
}

func (ftp *FrameTimingProfile) Report() {
    ftp.mu.RLock()
    defer ftp.mu.RUnlock()

    if len(ftp.frameTimings) == 0 {
        return
    }

    sort.Slice(ftp.frameTimings, func(i, j int) bool {
        return ftp.frameTimings[i] < ftp.frameTimings[j]
    })

    p50 := ftp.frameTimings[len(ftp.frameTimings)/2]
    p95 := ftp.frameTimings[int(float64(len(ftp.frameTimings))*0.95)]
    p99 := ftp.frameTimings[int(float64(len(ftp.frameTimings))*0.99)]

    log.Infof("Frame timing: p50=%v p95=%v p99=%v slow=%d",
        p50, p95, p99, ftp.slowFrameCount)
}
```

### Tool 2: Queue Depth Monitoring

```go
func (encoder *Encoder) MonitorQueueDepth(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            depth := encoder.GetQueueDepth()  // Frames pending encode

            if depth > 100 {
                log.Warnf("Encoder queue depth: %d (backpressure)", depth)
            }

        case <-ctx.Done():
            return
        }
    }
}
```

## Prevention Strategies

1. **Pre-flight checks** - Test encoding speed before starting stream
2. **Monitoring** - Continuous frame timing and queue depth tracking
3. **Automatic fallback** - Switch to faster preset on bottleneck
4. **Resource limits** - Cap concurrent encoders per GPU
5. **Load shedding** - Drop frames if can't keep up (for live-only)

See [encoding performance](../performance/encoding-performance.md) for optimization techniques and [frame leaks](./frame-leaks.md) for related debugging.
