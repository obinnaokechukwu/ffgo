---
title: Stream Lifecycle Management
keywords: [stream, lifecycle, state, management, control]
tags: [core-concept, internal, advanced]
related: [../media/decoder-lifecycle.md, ../media/encoder-lifecycle.md, ../patterns/error-handling-strategy.md]
source: [server/streams/stream.go, server/streams/manager.go]
---

# Stream Lifecycle Management

Streams are the fundamental unit of execution. Understanding stream lifecycle is essential for implementing reliable streaming servers.

## Stream States

```
[New] → [Starting] → [Running] → [Stopping] → [Stopped]
         |                            |
         +--------→ [Error] ← -------+
                       ↓
                    [Failed]
```

**New:** Stream created, not yet initialized

**Starting:** Initialization in progress (opening decoder, creating encoders)

**Running:** Normal operation (processing frames)

**Stopping:** Graceful shutdown in progress (draining buffers, closing files)

**Stopped:** Stream successfully completed

**Error:** Error occurred, attempting recovery

**Failed:** Error recovery failed, stream terminated

## Lifecycle Implementation

```go
type Stream struct {
    id       string
    state    StreamState
    mu       sync.RWMutex

    config   StreamConfig
    decoder  *ffgo.Decoder
    encoders map[string]*ffgo.Encoder
    variants map[string]*VariantEncoder

    // Timing
    startTime  time.Time
    stopTime   time.Time

    // Statistics
    statsInput  StreamStats
    statsOutput map[string]StreamStats

    // Control
    ctx       context.Context
    cancel    context.CancelFunc
    errChan   chan error
}

type StreamConfig struct {
    ID          string
    SourceURL   string
    OutputDir   string
    Profile     ABRProfile
    HWDevice    string
}

func NewStream(cfg StreamConfig) *Stream {
    ctx, cancel := context.WithCancel(context.Background())

    return &Stream{
        id:       cfg.ID,
        state:    StateNew,
        config:   cfg,
        variants: make(map[string]*VariantEncoder),
        statsOutput: make(map[string]StreamStats),
        ctx:      ctx,
        cancel:   cancel,
        errChan:  make(chan error, 1),
    }
}
```

## State Transitions

### Transition 1: New → Starting

Initialization begins:

```go
func (s *Stream) Start() error {
    s.mu.Lock()
    if s.state != StateNew {
        s.mu.Unlock()
        return fmt.Errorf("stream not in New state")
    }
    s.state = StateStarting
    s.startTime = time.Now()
    s.mu.Unlock()

    // Run initialization in separate goroutine
    go s.initialize()

    return nil
}

func (s *Stream) initialize() {
    defer func() {
        if r := recover(); r != nil {
            s.transitionToError(fmt.Errorf("panic: %v", r))
        }
    }()

    // 1. Open input decoder
    decoder, err := ffgo.NewDecoder(s.config.SourceURL,
        ffgo.WithHWDevice(s.config.HWDevice),
    )
    if err != nil {
        s.transitionToError(fmt.Errorf("decoder init failed: %w", err))
        return
    }
    s.decoder = decoder

    // 2. Create output encoders
    for _, vp := range s.config.Profile.Variants {
        encoder, err := s.createVariantEncoder(vp)
        if err != nil {
            s.transitionToError(fmt.Errorf("encoder %s init failed: %w", vp.Name, err))
            return
        }
        s.variants[vp.Name] = encoder
    }

    // 3. Transition to Running
    s.transitionToRunning()
}

func (s *Stream) createVariantEncoder(vp VariantProfile) (*VariantEncoder, error) {
    // ... encoder setup
    return encoder, nil
}

func (s *Stream) transitionToRunning() {
    s.mu.Lock()
    defer s.mu.Unlock()

    if s.state != StateStarting {
        return  // Already transitioned or failed
    }

    s.state = StateRunning

    // Start processing
    go s.process()
}
```

### Transition 2: Running → Stopping

Graceful shutdown initiated:

```go
func (s *Stream) Stop() error {
    s.mu.Lock()
    if s.state != StateRunning {
        s.mu.Unlock()
        return fmt.Errorf("stream not in Running state")
    }
    s.state = StateStoppingS
    s.mu.Unlock()

    // Signal processing goroutine to stop
    s.cancel()

    // Wait for processing to complete (with timeout)
    select {
    case err := <-s.errChan:
        return err
    case <-time.After(30 * time.Second):
        return fmt.Errorf("stream shutdown timeout")
    }
}

func (s *Stream) process() {
    defer func() {
        s.transitionToStopped()
    }()

    // Process frames
    for {
        select {
        case <-s.ctx.Done():
            // Stop requested
            s.drain()
            return

        default:
            frame, err := s.decoder.ReadFrame()
            if err != nil {
                if ffgo.IsEOF(err) {
                    s.drain()
                    return
                }
                s.transitionToError(err)
                return
            }

            if err := s.processFrame(frame); err != nil {
                s.transitionToError(err)
                return
            }
        }
    }
}

func (s *Stream) drain() {
    // Flush remaining frames from decoders
    // Close all encoders
    // Wait for output completion

    s.mu.Lock()
    defer s.mu.Unlock()

    if s.decoder != nil {
        s.decoder.Close()
    }
    for _, v := range s.variants {
        v.encoder.Close()
    }
}

func (s *Stream) transitionToStopped() {
    s.mu.Lock()
    defer s.mu.Unlock()

    s.state = StateStopped
    s.stopTime = time.Now()

    // Report completion
    s.errChan <- nil
}
```

