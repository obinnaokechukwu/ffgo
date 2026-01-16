---
title: Common Pitfalls and Debugging Guide
keywords: [debugging, troubleshooting, pitfalls, gotcha]
tags: [index]
---

# Common Pitfalls and Debugging Guide

This section documents the most common mistakes when building streaming servers with ffgo and how to debug them.

## Major Gotchas

- **Frame Leaks** - Forgetting to free frames, reference counting mismatches
- **Timestamp Rescaling** - Incorrect PTS/DTS in muxed output causing A/V sync issues
- **Thread Safety** - Improper decoder/encoder access from multiple goroutines
- **Keyframe Alignment** - ABR variants not synchronized, causing switching artifacts
- **Encoder Stalls** - Backpressure handling in async encoding
- **Memory Leaks** - Goroutine/decoder/encoder lifecycle issues

## Debugging Tools

**Go Built-in:**
- `runtime/pprof` - Heap, CPU, goroutine profiling
- `runtime.NumGoroutine()` - Track goroutine growth
- `pprof web` - Visualize allocation graphs

**External Tools:**
- Valgrind - FFmpeg-level memory debugging
- Strace - System call tracing
- Perf - CPU profiling

**Logging Levels:**

```go
// Enable FFmpeg debug logging
ffgo.SetLogLevel(ffgo.LogDebug)

// Or use callback for custom handling
ffgo.SetLogCallback(func(level, msg string) {
    if level == "error" || level == "fatal" {
        log.Printf("[FFmpeg %s] %s", level, msg)
    }
})
```

## Common Error Messages

**"failed to send packet"** - Encoder buffer full, need to flush

**"Rescale Timestamp Error"** - Time base mismatch in muxer

**"Codec not found"** - FFmpeg library missing codec support

**"Device not available"** - GPU/hardware acceleration not available

**"EAGAIN (try again)"** - Normal async operation, not an error

## Testing Strategy

1. **Unit tests** - Test decode, encode, scale individually
2. **Integration tests** - Test full pipeline with small files
3. **Load tests** - Run 10+ streams concurrently for hours
4. **Memory tests** - Monitor heap/goroutine growth
5. **Chaos tests** - Inject errors, kill streams, test recovery

## Performance Troubleshooting

**Slow encoding:**
1. Check CPU usage with `top`
2. Check GPU usage with `nvidia-smi`
3. Profile with `pprof`
4. Try faster preset
5. Add GPU acceleration

**Memory growth:**
1. Check frame accounting
2. Look for goroutine leaks
3. Profile heap allocation
4. Check for reference counting errors

**Dropped frames:**
1. Check timestamps (A/V sync)
2. Check for encoder stalls
3. Monitor queue depths
4. Check network bandwidth

See individual gotcha sections for detailed debugging strategies.
