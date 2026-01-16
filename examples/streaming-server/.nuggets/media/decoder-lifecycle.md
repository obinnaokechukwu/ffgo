---
title: Decoder Lifecycle and State Management
keywords: [decoder, state-machine, lifecycle, streaming, demux]
tags: [core-concept, internal, advanced]
related: [../patterns/ffgo-integration-pattern.md, ./encoder-lifecycle.md, ./frame-pipeline.md]
source: [server/media/decoder.go, server/media/stream_reader.go]
---

# Decoder Lifecycle and State Management

The Decoder is the entry point for all incoming streams in the server. Understanding its lifecycle and state management is critical for implementing reliable ingest handling.

## State Machine

Decoders follow a strict state machine:

```
[Created] -> [Opened] -> [Scanning] -> [Streaming] -> [Draining] -> [Closed]
    |           |           |            |             |             |
    +-----+-----+           |            +------+------+             |
          ^                 |                   |                     |
          |                 +---+---+-----+-----+                     |
          |                     |   |     |                           |
          +---------------------+   |     +---> [Error] --> [Closed]
                                    |
                                    +------> [EOF] --> [Draining] -> [Closed]
```

**Created:** Decoder allocated, no resources acquired

**Opened:** avformat_open_input successful, reading header metadata

**Scanning:** avformat_find_stream_info in progress, analyzing first packets

**Streaming:** Normal data flow, codec contexts open and decoding packets

**Draining:** Source exhausted (EOF), flushing frames from decoders

**Closed:** Resources released, decoder unusable

## Initialization Sequence

Opening a decoder requires several sequential FFmpeg calls:

```go
type Decoder struct {
    state          decoderState  // Current state
    formatCtx      avformat.FormatContext
    videoCodecCtx  avcodec.Context
    audioCodecCtx  avcodec.Context

    videoStreamIdx int
    audioStreamIdx int

    streamInfo map[int]*StreamInfo  // Per-stream metadata

    // Error tracking
    openErr    error
    scanErr    error
}

func (d *Decoder) Open(ctx context.Context) error {
    d.mu.Lock()
    defer d.mu.Unlock()

    // 1. Open input file/URL
    opts := avutil.NewDictionary()
    defer opts.Free()
    opts.Set("rtsp_transport", "tcp")  // For network sources

    if err := avformat.OpenInput(&d.formatCtx, d.url, nil, opts); err != nil {
        d.state = decoderError
        return fmt.Errorf("open_input failed: %w", err)
    }
    d.state = decoderOpened

    // 2. Probe format and find streams
    if err := avformat.FindStreamInfo(d.formatCtx, nil); err != nil {
        d.state = decoderError
        return fmt.Errorf("find_stream_info failed: %w", err)
    }
    d.state = decoderScanning

    // 3. Locate video/audio streams
    d.videoStreamIdx = avformat.FindBestStream(
        d.formatCtx, ffgo.MediaTypeVideo, -1, -1, nil, 0,
    )
    d.audioStreamIdx = avformat.FindBestStream(
        d.formatCtx, ffgo.MediaTypeAudio, -1, -1, nil, 0,
    )

    // 4. Initialize codec contexts
    if d.videoStreamIdx >= 0 {
        if err := d.openVideoDecoder(); err != nil {
            return err
        }
    }
    if d.audioStreamIdx >= 0 {
        if err := d.openAudioDecoder(); err != nil {
            return err
        }
    }

    d.state = decoderStreaming
    return nil
}

func (d *Decoder) openVideoDecoder() error {
    stream := avformat.GetStream(d.formatCtx, d.videoStreamIdx)
    codecPar := avformat.GetStreamCodecPar(stream)

    codec := avcodec.FindDecoder(avcodec.GetCodecID(codecPar))
    if codec == nil {
        return fmt.Errorf("video codec not found")
    }

    d.videoCodecCtx = avcodec.AllocContext(codec)
    if err := avcodec.CopyContext(d.videoCodecCtx, codecPar); err != nil {
        return err
    }

    // Hardware acceleration if requested
    if d.hwDevice != "" {
        hwCtx, err := ffgo.NewHWDevice(d.hwDevice)
        if err != nil {
            return fmt.Errorf("hw device failed: %w", err)
        }
        avcodec.SetHWDeviceContext(d.videoCodecCtx, hwCtx)
    }

    if err := avcodec.OpenContext(d.videoCodecCtx); err != nil {
        return fmt.Errorf("open video codec failed: %w", err)
    }

    // Cache stream info
    d.streamInfo[d.videoStreamIdx] = d.extractStreamInfo(d.videoStreamIdx)
    return nil
}
```

