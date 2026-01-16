---
title: Encoder Lifecycle and Streaming Output
keywords: [encoder, output, muxing, packets, transcoding]
tags: [core-concept, internal, advanced]
related: [../patterns/ffgo-integration-pattern.md, ./decoder-lifecycle.md, ./stream-copy-optimization.md]
source: [server/media/encoder.go, server/media/output_writer.go]
---

# Encoder Lifecycle and Streaming Output

Encoders are the output stage of transcoding pipelines. Unlike decoders which read source data, encoders produce encoded media for streaming delivery or recording.

## State Machine

Encoders follow an asymmetric state machine compared to decoders:

```
[Created] -> [Initialized] -> [HeaderWritten] -> [Encoding] -> [Draining] -> [Closed]
                  |                              |              |
                  +-----+                        |              |
                        |                        |              |
                        +----> [Error] ----------+-> [Closed]
```

**Created:** Encoder allocated, no streams added

**Initialized:** FormatContext created, output format and codecs specified, but header not written

**HeaderWritten:** Container header written, streams configured, ready to accept frames

**Encoding:** Receiving frames and outputting packets

**Draining:** No more frames coming, flushing codec and muxer

**Closed:** Resources released

## Initialization Sequence

Encoder setup is more complex than decoder due to output format requirements:

```go
type Encoder struct {
    state         encoderState
    formatCtx     avformat.FormatContext
    ioCtx         avformat.IOContext

    videoCodecCtx avcodec.Context
    audioCodecCtx avcodec.Context
    videoStream   avformat.Stream
    audioStream   avformat.Stream

    outputFormat  avformat.OutputFormat
    outputPath    string

    // Codec options passed from config
    codecOptions  map[string]interface{}

    // Muxing state
    headerWritten bool
    frameCount    int64
    lastPTS       map[int]int64  // Per-stream PTS tracking
}

// EncoderConfig specifies video and audio encoding parameters
type EncoderConfig struct {
    Video VideoEncoderConfig
    Audio AudioEncoderConfig
    Format string  // "mp4", "hls", "rtmp", etc.
}

type VideoEncoderConfig struct {
    Codec        ffgo.CodecID
    Width        int
    Height       int
    PixelFormat  ffgo.PixelFormat
    FrameRate    ffgo.Rational
    BitRate      int64
    CRF          int     // Quality (H.264/H.265)
    Preset       string  // "fast", "medium", "slow"
    Profile      string  // "baseline", "main", "high"
    HWDevice     string  // "cuda", "vaapi", etc.
}

type AudioEncoderConfig struct {
    Codec       ffgo.CodecID
    SampleRate  int
    Channels    int
    SampleFmt   ffgo.SampleFormat
    BitRate     int64
}

func (e *Encoder) Initialize(cfg EncoderConfig) error {
    e.mu.Lock()
    defer e.mu.Unlock()

    // 1. Determine output format from filename/config
    e.outputFormat = avformat.GuessFormat("", e.outputPath, "")
    if e.outputFormat == nil {
        return fmt.Errorf("unknown output format")
    }

    // 2. Create format context
    e.formatCtx, err = avformat.AllocFormatContext(e.outputFormat)
    if err != nil {
        return err
    }

    e.state = encoderInitialized
    return nil
}

func (e *Encoder) AddVideoStream(cfg VideoEncoderConfig) error {
    e.mu.Lock()
    defer e.mu.Unlock()

    if e.state != encoderInitialized {
        return fmt.Errorf("encoder not in initialized state")
    }

    // 1. Find codec
    codec := avcodec.FindEncoder(cfg.Codec)
    if codec == nil {
        return fmt.Errorf("video codec %d not found", cfg.Codec)
    }

    // 2. Allocate codec context
    e.videoCodecCtx = avcodec.AllocContext(codec)
    if e.videoCodecCtx == nil {
        return fmt.Errorf("failed to allocate video codec context")
    }

    // 3. Configure codec parameters
    avcodec.SetContextWidth(e.videoCodecCtx, cfg.Width)
    avcodec.SetContextHeight(e.videoCodecCtx, cfg.Height)
    avcodec.SetContextPixFmt(e.videoCodecCtx, cfg.PixelFormat)
    avcodec.SetContextTimeBase(e.videoCodecCtx, cfg.FrameRate.Invert())
    avcodec.SetContextBitRate(e.videoCodecCtx, cfg.BitRate)

    // 4. Apply codec-specific options
    opts := avutil.NewDictionary()
    defer opts.Free()

    opts.Set("preset", cfg.Preset)      // ffmpeg -preset
    opts.Set("profile", cfg.Profile)     // ffmpeg -profile:v
    if cfg.CRF > 0 {
        opts.Set("crf", fmt.Sprint(cfg.CRF))
    }

    // Hardware acceleration
    if cfg.HWDevice != "" {
        hwCtx, err := ffgo.NewHWDevice(cfg.HWDevice)
        if err == nil {
            avcodec.SetHWDeviceContext(e.videoCodecCtx, hwCtx)
        }
    }

    // 5. Open codec
    if err := avcodec.OpenContext(e.videoCodecCtx, opts); err != nil {
        return fmt.Errorf("failed to open video codec: %w", err)
    }

    // 6. Create stream in muxer
    e.videoStream, err = avformat.NewStream(e.formatCtx)
    if err != nil {
        return err
    }

    // 7. Copy codec parameters from context to stream
    if err := avcodec.CopyContextToParameters(e.videoCodecCtx, e.videoStream); err != nil {
        return err
    }

    return nil
}

func (e *Encoder) AddAudioStream(cfg AudioEncoderConfig) error {
    // Similar to AddVideoStream but for audio codec
    // ...
}
```

