---
title: Debugging Frame Memory Leaks
keywords: [debugging, leaks, memory, frame, troubleshooting]
tags: [gotcha, guide, advanced]
related: [../media/frame-pipeline.md, ../performance/memory-efficiency.md]
source: [server/debug/memory_leak_detector.go]
---

# Debugging Frame Memory Leaks

Frame memory leaks are the most common issue in streaming servers. A single leaked frame is 12-50 MB depending on resolution. In high-throughput systems, this quickly exhausts memory.

## Common Leak Patterns

### Pattern 1: Missing FrameFree in Error Paths

```go
// LEAK: Frame not freed on error
func processFrame(frame ffgo.Frame) error {
    if frame == nil {
        return fmt.Errorf("nil frame")  // LEAK: frame not freed!
    }

    // Process...
    return nil
}
```

**Fix:** Always defer cleanup:

```go
func processFrame(frame ffgo.Frame) error {
    defer ffgo.FrameFree(&frame)

    if frame == nil {
        return fmt.Errorf("nil frame")  // Frame freed by defer
    }

    // Process...
    return nil
}
```

### Pattern 2: Shared References

When using FrameRef for multiple consumers, all must release properly:

```go
// LEAK: Reference counting mismatch
func duplicateFrame(src ffgo.Frame) ([]ffgo.Frame, error) {
    clones := make([]ffgo.Frame, 3)

    for i := range clones {
        clone := ffgo.FrameAlloc()
        if err := ffgo.FrameRef(clone, src); err != nil {
            return nil, err  // LEAK: allocated clones not freed!
        }
        clones[i] = clone
    }

    return clones, nil
}
```

**Fix:** Cleanup on error:

```go
func duplicateFrame(src ffgo.Frame) ([]ffgo.Frame, error) {
    clones := make([]ffgo.Frame, 3)

    for i := range clones {
        clone := ffgo.FrameAlloc()
        if err := ffgo.FrameRef(clone, src); err != nil {
            // Cleanup previous clones
            for j := 0; j < i; j++ {
                ffgo.FrameFree(&clones[j])
            }
            ffgo.FrameFree(&clone)
            return nil, err
        }
        clones[i] = clone
    }

    return clones, nil
}
```

### Pattern 3: Goroutine Exit Without Cleanup

```go
// LEAK: Goroutine exits with pending frames
func (p *Pipeline) processAsync(frame ffgo.Frame) {
    go func() {
        // ... processing
        if err != nil {
            return  // LEAK: frame never freed!
        }
        // Process frame...
    }()
}
```

**Fix:** Ensure cleanup in all paths:

```go
func (p *Pipeline) processAsync(frame ffgo.Frame) {
    go func() {
        defer ffgo.FrameFree(&frame)

        // ... processing
        if err != nil {
            return  // Frame freed by defer
        }
        // Process frame...
    }()
}
```

### Pattern 4: Panic Without Cleanup

```go
// LEAK: Panic prevents defer execution if wrong
func dangerousFrame(frame ffgo.Frame) {
    defer ffgo.FrameFree(&frame)

    // Panic occurs BEFORE defer registration (unlikely but possible)
    panicErr := somethingFatal()

    // This defer runs, so OK
}
```

However, nested defers can cause issues:

```go
// LEAK: Multiple frames, only some deferred
func batchProcess(frames []ffgo.Frame) {
    for _, frame := range frames {
        defer ffgo.FrameFree(&frame)  // Wrong! Executes in reverse order on panic
        process(frame)
    }
}

// Correct: Use explicit cleanup
func batchProcess(frames []ffgo.Frame) {
    for _, frame := range frames {
        process(frame)
        ffgo.FrameFree(&frame)
    }
}
```

## Detection Techniques

### Technique 1: Frame Accounting

Track allocations vs. deallocations:

```go
type FrameTracker struct {
    allocated int32
    freed     int32
    mu        sync.Mutex
    active    map[unsafe.Pointer]struct{}
}

func (ft *FrameTracker) OnFrameAlloc() {
    atomic.AddInt32(&ft.allocated, 1)
}

func (ft *FrameTracker) OnFrameFree() {
    atomic.AddInt32(&ft.freed, 1)
}

func (ft *FrameTracker) LiveFrames() int32 {
    return atomic.LoadInt32(&ft.allocated) - atomic.LoadInt32(&ft.freed)
}

func (ft *FrameTracker) Report() {
    live := ft.LiveFrames()
    if live > 0 {
        log.Warnf("Live frames: %d (allocated: %d, freed: %d)",
            live,
            atomic.LoadInt32(&ft.allocated),
            atomic.LoadInt32(&ft.freed),
        )
    }
}

// Wrap allocations
var frameTracker FrameTracker

func AllocFrameTracked() ffgo.Frame {
    frameTracker.OnFrameAlloc()
    return ffgo.FrameAlloc()
}

func FreeFrameTracked(frame *ffgo.Frame) {
    frameTracker.OnFrameFree()
    ffgo.FrameFree(frame)
}
```