## Packet-to-Frame Pipeline

The core decode loop operates on the state machine:

```go
func (d *Decoder) ReadFrame(ctx context.Context) (ffgo.Frame, error) {
    d.mu.RLock()
    if d.state == decoderClosed {
        d.mu.RUnlock()
        return nil, fmt.Errorf("decoder closed")
    }
    d.mu.RUnlock()

    for {
        // Read packet from input
        pkt := avcodec.AllocPacket()
        defer avcodec.FreePacket(pkt)

        err := avformat.ReadFrame(d.formatCtx, pkt)
        if err != nil {
            if ffgo.IsEOF(err) {
                // Signal EOF, enter draining state
                d.mu.Lock()
                d.state = decoderDraining
                d.mu.Unlock()

                // Send flush packet (NULL)
                return d.drainDecoders(ctx)
            }
            return nil, fmt.Errorf("read_frame failed: %w", err)
        }

        // Route packet to correct decoder
        streamIdx := avcodec.GetPacketStreamIndex(pkt)
        frame := ffgo.FrameAlloc()

        if streamIdx == d.videoStreamIdx {
            if err := avcodec.SendPacket(d.videoCodecCtx, pkt); err != nil {
                return nil, fmt.Errorf("send packet failed: %w", err)
            }
            if err := avcodec.ReceiveFrame(d.videoCodecCtx, frame); err != nil {
                if ffgo.IsAgain(err) {
                    continue  // Need more packets
                }
                return nil, fmt.Errorf("receive frame failed: %w", err)
            }
            return frame, nil
        }
        // Similar for audio...
    }
}

func (d *Decoder) drainDecoders(ctx context.Context) (ffgo.Frame, error) {
    // After EOF, send NULL packets to flush remaining frames
    frame := ffgo.FrameAlloc()

    // Try video decoder first
    if d.videoCodecCtx != nil {
        if err := avcodec.SendPacket(d.videoCodecCtx, nil); err != nil && !ffgo.IsEOF(err) {
            return nil, err
        }
        if err := avcodec.ReceiveFrame(d.videoCodecCtx, frame); err != nil {
            if !ffgo.IsEOF(err) && !ffgo.IsAgain(err) {
                return nil, err
            }
        } else {
            return frame, nil
        }
    }

    // If no frames available, signal completion
    d.mu.Lock()
    d.state = decoderClosed
    d.mu.Unlock()

    return nil, ffgo.ErrEOF
}
```

## Important Considerations

**Thread Safety:** Decoders are NOT thread-safe. Only one goroutine can call Read* methods at a time. Use the mutex if exposing through multiple consumers.

**Memory Management:** Each `ReadFrame` returns a frame with allocated buffers. The caller MUST call `ffgo.FrameUnref()` and then `ffgo.FrameFree(&frame)` when finished to prevent memory leaks.

**Stream Selection:** The Decoder picks the "best" stream of each type. To select specific streams, inspect streamInfo and re-open decoders for specific indices.

**Time Tracking:** Video and audio streams have independent time bases. The pts field in Frame uses the stream's time_base for conversion to seconds.

See [memory efficiency](../performance/memory-efficiency.md) for pooling decoded frames and [frame leaks](../gotchas/frame-leaks.md) for lifetime/cleanup gotchas.
