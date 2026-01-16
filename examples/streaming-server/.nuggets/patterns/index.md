---
title: Design Patterns and Common Idioms
keywords: [pattern, design, idiom, architecture]
tags: [index]
---

# Design Patterns and Common Idioms

This section documents design patterns and best practices used throughout the streaming server codebase.

## Architectural Patterns

- **Producer-Consumer** - Decoders produce frames, encoders consume
- **Pipeline** - Frames flow through decode → scale → filter → encode stages
- **Object Pooling** - Reuse frames, buffers, decoders to reduce allocation overhead
- **Reference Counting** - Share frame data across multiple stages
- **Stage Pattern** - Each pipeline stage is an independent goroutine

## Error Handling Patterns

- **Transient vs. Fatal** - Classify errors and retry appropriately
- **Fallback Strategies** - GPU → CPU, high quality → low quality
- **Circuit Breaker** - Stop trying after N failures
- **Error Budgets** - Terminate stream after error quota exceeded

## Concurrency Patterns

- **Channel-based Communication** - Goroutines exchange frames through channels
- **Goroutine Per Stream** - Independent processing for each stream
- **Resource Pooling** - Limit concurrent encoding to avoid overload
- **Graceful Degradation** - Reduce features under resource pressure

## Configuration Patterns

- **Functional Options** - Use functional options for flexible configuration
- **Preset System** - Pre-defined quality/performance profiles
- **Dynamic Reconfiguration** - Change settings without restarting streams
- **Environment Overrides** - Config file + env vars + command-line flags

## Common Implementation Patterns

**State Machine for Decoder:**

```
Created → Opened → Scanning → Streaming → Draining → Closed
```

**Async Encoding Loop:**

```go
for frame := range incomingFrames {
    encoder.SendFrame(frame)
    for {
        pkt, err := encoder.ReceivePacket()
        if err == EAGAIN {
            break
        }
        // Process packet
    }
}
```

**Frame Pooling:**

```go
for frame := range pool.Acquire() {
    // Use frame
    pool.Release(frame)
}
```

## Related Topics

- `ffgo-integration-pattern` - How ffgo integrates with server architecture
- `error-handling-strategy` - Error classification and recovery