### Technique 2: Goroutine Leak Detection

Use runtime.NumGoroutine to detect goroutine leaks (which often accompany frame leaks):

```go
func monitorGoroutines(ctx context.Context) {
    baseline := runtime.NumGoroutine()
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            current := runtime.NumGoroutine()
            delta := current - baseline

            if delta > 10 {
                log.Warnf("Goroutine growth: %d -> %d (+%d)",
                    baseline, current, delta)

                // Dump goroutine stack for analysis
                buf := make([]byte, 1<<20)
                n := runtime.Stack(buf, true)
                log.Debugf("Goroutine dump:\n%s", string(buf[:n]))

                baseline = current
            }

        case <-ctx.Done():
            return
        }
    }
}
```

### Technique 3: Memory Profiling

Use Go's built-in profiling to identify allocation hotspots:

```go
import _ "net/http/pprof"

func init() {
    // Enable pprof profiling
    go func() {
        log.Println(http.ListenAndServe("localhost:6060", nil))
    }()
}

// In test or main:
func testWithProfiler(t *testing.T) {
    f, _ := os.Create("/tmp/mem.prof")
    defer f.Close()

    runtime.GC()
    pprof.WriteHeapProfile(f)

    // Run test...

    runtime.GC()
    pprof.WriteHeapProfile(f)

    // Analyze with: go tool pprof /tmp/mem.prof
}
```

Then analyze:

```bash
go tool pprof /tmp/mem.prof
(pprof) top
(pprof) list FrameAlloc
(pprof) web  # Generate SVG graph
```

### Technique 4: Valgrind (Linux)

For low-level debugging, use Valgrind to detect FFmpeg-level leaks:

```bash
# Run with Valgrind
valgrind --leak-check=full --show-leak-kinds=all \
    ./streaming-server --config test.yaml

# Valgrind will report:
# - Definitely lost: Leaks in your code
# - Indirectly lost: Leaks in dependencies
# - Possibly lost: Uncertain allocation tracking
```

## Systematic Leak Detection

In production, implement continuous monitoring:

```go
type LeakDetector struct {
    checkInterval  time.Duration
    alertThreshold int32
}

func (ld *LeakDetector) Monitor(ctx context.Context, tracker *FrameTracker) {
    ticker := time.NewTicker(ld.checkInterval)
    defer ticker.Stop()

    baselineGoroutines := runtime.NumGoroutine()
    baselineHeap := getHeapSize()

    for {
        select {
        case <-ticker.C:
            live := tracker.LiveFrames()
            if live > ld.alertThreshold {
                log.Errorf("Possible frame leak: %d frames allocated", live)

                // Trigger dump
                ld.dumpState()
            }

            // Check for goroutine leaks
            currentGoroutines := runtime.NumGoroutine()
            if currentGoroutines > baselineGoroutines*2 {
                log.Warnf("Goroutine growth: %d -> %d",
                    baselineGoroutines, currentGoroutines)
            }

        case <-ctx.Done():
            return
        }
    }
}

func (ld *LeakDetector) dumpState() {
    // Save memory profile
    f, _ := os.Create(fmt.Sprintf("/tmp/heap_%d.prof", time.Now().Unix()))
    defer f.Close()
    pprof.WriteHeapProfile(f)

    // Save goroutine dump
    buf := make([]byte, 1<<20)
    n := runtime.Stack(buf, true)
    log.Infof("Goroutine dump:\n%s", string(buf[:n]))
}
```

## Prevention Best Practices

1. **Always defer frame cleanup** - Make it automatic
2. **Use frame pools** - Reduces alloc/free churn
3. **Add frame accounting** - Know your allocation rate
4. **Monitor continuously** - Alert on anomalies
5. **Test with valgrind** - Catch C-level leaks early
6. **Load test** - Leaks only appear under sustained load

See [memory efficiency](../performance/memory-efficiency.md) for pooling strategies and [frame pipeline](../media/frame-pipeline.md) for correct reference counting.
