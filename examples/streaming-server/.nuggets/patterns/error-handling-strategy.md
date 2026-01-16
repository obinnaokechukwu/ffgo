---
title: Error Handling Strategy in Media Processing
keywords: [error, handling, recovery, resilience, logging]
tags: [pattern, guide, advanced]
related: [../media/decoder-lifecycle.md, ../media/encoder-lifecycle.md, ../gotchas/index.md]
source: [server/core/stream.go, server/media/processor.go]
---

# Error Handling Strategy in Media Processing

Error handling in streaming servers is nuanced. Some errors are recoverable (EAGAIN), some are fatal (codec not found), and some require fallback strategies.

## FFmpeg Error Categories

ffgo/FFmpeg returns errors in these categories:

### Category 1: Transient Errors (Retry)

These errors indicate temporary conditions:

```go
// EAGAIN - Encoder buffer full, decoder needs more packets
if ffgo.IsAgain(err) {
    // Try again soon
    return retry(operation)
}

// Example: Encoding produces buffered packets
for {
    pkt, err := encoder.GetPacket()
    if ffgo.IsAgain(err) {
        // Send more frames first
        break
    }
    if err != nil {
        return err
    }
    // Process packet
}
```

### Category 2: EOF Errors (Graceful Termination)

End-of-file indicates input exhaustion:

```go
if ffgo.IsEOF(err) {
    // Normal stream end, drain remaining frames
    for {
        frame, err := decoder.ReadFrame()
        if ffgo.IsEOF(err) {
            break  // All frames drained
        }
        // Process frame
    }
}
```

### Category 3: Fatal Errors (Abort Stream)

These errors indicate unrecoverable conditions:

```go
// Codec not found
if strings.Contains(err.Error(), "codec not found") {
    log.Errorf("Stream %s: unsupported codec", streamID)
    return handleStreamError(streamID, err)
}

// Format error
if strings.Contains(err.Error(), "Invalid data") {
    log.Errorf("Stream %s: corrupted data", streamID)
    return handleStreamError(streamID, err)
}

// Device not available
if strings.Contains(err.Error(), "device not available") {
    log.Errorf("Hardware device unavailable")
    // Fall back to software decode
}
```

## Wrapped Error Types

The streaming server defines error types for clarity:

```go
type StreamError struct {
    StreamID  string
    Operation string  // "decode", "encode", "segment"
    Err       error
    Recovered bool
}

func (se *StreamError) Error() string {
    status := "fatal"
    if se.Recovered {
        status = "recovered"
    }
    return fmt.Sprintf("[%s] %s %s: %v", se.StreamID, se.Operation, status, se.Err)
}

// Wrapper functions
func decodeError(streamID string, err error) error {
    return &StreamError{
        StreamID:  streamID,
        Operation: "decode",
        Err:       err,
    }
}
```

## Recovery Strategies

### Strategy 1: Retry with Backoff

For transient errors, retry with exponential backoff:

```go
func retryWithBackoff(operation func() error, maxRetries int) error {
    var lastErr error
    backoff := 10 * time.Millisecond

    for attempt := 0; attempt < maxRetries; attempt++ {
        if err := operation(); err != nil {
            if !isTransient(err) {
                return err  // Fatal, don't retry
            }
            lastErr = err
            time.Sleep(backoff)
            backoff *= 2
            continue
        }
        return nil  // Success
    }

    return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// Usage
err := retryWithBackoff(func() error {
    frame, err := decoder.ReadFrame()
    if err != nil {
        return err
    }
    defer ffgo.FrameFree(&frame)
    ffgo.FrameUnref(frame)
    return nil
}, 3)
```

### Strategy 2: Fallback Codec

If GPU decode fails, fall back to CPU:

```go
func openDecoderWithFallback(url string) (*ffgo.Decoder, error) {
    // Try GPU first
    decoder, err := ffgo.NewDecoder(url,
        ffgo.WithHWDevice("cuda"),
    )
    if err == nil {
        log.Printf("Using NVIDIA GPU decoding")
        return decoder, nil
    }

    log.Warnf("GPU decoding failed: %v, falling back to CPU", err)

    // Fall back to CPU
    return ffgo.NewDecoder(url)
}
```

### Strategy 3: Quality Degradation

On encoding overload, reduce quality:

```go
type AdaptiveEncoder struct {
    baseConfig ffgo.VideoEncoderConfig
    currentCRF int
    failCount  int
}

func (ae *AdaptiveEncoder) Encode(frame ffgo.Frame) error {
    err := ae.encoder.EncodeFrame(frame)

    if err != nil {
        ae.failCount++

        // If consistently failing, reduce quality
        if ae.failCount > 5 {
            ae.currentCRF = min(51, ae.currentCRF+2)
            log.Warnf("Encoding errors, reducing quality to CRF=%d", ae.currentCRF)

            // Re-create encoder with new quality
            ae.encoder.Close()
            return ae.recreateEncoder()
        }

        return err
    }

    ae.failCount = 0  // Reset on success
    return nil
}
```