## Header Writing and Muxer Initialization

The header must be written before encoding frames:

```go
func (e *Encoder) WriteHeader() error {
    e.mu.Lock()
    defer e.mu.Unlock()

    if e.state != encoderInitialized {
        return fmt.Errorf("encoder not ready for header")
    }

    // 1. Set output file/callback
    if e.isNetworkOutput() {
        // Use custom output callbacks for streaming
        e.ioCtx, err = avformat.AllocIOContext(
            e.customWriteFunc,
            nil,  // read_packet
            nil,  // seek
        )
        if err != nil {
            return err
        }
        avformat.SetIOContext(e.formatCtx, e.ioCtx)
    } else {
        // File output
        if err := avformat.OpenFileOutput(e.formatCtx, e.outputPath); err != nil {
            return err
        }
    }

    // 2. Write format header
    if err := avformat.WriteHeader(e.formatCtx, nil); err != nil {
        return fmt.Errorf("write header failed: %w", err)
    }

    e.headerWritten = true
    e.state = encoderHeaderWritten
    return nil
}
```

## Frame-to-Packet Encoding

The core encoding loop is asynchronous:

```go
func (e *Encoder) EncodeVideoFrame(frame ffgo.Frame) error {
    e.mu.RLock()
    if !e.headerWritten {
        e.mu.RUnlock()
        return fmt.Errorf("header not written")
    }
    e.mu.RUnlock()

    // Encoding is asynchronous - send frame, receive packets
    if err := avcodec.SendFrame(e.videoCodecCtx, frame); err != nil {
        if ffgo.IsAgain(err) {
            // Encoder buffer full, must flush before sending more
            return e.flushVideoPackets()
        }
        return fmt.Errorf("send frame failed: %w", err)
    }

    return e.flushVideoPackets()
}

func (e *Encoder) flushVideoPackets() error {
    pkt := avcodec.AllocPacket()
    defer avcodec.FreePacket(pkt)

    for {
        err := avcodec.ReceivePacket(e.videoCodecCtx, pkt)
        if err != nil {
            if ffgo.IsAgain(err) {
                return nil  // Normal - no more packets available
            }
            if ffgo.IsEOF(err) {
                return nil  // Flushing complete
            }
            return fmt.Errorf("receive packet failed: %w", err)
        }

        // Write packet to output
        if err := e.writePacket(pkt, e.videoStream); err != nil {
            return err
        }
    }
}

func (e *Encoder) writePacket(pkt ffgo.Packet, stream avformat.Stream) error {
    // Rescale packet timing to output stream's time base
    avcodec.RescalePacket(
        pkt,
        avcodec.GetStreamTimeBase(e.videoStream),
    )

    pkt.StreamIndex = avformat.GetStreamIndex(stream)

    // Write to muxer
    if err := avformat.WriteFrame(e.formatCtx, pkt); err != nil {
        return fmt.Errorf("write frame failed: %w", err)
    }

    e.frameCount++
    return nil
}
```

## Graceful Shutdown and Draining

When encoding is complete, frames must be flushed and trailer written:

```go
func (e *Encoder) Close() error {
    e.mu.Lock()
    defer e.mu.Unlock()

    if e.state == encoderClosed {
        return nil  // Idempotent
    }

    if e.state == encoderEncoding {
        // Enter draining state
        e.state = encoderDraining

        // 1. Flush video codec
        if e.videoCodecCtx != nil {
            if err := avcodec.SendFrame(e.videoCodecCtx, nil); err != nil {
                // Not a fatal error
            }
            e.flushVideoPackets()  // Best effort
        }

        // 2. Flush audio codec
        if e.audioCodecCtx != nil {
            if err := avcodec.SendFrame(e.audioCodecCtx, nil); err != nil {
                // Not a fatal error
            }
            e.flushAudioPackets()  // Best effort
        }
    }

    // 3. Write trailer
    if e.headerWritten {
        avformat.WriteTrailer(e.formatCtx)
    }

    // 4. Close codec contexts
    if e.videoCodecCtx != nil {
        avcodec.FreeContext(e.videoCodecCtx)
    }
    if e.audioCodecCtx != nil {
        avcodec.FreeContext(e.audioCodecCtx)
    }

    // 5. Close output
    if e.ioCtx != nil {
        avformat.FreeIOContext(e.ioCtx)
    } else {
        avformat.CloseFileOutput(e.formatCtx)
    }

    avformat.FreeContext(e.formatCtx)

    e.state = encoderClosed
    return nil
}
```

## Critical Design Patterns

**Asynchronous Encoding:** ffmpeg uses send/receive semantics. Frames sent to the encoder don't immediately produce packets. You must call ReceivePacket repeatedly until EAGAIN.

**Time Base Rescaling:** Output packets must use the output stream's time base, not the decoder's. Always rescale PTS/DTS before writing.

**One Encoder Per Output:** You cannot encode to multiple outputs with a single Encoder instance. Create separate Encoder instances for HLS, DASH, and fallback outputs.

See [encoding performance](../performance/encoding-performance.md) for buffer management and [gotchas](../gotchas/index.md) for common output issues.