### Transition 3: Error Handling

On error, attempt recovery or fail:

```go
func (s *Stream) transitionToError(err error) {
    s.mu.Lock()
    defer s.mu.Unlock()

    log.Errorf("Stream %s error: %v", s.id, err)

    s.state = StateError
    s.lastErr = err

    // Attempt recovery based on error type
    if isRecoverable(err) {
        s.attemptRecovery()
    } else {
        s.transitionToFailed()
    }
}

func (s *Stream) attemptRecovery() {
    // Restart decoder
    // Could reconnect to source, fall back to CPU, etc.

    if err := s.initialize(); err != nil {
        s.transitionToFailed()
    }
}

func (s *Stream) transitionToFailed() {
    s.state = StateFailed
    s.stopTime = time.Now()

    // Cleanup
    if s.decoder != nil {
        s.decoder.Close()
    }

    // Report failure
    s.errChan <- s.lastErr
}
```

## Stream Manager

The manager coordinates multiple streams:

```go
type StreamManager struct {
    streams map[string]*Stream
    mu      sync.RWMutex

    maxConcurrent int
    resourcePool  *ResourcePool
}

func (sm *StreamManager) CreateStream(cfg StreamConfig) (*Stream, error) {
    sm.mu.Lock()
    defer sm.mu.Unlock()

    // Check limits
    if len(sm.streams) >= sm.maxConcurrent {
        return nil, fmt.Errorf("max concurrent streams reached")
    }

    // Check resources
    if !sm.resourcePool.CanAllocate(cfg) {
        return nil, fmt.Errorf("insufficient resources")
    }

    stream := NewStream(cfg)
    sm.streams[cfg.ID] = stream

    if err := stream.Start(); err != nil {
        delete(sm.streams, cfg.ID)
        return nil, err
    }

    return stream, nil
}

func (sm *StreamManager) TerminateStream(id string) error {
    sm.mu.Lock()
    stream, ok := sm.streams[id]
    if !ok {
        sm.mu.Unlock()
        return fmt.Errorf("stream not found")
    }
    sm.mu.Unlock()

    if err := stream.Stop(); err != nil {
        log.Errorf("Stream %s termination error: %v", id, err)
    }

    sm.mu.Lock()
    delete(sm.streams, id)
    sm.resourcePool.Release(stream.config)
    sm.mu.Unlock()

    return nil
}

func (sm *StreamManager) MonitorStreams(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            sm.mu.RLock()
            for id, stream := range sm.streams {
                state := stream.GetState()
                stats := stream.GetStats()

                if state == StateFailed {
                    log.Errorf("Stream %s failed: %v", id, stream.lastErr)
                    // Could auto-restart here
                }

                log.Debugf("Stream %s: state=%s in=%d frames out=%d frames",
                    id, state, stats.FramesIn, stats.FramesOut)
            }
            sm.mu.RUnlock()

        case <-ctx.Done():
            return
        }
    }
}
```

## Statistics Tracking

Track stream health and progress:

```go
type StreamStats struct {
    FramesIn       int64
    FramesOut      int64
    BytesIn        int64
    BytesOut       int64
    DurationProcessed time.Duration
    AverageBitrate    int64
    ErrorCount     int32
    Uptime         time.Duration
}

func (s *Stream) GetStats() StreamStats {
    s.mu.RLock()
    defer s.mu.RUnlock()

    uptime := time.Since(s.startTime)
    if uptime == 0 {
        uptime = time.Since(s.stopTime)
    }

    return StreamStats{
        FramesIn:          s.statsInput.FramesIn,
        FramesOut:         s.statsInput.FramesOut,
        DurationProcessed: s.statsInput.DurationProcessed,
        AverageBitrate:    s.statsInput.BytesOut * 8 / int64(s.statsInput.DurationProcessed.Seconds()),
        Uptime:            uptime,
    }
}
```

## Key Design Principles

1. **Single ownership** - Only one goroutine per stream for simplicity
2. **Graceful degradation** - Continue processing despite errors where possible
3. **Resource tracking** - Know memory/CPU per stream
4. **Clean separation** - Stream logic independent of manager logic
5. **Observable** - Expose state and statistics for monitoring

See [error handling strategy](../patterns/error-handling-strategy.md) for error classification and [decoder lifecycle](../media/decoder-lifecycle.md) for lifecycle of underlying components.