## Error Logging and Monitoring

Structured logging helps troubleshooting:

```go
type ErrorLogger struct {
    // Count errors by type
    counts map[string]int32
    mu     sync.RWMutex
}

func (el *ErrorLogger) LogError(err error, metadata map[string]interface{}) {
    // Classify error
    errType := classifyError(err)

    el.mu.Lock()
    el.counts[errType]++
    el.mu.Unlock()

    // Log with context
    log.WithFields(map[string]interface{}{
        "error_type": errType,
        "error":      err.Error(),
    }).
    WithFields(metadata).
    Error("Media processing error")

    // Alert if error rate spikes
    if el.counts[errType] > 10 {
        alert(fmt.Sprintf("High error rate: %s", errType))
    }
}

func classifyError(err error) string {
    errStr := err.Error()

    if ffgo.IsEOF(err) {
        return "eof"
    }
    if ffgo.IsAgain(err) {
        return "eagain"
    }
    if strings.Contains(errStr, "codec") {
        return "codec_error"
    }
    if strings.Contains(errStr, "memory") {
        return "memory_error"
    }
    return "unknown"
}
```

## Per-Stream Error Isolation

Errors in one stream should not affect others:

```go
type StreamProcessor struct {
    streams map[string]*Stream
    mu      sync.RWMutex
}

func (sp *StreamProcessor) ProcessStream(streamID string) {
    stream := sp.streams[streamID]

    // Run with panic recovery
    defer func() {
        if r := recover(); r != nil {
            log.Errorf("Stream %s panicked: %v", streamID, r)
            sp.terminateStream(streamID)
        }
    }()

    // Run stream
    if err := stream.Process(); err != nil {
        log.Errorf("Stream %s error: %v", streamID, err)
        sp.terminateStream(streamID)
    }
}

func (sp *StreamProcessor) terminateStream(streamID string) {
    sp.mu.Lock()
    defer sp.mu.Unlock()

    if stream, ok := sp.streams[streamID]; ok {
        stream.Close()
        delete(sp.streams, streamID)
    }
}
```

## Error Budgets

For reliability, track error budgets:

```go
type ErrorBudget struct {
    maxErrors     int
    windowSize    time.Duration
    recentErrors  []time.Time
    mu             sync.RWMutex
}

func (eb *ErrorBudget) IsExhausted() bool {
    eb.mu.RLock()
    defer eb.mu.RUnlock()

    now := time.Now()
    cutoff := now.Add(-eb.windowSize)

    // Remove old errors outside window
    for len(eb.recentErrors) > 0 && eb.recentErrors[0].Before(cutoff) {
        eb.recentErrors = eb.recentErrors[1:]
    }

    return len(eb.recentErrors) >= eb.maxErrors
}

func (eb *ErrorBudget) RecordError() {
    eb.mu.Lock()
    defer eb.mu.Unlock()

    eb.recentErrors = append(eb.recentErrors, time.Now())
}

// Usage
stream := &Stream{
    errorBudget: &ErrorBudget{
        maxErrors:  5,
        windowSize: 1 * time.Minute,
    },
}

if err := stream.Process(); err != nil {
    stream.errorBudget.RecordError()

    if stream.errorBudget.IsExhausted() {
        log.Errorf("Stream %s error budget exhausted, terminating", stream.ID)
        return
    }
}
```

## Testing Error Scenarios

Test error handling with fault injection:

```go
type FaultInjector struct {
    failureRate float64  // 0.0-1.0
    failureType string   // "codec", "memory", "io"
}

func (fi *FaultInjector) Decode() error {
    if rand.Float64() < fi.failureRate {
        switch fi.failureType {
        case "codec":
            return fmt.Errorf("ffmpeg: codec not found")
        case "memory":
            return fmt.Errorf("ffmpeg: memory allocation failed")
        case "io":
            return fmt.Errorf("ffmpeg: I/O error")
        }
    }
    return nil
}

// Test with 5% error rate
func TestErrorRecovery(t *testing.T) {
    injector := &FaultInjector{
        failureRate: 0.05,
        failureType: "codec",
    }

    // Run stream with injected errors
    for i := 0; i < 1000; i++ {
        if err := injector.Decode(); err != nil {
            // Verify recovery
            require.NoError(t, handleError(err))
        }
    }
}
```

## Key Principles

1. **Classify errors immediately** - Transient vs. fatal
2. **Isolate errors per stream** - Don't cascade
3. **Log with context** - Include stream ID, operation, metadata
4. **Set error budgets** - Terminate after N errors in window
5. **Test error paths** - Use fault injection
6. **Implement fallbacks** - GPU → CPU, hardware → software
7. **Monitor error rates** - Alert on spikes

See [gotchas](../gotchas/index.md) for common error scenarios and [stream lifecycle](../streams/stream-lifecycle.md) for cleanup and shutdown on errors.
