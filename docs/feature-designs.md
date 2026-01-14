# ffgo Feature Designs: Complete FFmpeg Coverage

> **Purpose**: Specification-grade designs for all missing features identified in gap-analysis.md.
> These designs account for purego constraints while providing clean user-facing APIs.

---

## Design Philosophy

1. **Hide purego limitations**: Users should never encounter purego-specific errors
2. **100% FFmpeg compatibility**: Every FFmpeg feature should be accessible
3. **Idiomatic Go**: APIs feel native, not like C bindings
4. **Safe by default**: Memory safety, proper cleanup, clear ownership

---

## Table of Contents

1. [Audio Resampling (swresample)](#1-audio-resampling-swresample)
2. [Filter Graphs (avfilter)](#2-filter-graphs-avfilter)
3. [Audio Encoding](#3-audio-encoding)
4. [Advanced Codec Options](#4-advanced-codec-options)
5. [Subtitle Support](#5-subtitle-support)
6. [Bitstream Filters](#6-bitstream-filters)
7. [Stream Copy Mode](#7-stream-copy-mode)
8. [Hardware Acceleration (Complete)](#8-hardware-acceleration-complete)
9. [Multi-Stream Muxing](#9-multi-stream-muxing)
10. [Advanced Seeking](#10-advanced-seeking)
11. [Metadata Handling](#11-metadata-handling)
12. [Device I/O](#12-device-io)
13. [Network Protocols](#13-network-protocols)
14. [Image Sequences](#14-image-sequences)
15. [Color Space Handling](#15-color-space-handling)

---

## 1. Audio Resampling (swresample)

### 1.1 FFmpeg API Analysis

**Core Functions** (all purego-compatible):
```c
SwrContext *swr_alloc(void);                                    // 0 args ✓
SwrContext *swr_alloc_set_opts2(SwrContext **ps,                // 9 args ✓
    const AVChannelLayout *out_ch_layout, enum AVSampleFormat out_sample_fmt, int out_sample_rate,
    const AVChannelLayout *in_ch_layout, enum AVSampleFormat in_sample_fmt, int in_sample_rate,
    int log_offset, void *log_ctx);
int swr_init(SwrContext *s);                                     // 1 arg ✓
void swr_free(SwrContext **s);                                   // 1 arg ✓
int swr_convert(SwrContext *s, uint8_t **out, int out_count,     // 5 args ✓
                const uint8_t **in, int in_count);
int swr_convert_frame(SwrContext *swr, AVFrame *output, const AVFrame *input); // 3 args ✓
int64_t swr_get_delay(SwrContext *s, int64_t base);              // 2 args ✓
int swr_get_out_samples(SwrContext *s, int in_samples);          // 2 args ✓
```

**purego Constraints**:
- All functions have ≤15 arguments ✓
- No variadic functions ✓
- No struct-by-value returns ✓
- `AVChannelLayout` passed by pointer ✓

### 1.2 Package Structure

```
swresample/
├── swresample.go      # Main API
├── context.go         # SwrContext wrapper
├── options.go         # Configuration options
└── swresample_test.go # Tests
```

### 1.3 Low-Level API

```go
// swresample/swresample.go
package swresample

import "unsafe"

// SwrContext is an opaque audio resampling context
type SwrContext = unsafe.Pointer

// Function bindings
var (
    swr_alloc              func() SwrContext
    swr_alloc_set_opts2    func(ps *SwrContext, outChLayout unsafe.Pointer, outFmt int32, outRate int32,
                                inChLayout unsafe.Pointer, inFmt int32, inRate int32,
                                logOffset int32, logCtx unsafe.Pointer) int32
    swr_init               func(s SwrContext) int32
    swr_free               func(s *SwrContext)
    swr_convert            func(s SwrContext, out, in unsafe.Pointer, outCount, inCount int32) int32
    swr_convert_frame      func(s SwrContext, output, input unsafe.Pointer) int32
    swr_get_delay          func(s SwrContext, base int64) int64
    swr_get_out_samples    func(s SwrContext, inSamples int32) int32
)

// Alloc allocates a new SwrContext
func Alloc() SwrContext

// Init initializes the context after setting options
func Init(s SwrContext) error

// Free releases a SwrContext
func Free(s *SwrContext)

// Convert resamples audio data
// Returns number of samples output per channel, or negative on error
func Convert(s SwrContext, out [][]byte, outCount int, in [][]byte, inCount int) (int, error)

// ConvertFrame resamples an entire frame
func ConvertFrame(s SwrContext, output, input *avutil.Frame) error

// GetDelay returns the delay (in samples at the given sample rate)
func GetDelay(s SwrContext, base int64) int64

// GetOutSamples estimates output samples for given input
func GetOutSamples(s SwrContext, inSamples int) int
```

### 1.4 High-Level API

```go
// ffgo/resampler.go
package ffgo

// Resampler converts audio between formats
type Resampler struct {
    ctx       swresample.SwrContext
    srcFormat AudioFormat
    dstFormat AudioFormat
    closed    bool
}

// AudioFormat describes audio characteristics
type AudioFormat struct {
    SampleRate    int              // e.g., 44100, 48000
    Channels      int              // e.g., 1, 2, 6
    ChannelLayout ChannelLayout    // e.g., ChannelLayoutStereo
    SampleFormat  SampleFormat     // e.g., SampleFormatS16, SampleFormatFLTP
}

// ChannelLayout represents audio channel configuration
type ChannelLayout int64

const (
    ChannelLayoutMono        ChannelLayout = 0x4        // AV_CH_LAYOUT_MONO
    ChannelLayoutStereo      ChannelLayout = 0x3        // AV_CH_LAYOUT_STEREO
    ChannelLayout5Point1     ChannelLayout = 0x3F       // AV_CH_LAYOUT_5POINT1
    ChannelLayout7Point1     ChannelLayout = 0x63F      // AV_CH_LAYOUT_7POINT1
)

// SampleFormat represents audio sample formats
type SampleFormat int32

const (
    SampleFormatNone SampleFormat = -1
    SampleFormatU8   SampleFormat = 0   // Unsigned 8-bit
    SampleFormatS16  SampleFormat = 1   // Signed 16-bit
    SampleFormatS32  SampleFormat = 2   // Signed 32-bit
    SampleFormatFLT  SampleFormat = 3   // Float 32-bit
    SampleFormatDBL  SampleFormat = 4   // Float 64-bit
    SampleFormatU8P  SampleFormat = 5   // Unsigned 8-bit planar
    SampleFormatS16P SampleFormat = 6   // Signed 16-bit planar
    SampleFormatS32P SampleFormat = 7   // Signed 32-bit planar
    SampleFormatFLTP SampleFormat = 8   // Float 32-bit planar
    SampleFormatDBLP SampleFormat = 9   // Float 64-bit planar
    SampleFormatS64  SampleFormat = 10  // Signed 64-bit
    SampleFormatS64P SampleFormat = 11  // Signed 64-bit planar
)

// NewResampler creates an audio resampler
//
// Example:
//   resampler, err := ffgo.NewResampler(
//       ffgo.AudioFormat{SampleRate: 44100, Channels: 2, SampleFormat: ffgo.SampleFormatS16},
//       ffgo.AudioFormat{SampleRate: 48000, Channels: 2, SampleFormat: ffgo.SampleFormatFLTP},
//   )
func NewResampler(src, dst AudioFormat) (*Resampler, error)

// Resample converts an audio frame
// Returns a new frame with resampled audio, or nil if more input needed
func (r *Resampler) Resample(frame *Frame) (*Frame, error)

// Flush drains any remaining samples from the resampler
func (r *Resampler) Flush() (*Frame, error)

// Close releases resources
func (r *Resampler) Close() error
```

### 1.5 Usage Examples

```go
// Example 1: Simple sample rate conversion
resampler, err := ffgo.NewResampler(
    ffgo.AudioFormat{SampleRate: 44100, Channels: 2, SampleFormat: ffgo.SampleFormatS16},
    ffgo.AudioFormat{SampleRate: 48000, Channels: 2, SampleFormat: ffgo.SampleFormatS16},
)
if err != nil {
    return err
}
defer resampler.Close()

for {
    frame, err := decoder.ReadFrame()
    if frame == nil {
        break
    }
    if frame.MediaType() == ffgo.MediaTypeAudio {
        resampled, err := resampler.Resample(frame)
        if err != nil {
            return err
        }
        if resampled != nil {
            encoder.WriteAudioFrame(resampled)
        }
    }
}

// Flush remaining samples
for {
    flushed, err := resampler.Flush()
    if err != nil || flushed == nil {
        break
    }
    encoder.WriteAudioFrame(flushed)
}

// Example 2: Stereo to 5.1 upmix
resampler, _ := ffgo.NewResampler(
    ffgo.AudioFormat{SampleRate: 48000, Channels: 2, ChannelLayout: ffgo.ChannelLayoutStereo, SampleFormat: ffgo.SampleFormatFLTP},
    ffgo.AudioFormat{SampleRate: 48000, Channels: 6, ChannelLayout: ffgo.ChannelLayout5Point1, SampleFormat: ffgo.SampleFormatFLTP},
)

// Example 3: Format conversion only (no rate change)
resampler, _ := ffgo.NewResampler(
    ffgo.AudioFormat{SampleRate: 48000, Channels: 2, SampleFormat: ffgo.SampleFormatS16},
    ffgo.AudioFormat{SampleRate: 48000, Channels: 2, SampleFormat: ffgo.SampleFormatFLTP},
)
```

### 1.6 Error Handling

| Condition | Behavior |
|-----------|----------|
| Invalid sample rate (0 or negative) | Return `ErrInvalidSampleRate` |
| Invalid channel count | Return `ErrInvalidChannels` |
| Incompatible format combination | Return `ErrIncompatibleFormats` |
| Resampler not initialized | Return `ErrNotInitialized` |
| FFmpeg internal error | Return `*FFmpegError` with code and message |
| Nil frame passed | Return `nil, nil` (no output yet) |
| Resampler already closed | Return `ErrClosed` |

### 1.7 Memory Management

- `SwrContext` allocated by FFmpeg, freed by `Close()`
- Output frames allocated by `Resample()`, caller owns them
- Input frames not modified, caller retains ownership
- Use finalizer as safety net for unclosed resamplers

### 1.8 Shim Requirements

**None** - all swresample functions are non-variadic and compatible with purego.

---

## 2. Filter Graphs (avfilter)

### 2.1 FFmpeg API Analysis

**Core Functions** (purego compatibility):
```c
// Graph management
AVFilterGraph *avfilter_graph_alloc(void);                       // 0 args ✓
void avfilter_graph_free(AVFilterGraph **graph);                 // 1 arg ✓
int avfilter_graph_config(AVFilterGraph *graphctx, void *log_ctx); // 2 args ✓
int avfilter_graph_parse2(AVFilterGraph *graph, const char *filters, // 3 args ✓
                          AVFilterInOut **inputs, AVFilterInOut **outputs);

// Filter operations
const AVFilter *avfilter_get_by_name(const char *name);          // 1 arg ✓
int avfilter_graph_create_filter(AVFilterContext **filt_ctx,     // 6 args ✓
    const AVFilter *filt, const char *name, const char *args,
    void *opaque, AVFilterGraph *graph_ctx);

// Buffer source/sink
int av_buffersrc_add_frame(AVFilterContext *ctx, AVFrame *frame); // 2 args ✓
int av_buffersrc_add_frame_flags(AVFilterContext *ctx, AVFrame *frame, int flags); // 3 args ✓
int av_buffersink_get_frame(AVFilterContext *ctx, AVFrame *frame); // 2 args ✓
int av_buffersink_get_frame_flags(AVFilterContext *ctx, AVFrame *frame, int flags); // 3 args ✓

// Filter linking
int avfilter_link(AVFilterContext *src, unsigned srcpad,         // 4 args ✓
                  AVFilterContext *dst, unsigned dstpad);
```

**purego Constraints**:
- All functions ≤15 args ✓
- No variadic functions in core API ✓
- Some internal functions use variadics (use `avfilter_graph_parse2` instead)

### 2.2 Package Structure

```
avfilter/
├── avfilter.go       # Core bindings
├── graph.go          # FilterGraph wrapper
├── context.go        # FilterContext wrapper
├── buffersrc.go      # Buffer source functions
├── buffersink.go     # Buffer sink functions
└── avfilter_test.go  # Tests
```

### 2.3 High-Level API

```go
// ffgo/filter.go
package ffgo

// FilterGraph represents a filter processing pipeline
type FilterGraph struct {
    graph      *avfilter.Graph
    bufferSrc  *avfilter.Context  // Input filter
    bufferSink *avfilter.Context  // Output filter
    srcFormat  FilterFormat
    closed     bool
}

// FilterFormat describes the format for filter input/output
type FilterFormat struct {
    // For video
    Width       int
    Height      int
    PixelFormat PixelFormat
    TimeBase    Rational
    FrameRate   Rational

    // For audio
    SampleRate    int
    Channels      int
    ChannelLayout ChannelLayout
    SampleFormat  SampleFormat
}

// FilterGraphConfig configures a filter graph
type FilterGraphConfig struct {
    // Filter description string (ffmpeg filter syntax)
    // Examples:
    //   "scale=1280:720"
    //   "overlay=10:10"
    //   "hflip,vflip"
    //   "volume=0.5"
    //   "[0:v][1:v]overlay=10:10[out]"
    Filters string

    // Input format (auto-detected from source if not set)
    InputFormat *FilterFormat

    // Output format (uses filter output if not set)
    OutputFormat *FilterFormat
}

// NewFilterGraph creates a filter graph from a filter string
//
// For video:
//   graph, err := ffgo.NewFilterGraph(ffgo.FilterGraphConfig{
//       Filters: "scale=1280:720,hflip",
//       InputFormat: &ffgo.FilterFormat{
//           Width: 1920, Height: 1080,
//           PixelFormat: ffgo.PixelFormatYUV420P,
//       },
//   })
//
// For audio:
//   graph, err := ffgo.NewFilterGraph(ffgo.FilterGraphConfig{
//       Filters: "volume=0.5,aresample=48000",
//       InputFormat: &ffgo.FilterFormat{
//           SampleRate: 44100, Channels: 2,
//           SampleFormat: ffgo.SampleFormatFLTP,
//       },
//   })
func NewFilterGraph(config FilterGraphConfig) (*FilterGraph, error)

// NewVideoFilterGraph creates a video filter graph (convenience)
func NewVideoFilterGraph(filters string, width, height int, pixFmt PixelFormat) (*FilterGraph, error)

// NewAudioFilterGraph creates an audio filter graph (convenience)
func NewAudioFilterGraph(filters string, sampleRate, channels int, sampleFmt SampleFormat) (*FilterGraph, error)

// Filter processes a frame through the filter graph
// Returns filtered frames (may be 0, 1, or many depending on filter)
func (g *FilterGraph) Filter(frame *Frame) ([]*Frame, error)

// Flush drains remaining frames from the filter graph
func (g *FilterGraph) Flush() ([]*Frame, error)

// Close releases resources
func (g *FilterGraph) Close() error
```

### 2.4 Common Filter Presets

```go
// ffgo/filters.go
package ffgo

// Video Filters - Preset constructors for common operations

// Scale creates a scaling filter
func Scale(width, height int) string {
    return fmt.Sprintf("scale=%d:%d", width, height)
}

// ScaleKeepAspect scales maintaining aspect ratio
// Use -1 for auto-calculate dimension
func ScaleKeepAspect(width, height int) string {
    return fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", width, height)
}

// Crop creates a crop filter
func Crop(width, height, x, y int) string {
    return fmt.Sprintf("crop=%d:%d:%d:%d", width, height, x, y)
}

// Overlay creates an overlay filter
func Overlay(x, y int) string {
    return fmt.Sprintf("overlay=%d:%d", x, y)
}

// Rotate creates a rotation filter
// Angle in radians, or use constants: "PI/2", "PI", "-PI/2"
func Rotate(angle string) string {
    return fmt.Sprintf("rotate=%s", angle)
}

// HFlip creates a horizontal flip filter
func HFlip() string { return "hflip" }

// VFlip creates a vertical flip filter
func VFlip() string { return "vflip" }

// Blur creates a blur filter (sigma: blur strength)
func Blur(sigma float64) string {
    return fmt.Sprintf("gblur=sigma=%.2f", sigma)
}

// Sharpen creates a sharpen filter
func Sharpen(amount float64) string {
    return fmt.Sprintf("unsharp=5:5:%.2f", amount)
}

// Denoise creates a denoise filter
func Denoise(strength int) string {
    return fmt.Sprintf("hqdn3d=%d", strength)
}

// Watermark creates a watermark overlay filter
func Watermark(imagePath string, x, y int) string {
    return fmt.Sprintf("movie=%s[wm];[in][wm]overlay=%d:%d[out]", imagePath, x, y)
}

// FadeIn creates a fade-in effect
func FadeIn(startFrame, numFrames int) string {
    return fmt.Sprintf("fade=in:st=%d:d=%d", startFrame, numFrames)
}

// FadeOut creates a fade-out effect
func FadeOut(startFrame, numFrames int) string {
    return fmt.Sprintf("fade=out:st=%d:d=%d", startFrame, numFrames)
}

// DrawText creates a text overlay filter
func DrawText(text string, x, y int, fontSize int, fontColor string) string {
    return fmt.Sprintf("drawtext=text='%s':x=%d:y=%d:fontsize=%d:fontcolor=%s",
        text, x, y, fontSize, fontColor)
}

// Audio Filters

// Volume adjusts audio volume (1.0 = unchanged)
func Volume(multiplier float64) string {
    return fmt.Sprintf("volume=%.2f", multiplier)
}

// AudioResample resamples audio to target rate
func AudioResample(sampleRate int) string {
    return fmt.Sprintf("aresample=%d", sampleRate)
}

// AudioMix mixes multiple audio inputs
func AudioMix(inputs int) string {
    return fmt.Sprintf("amix=inputs=%d", inputs)
}

// Normalize normalizes audio levels
func Normalize() string {
    return "loudnorm"
}

// Equalizer creates an equalizer filter
func Equalizer(frequency int, width float64, gain float64) string {
    return fmt.Sprintf("equalizer=f=%d:width_type=o:width=%.2f:g=%.1f", frequency, width, gain)
}
```

### 2.5 Complex Filter Graph Example

```go
// Picture-in-picture with fade
func createPIPFilter(mainW, mainH, pipW, pipH, pipX, pipY int) (*FilterGraph, error) {
    // Complex filter with two inputs
    filters := fmt.Sprintf(
        "[1:v]scale=%d:%d[pip];[0:v][pip]overlay=%d:%d[out]",
        pipW, pipH, pipX, pipY,
    )

    return ffgo.NewFilterGraph(ffgo.FilterGraphConfig{
        Filters: filters,
        InputFormat: &ffgo.FilterFormat{
            Width: mainW, Height: mainH,
            PixelFormat: ffgo.PixelFormatYUV420P,
        },
    })
}

// Usage
mainDecoder, _ := ffgo.NewDecoder("main.mp4")
pipDecoder, _ := ffgo.NewDecoder("pip.mp4")
encoder, _ := ffgo.NewEncoder("output.mp4", &ffgo.EncoderOptions{...})

graph, _ := createPIPFilter(1920, 1080, 320, 180, 1580, 880)
defer graph.Close()

for {
    mainFrame, _ := mainDecoder.ReadFrame()
    pipFrame, _ := pipDecoder.ReadFrame()

    if mainFrame == nil {
        break
    }

    // Push both frames to filter
    filtered, err := graph.FilterMultiple([]*Frame{mainFrame, pipFrame})
    if err != nil {
        return err
    }

    for _, f := range filtered {
        encoder.WriteVideoFrame(f)
    }
}
```

### 2.6 Error Handling

| Condition | Behavior |
|-----------|----------|
| Invalid filter string | Return `ErrInvalidFilterString` with parse details |
| Missing required input | Return `ErrMissingInput` |
| Format mismatch | Return `ErrFormatMismatch` with expected vs actual |
| Filter not found | Return `ErrFilterNotFound` with filter name |
| Graph not configured | Return `ErrNotConfigured` |
| FFmpeg internal error | Return `*FFmpegError` |

### 2.7 Shim Requirements

**Minimal** - Only if using `avfilter_graph_parse_ptr` (variadic).
Use `avfilter_graph_parse2` instead (non-variadic).

---

## 3. Audio Encoding

### 3.1 Current Status

Audio encoding is stubbed - `WriteAudioFrame()` returns "not yet implemented".

### 3.2 Implementation Design

```go
// ffgo/encoder.go additions

// AudioEncoderConfig configures audio encoding
type AudioEncoderConfig struct {
    Codec         CodecID        // CodecAAC, CodecOpus, CodecMP3, etc.
    SampleRate    int            // e.g., 44100, 48000
    Channels      int            // 1, 2, 6, 8
    ChannelLayout ChannelLayout  // ChannelLayoutStereo, etc.
    SampleFormat  SampleFormat   // SampleFormatFLTP (most common for encoding)
    Bitrate       int64          // Target bitrate in bits/sec

    // Codec-specific options
    Profile       AudioProfile   // AAC: LC, HE, HEv2
    Quality       float32        // VBR quality (0.0-1.0) - codec dependent
}

// AudioProfile for codec profiles
type AudioProfile int

const (
    AudioProfileDefault AudioProfile = 0
    AudioProfileAAC_LC  AudioProfile = 1
    AudioProfileAAC_HE  AudioProfile = 4
    AudioProfileAAC_HEv2 AudioProfile = 28
)

// WriteAudioFrame encodes and writes an audio frame
// Frame must have correct sample format for the encoder
func (e *Encoder) WriteAudioFrame(frame *Frame) error {
    if e.audioCodecCtx == nil {
        return ErrNoAudioStream
    }

    // Validate frame format matches encoder
    if frame.SampleFormat() != e.audioConfig.SampleFormat {
        return fmt.Errorf("sample format mismatch: got %v, need %v",
            frame.SampleFormat(), e.audioConfig.SampleFormat)
    }

    // Send frame to encoder
    ret := avcodec_send_frame(e.audioCodecCtx, frame.ptr)
    if ret < 0 {
        return newError(ret, "avcodec_send_frame (audio)")
    }

    // Receive and write packets
    for {
        ret := avcodec_receive_packet(e.audioCodecCtx, e.audioPacket)
        if ret == AVERROR_EAGAIN || ret == AVERROR_EOF {
            break
        }
        if ret < 0 {
            return newError(ret, "avcodec_receive_packet (audio)")
        }

        // Set stream index and write
        e.audioPacket.SetStreamIndex(e.audioStreamIndex)
        ret = av_interleaved_write_frame(e.formatCtx, e.audioPacket)
        if ret < 0 {
            return newError(ret, "av_interleaved_write_frame (audio)")
        }

        av_packet_unref(e.audioPacket)
    }

    return nil
}
```

### 3.3 Frame Size Handling

Many audio encoders require specific frame sizes (e.g., AAC = 1024 samples).

```go
// AudioFrameBuffer accumulates samples for fixed-size encoding
type AudioFrameBuffer struct {
    encoder    *Encoder
    buffer     *Frame      // Accumulated samples
    frameSize  int         // Required samples per frame
    accumulated int        // Current sample count
}

// NewAudioFrameBuffer creates a buffer for frame-size-sensitive encoders
func NewAudioFrameBuffer(encoder *Encoder) *AudioFrameBuffer {
    frameSize := encoder.AudioFrameSize() // From codec context
    return &AudioFrameBuffer{
        encoder:   encoder,
        frameSize: frameSize,
        buffer:    allocAudioFrame(encoder.audioConfig),
    }
}

// Write accumulates samples and encodes complete frames
func (b *AudioFrameBuffer) Write(frame *Frame) error {
    // Copy samples to buffer
    // When buffer reaches frameSize, encode and reset
    // Handle partial frames at end
}

// Flush encodes any remaining samples with padding
func (b *AudioFrameBuffer) Flush() error
```

---

## 4. Advanced Codec Options

### 4.1 Design

```go
// ffgo/options.go

// VideoEncoderConfig with advanced options
type VideoEncoderConfig struct {
    // Basic settings (existing)
    Codec     CodecID
    Width     int
    Height    int
    FrameRate Rational
    Bitrate   int64

    // NEW: Preset and tuning
    Preset    EncoderPreset   // ultrafast, fast, medium, slow, veryslow
    Tune      EncoderTune     // film, animation, grain, etc.

    // NEW: Profile and level
    Profile   VideoProfile    // baseline, main, high, high10, etc.
    Level     VideoLevel      // 3.0, 4.0, 4.1, 5.1, etc.

    // NEW: Rate control
    RateControl RateControlMode
    CRF         int           // Constant Rate Factor (0-51 for x264)
    CQP         int           // Constant Quantizer Parameter
    MinBitrate  int64         // For VBR
    MaxBitrate  int64         // For VBR/CBR
    BufferSize  int64         // VBV buffer size

    // NEW: GOP structure
    GOPSize     int           // Keyframe interval
    MaxBFrames  int           // Maximum B-frames
    BFrameStrategy int        // B-frame placement strategy
    RefFrames   int           // Reference frames

    // NEW: Quality settings
    Threads     int           // Encoding threads (0 = auto)

    // NEW: Codec-specific options (passed directly)
    // Example: {"x264-params": "aq-mode=3:psy-rd=1.0"}
    CodecOptions map[string]string
}

// EncoderPreset controls speed/quality tradeoff
type EncoderPreset string

const (
    PresetDefault    EncoderPreset = ""
    PresetUltrafast  EncoderPreset = "ultrafast"
    PresetSuperfast  EncoderPreset = "superfast"
    PresetVeryfast   EncoderPreset = "veryfast"
    PresetFaster     EncoderPreset = "faster"
    PresetFast       EncoderPreset = "fast"
    PresetMedium     EncoderPreset = "medium"
    PresetSlow       EncoderPreset = "slow"
    PresetSlower     EncoderPreset = "slower"
    PresetVeryslow   EncoderPreset = "veryslow"
    PresetPlacebo    EncoderPreset = "placebo"
)

// EncoderTune optimizes for content type
type EncoderTune string

const (
    TuneDefault     EncoderTune = ""
    TuneFilm        EncoderTune = "film"
    TuneAnimation   EncoderTune = "animation"
    TuneGrain       EncoderTune = "grain"
    TuneStillImage  EncoderTune = "stillimage"
    TuneFastDecode  EncoderTune = "fastdecode"
    TuneZeroLatency EncoderTune = "zerolatency"
)

// VideoProfile for codec profiles
type VideoProfile string

const (
    ProfileDefault    VideoProfile = ""
    ProfileBaseline   VideoProfile = "baseline"
    ProfileMain       VideoProfile = "main"
    ProfileHigh       VideoProfile = "high"
    ProfileHigh10     VideoProfile = "high10"
    ProfileHigh422    VideoProfile = "high422"
    ProfileHigh444    VideoProfile = "high444"
    // HEVC
    ProfileMain10HEVC VideoProfile = "main10"
)

// VideoLevel for codec levels
type VideoLevel string

const (
    LevelDefault VideoLevel = ""
    Level30      VideoLevel = "3.0"
    Level31      VideoLevel = "3.1"
    Level40      VideoLevel = "4.0"
    Level41      VideoLevel = "4.1"
    Level50      VideoLevel = "5.0"
    Level51      VideoLevel = "5.1"
    Level52      VideoLevel = "5.2"
)

// RateControlMode specifies bitrate control method
type RateControlMode int

const (
    RateControlDefault RateControlMode = iota
    RateControlCRF     // Constant Rate Factor (quality-based)
    RateControlCQP     // Constant Quantizer Parameter
    RateControlCBR     // Constant Bitrate
    RateControlVBR     // Variable Bitrate
    RateControlABR     // Average Bitrate
)
```

### 4.2 Implementation

```go
// applyVideoEncoderOptions applies VideoEncoderConfig to AVCodecContext
func applyVideoEncoderOptions(ctx unsafe.Pointer, config *VideoEncoderConfig) error {
    // Basic options
    setCodecContextInt(ctx, "width", config.Width)
    setCodecContextInt(ctx, "height", config.Height)
    setCodecContextBitrate(ctx, config.Bitrate)

    // Preset (via private options)
    if config.Preset != "" {
        av_opt_set(ctx, "preset", string(config.Preset), AV_OPT_SEARCH_CHILDREN)
    }

    // Tune
    if config.Tune != "" {
        av_opt_set(ctx, "tune", string(config.Tune), AV_OPT_SEARCH_CHILDREN)
    }

    // Profile
    if config.Profile != "" {
        av_opt_set(ctx, "profile", string(config.Profile), AV_OPT_SEARCH_CHILDREN)
    }

    // Level
    if config.Level != "" {
        av_opt_set(ctx, "level", string(config.Level), AV_OPT_SEARCH_CHILDREN)
    }

    // Rate control
    switch config.RateControl {
    case RateControlCRF:
        av_opt_set_int(ctx, "crf", int64(config.CRF), AV_OPT_SEARCH_CHILDREN)
    case RateControlCQP:
        av_opt_set_int(ctx, "qp", int64(config.CQP), AV_OPT_SEARCH_CHILDREN)
    case RateControlCBR:
        setCodecContextBitrate(ctx, config.Bitrate)
        setCodecContextMaxBitrate(ctx, config.Bitrate)
        setCodecContextBufferSize(ctx, config.BufferSize)
    case RateControlVBR:
        setCodecContextBitrate(ctx, config.Bitrate)
        setCodecContextMinBitrate(ctx, config.MinBitrate)
        setCodecContextMaxBitrate(ctx, config.MaxBitrate)
    }

    // GOP structure
    if config.GOPSize > 0 {
        setCodecContextGOPSize(ctx, config.GOPSize)
    }
    if config.MaxBFrames >= 0 {
        setCodecContextMaxBFrames(ctx, config.MaxBFrames)
    }
    if config.RefFrames > 0 {
        av_opt_set_int(ctx, "refs", int64(config.RefFrames), AV_OPT_SEARCH_CHILDREN)
    }

    // Threads
    if config.Threads > 0 {
        setCodecContextThreadCount(ctx, config.Threads)
    }

    // Custom codec options
    for key, value := range config.CodecOptions {
        av_opt_set(ctx, key, value, AV_OPT_SEARCH_CHILDREN)
    }

    return nil
}
```

### 4.3 Usage Examples

```go
// High-quality encoding for archival
encoder, err := ffgo.NewEncoder("output.mp4", &ffgo.EncoderOptions{
    Video: &ffgo.VideoEncoderConfig{
        Codec:       ffgo.CodecH264,
        Width:       1920,
        Height:      1080,
        FrameRate:   ffgo.NewRational(30, 1),
        Preset:      ffgo.PresetSlow,
        RateControl: ffgo.RateControlCRF,
        CRF:         18,
        Profile:     ffgo.ProfileHigh,
        Level:       ffgo.Level41,
    },
})

// Fast encoding for streaming
encoder, err := ffgo.NewEncoder("output.mp4", &ffgo.EncoderOptions{
    Video: &ffgo.VideoEncoderConfig{
        Codec:       ffgo.CodecH264,
        Width:       1280,
        Height:      720,
        FrameRate:   ffgo.NewRational(30, 1),
        Preset:      ffgo.PresetVeryfast,
        Tune:        ffgo.TuneZeroLatency,
        RateControl: ffgo.RateControlCBR,
        Bitrate:     2_000_000,
        GOPSize:     30,  // 1 second keyframe interval
        MaxBFrames:  0,   // No B-frames for low latency
    },
})

// HEVC encoding with 10-bit
encoder, err := ffgo.NewEncoder("output.mp4", &ffgo.EncoderOptions{
    Video: &ffgo.VideoEncoderConfig{
        Codec:       ffgo.CodecHEVC,
        Width:       3840,
        Height:      2160,
        Preset:      ffgo.PresetMedium,
        Profile:     ffgo.ProfileMain10HEVC,
        RateControl: ffgo.RateControlCRF,
        CRF:         22,
    },
})
```

---

## 5. Subtitle Support

### 5.1 FFmpeg API Analysis

```c
// Subtitle decoding
int avcodec_decode_subtitle2(AVCodecContext *avctx, AVSubtitle *sub,
                             int *got_sub_ptr, AVPacket *avpkt);  // 4 args ✓

// Subtitle encoding
int avcodec_encode_subtitle(AVCodecContext *avctx, uint8_t *buf, int buf_size,
                            const AVSubtitle *sub);               // 4 args ✓

// Subtitle structure freeing
void avsubtitle_free(AVSubtitle *sub);                           // 1 arg ✓
```

**purego Constraints**:
- All functions compatible ✓
- `AVSubtitle` is a struct, but passed by pointer ✓

### 5.2 Design

```go
// ffgo/subtitle.go
package ffgo

// SubtitleType indicates the subtitle format
type SubtitleType int

const (
    SubtitleTypeNone   SubtitleType = 0
    SubtitleTypeBitmap SubtitleType = 1  // DVD, Blu-ray
    SubtitleTypeText   SubtitleType = 2  // SRT, ASS, WebVTT
    SubtitleTypeASS    SubtitleType = 3  // ASS/SSA with styling
)

// Subtitle represents a decoded subtitle
type Subtitle struct {
    Type      SubtitleType
    StartTime time.Duration  // Display start
    EndTime   time.Duration  // Display end

    // For text subtitles
    Text      string         // Plain text (SRT)
    ASS       string         // ASS markup (if available)

    // For bitmap subtitles
    Rects     []SubtitleRect // Bitmap regions
}

// SubtitleRect represents a bitmap subtitle region
type SubtitleRect struct {
    X, Y          int      // Position
    Width, Height int      // Dimensions
    Data          []byte   // Bitmap data (RGBA or palette)
    Palette       []uint32 // Color palette (if indexed)
}

// SubtitleDecoder decodes subtitle streams
type SubtitleDecoder struct {
    codecCtx unsafe.Pointer
    subtitle unsafe.Pointer // Reusable AVSubtitle
}

// NewSubtitleDecoder creates a subtitle decoder
func NewSubtitleDecoder(stream *StreamInfo) (*SubtitleDecoder, error)

// Decode decodes a subtitle packet
func (d *SubtitleDecoder) Decode(packet *Packet) (*Subtitle, error)

// Close releases resources
func (d *SubtitleDecoder) Close() error


// SubtitleEncoder encodes subtitles
type SubtitleEncoder struct {
    codecCtx unsafe.Pointer
}

// NewSubtitleEncoder creates a subtitle encoder
func NewSubtitleEncoder(format SubtitleFormat, opts *SubtitleEncoderOptions) (*SubtitleEncoder, error)

// Encode encodes a subtitle
func (e *SubtitleEncoder) Encode(sub *Subtitle) (*Packet, error)

// Close releases resources
func (e *SubtitleEncoder) Close() error


// SubtitleFormat specifies output format
type SubtitleFormat int

const (
    SubtitleFormatSRT    SubtitleFormat = iota
    SubtitleFormatASS
    SubtitleFormatWebVTT
    SubtitleFormatMOVText
)
```

### 5.3 Subtitle Rendering (Burn-in)

```go
// SubtitleRenderer burns subtitles into video frames
type SubtitleRenderer struct {
    graph *FilterGraph  // Uses subtitles filter
}

// NewSubtitleRenderer creates a renderer from subtitle file
func NewSubtitleRenderer(subtitlePath string, videoWidth, videoHeight int) (*SubtitleRenderer, error) {
    // Use FFmpeg's subtitles filter
    filters := fmt.Sprintf("subtitles=%s", subtitlePath)

    graph, err := NewVideoFilterGraph(filters, videoWidth, videoHeight, PixelFormatYUV420P)
    if err != nil {
        return nil, err
    }

    return &SubtitleRenderer{graph: graph}, nil
}

// Render burns subtitle onto frame at given timestamp
func (r *SubtitleRenderer) Render(frame *Frame) (*Frame, error) {
    return r.graph.Filter(frame)
}

// Close releases resources
func (r *SubtitleRenderer) Close() error {
    return r.graph.Close()
}
```

### 5.4 Usage Examples

```go
// Extract subtitles from video
decoder, _ := ffgo.NewDecoder("video.mkv")
subDecoder, _ := ffgo.NewSubtitleDecoder(decoder.SubtitleStream())
defer subDecoder.Close()

var subtitles []*ffgo.Subtitle
for {
    packet, _ := decoder.ReadPacket()
    if packet == nil {
        break
    }

    if packet.StreamIndex() == decoder.SubtitleStreamIndex() {
        sub, err := subDecoder.Decode(packet)
        if err == nil && sub != nil {
            subtitles = append(subtitles, sub)
        }
    }
}

// Write to SRT file
writeSRT(subtitles, "output.srt")

// Burn subtitles into video
renderer, _ := ffgo.NewSubtitleRenderer("subs.srt", 1920, 1080)
defer renderer.Close()

for {
    frame, _ := decoder.ReadFrame()
    if frame == nil {
        break
    }

    rendered, _ := renderer.Render(frame)
    encoder.WriteVideoFrame(rendered)
}
```

---

## 6. Bitstream Filters

### 6.1 FFmpeg API Analysis

```c
const AVBitStreamFilter *av_bsf_get_by_name(const char *name);   // 1 arg ✓
int av_bsf_alloc(const AVBitStreamFilter *filter, AVBSFContext **ctx); // 2 args ✓
int av_bsf_init(AVBSFContext *ctx);                              // 1 arg ✓
int av_bsf_send_packet(AVBSFContext *ctx, AVPacket *pkt);        // 2 args ✓
int av_bsf_receive_packet(AVBSFContext *ctx, AVPacket *pkt);     // 2 args ✓
void av_bsf_free(AVBSFContext **ctx);                            // 1 arg ✓
```

All functions are purego-compatible ✓

### 6.2 Design

```go
// ffgo/bsf.go
package ffgo

// BitstreamFilter applies packet-level transformations
type BitstreamFilter struct {
    ctx    unsafe.Pointer  // AVBSFContext
    packet unsafe.Pointer  // Reusable output packet
}

// Common bitstream filter names
const (
    BSFh264MP4toAnnexB    = "h264_mp4toannexb"  // MP4 → raw H.264
    BSFHevcMP4toAnnexB    = "hevc_mp4toannexb"  // MP4 → raw HEVC
    BSFAACtoADTS          = "aac_adtstoasc"     // ADTS → ASC
    BSFExtractExtradata   = "extract_extradata"
    BSFRemoveExtradata    = "remove_extradata"
    BSFDumpExtradata      = "dump_extra"
    BSFH264Metadata       = "h264_metadata"
    BSFNull               = "null"              // Pass-through
)

// NewBitstreamFilter creates a bitstream filter
func NewBitstreamFilter(name string, codecPar *CodecParameters) (*BitstreamFilter, error)

// Filter processes a packet through the filter
// May return multiple packets or none (buffering)
func (f *BitstreamFilter) Filter(packet *Packet) ([]*Packet, error)

// Flush drains remaining packets
func (f *BitstreamFilter) Flush() ([]*Packet, error)

// Close releases resources
func (f *BitstreamFilter) Close() error
```

### 6.3 Usage: Stream Copy with Format Conversion

```go
// Copy H.264 from MP4 to TS (requires Annex B conversion)
func remuxMP4toTS(input, output string) error {
    decoder, _ := ffgo.NewDecoder(input)
    defer decoder.Close()

    encoder, _ := ffgo.NewEncoder(output, &ffgo.EncoderOptions{
        // Stream copy mode (no re-encoding)
        CopyVideo: true,
        CopyAudio: true,
    })
    defer encoder.Close()

    // Create bitstream filter for H.264 Annex B conversion
    bsf, _ := ffgo.NewBitstreamFilter(ffgo.BSFh264MP4toAnnexB, decoder.VideoStream().CodecParameters())
    defer bsf.Close()

    for {
        packet, _ := decoder.ReadPacket()
        if packet == nil {
            break
        }

        if packet.StreamIndex() == decoder.VideoStreamIndex() {
            // Filter video packets
            filtered, _ := bsf.Filter(packet)
            for _, p := range filtered {
                encoder.WritePacket(p)
            }
        } else {
            // Pass through other packets
            encoder.WritePacket(packet)
        }
    }

    // Flush BSF
    for _, p := range bsf.Flush() {
        encoder.WritePacket(p)
    }

    return nil
}
```

---

## 7. Stream Copy Mode

### 7.1 Design

```go
// ffgo/encoder.go additions

// EncoderOptions with stream copy support
type EncoderOptions struct {
    Video *VideoEncoderConfig
    Audio *AudioEncoderConfig

    // NEW: Stream copy options (fast remuxing)
    CopyVideo bool  // Copy video without re-encoding
    CopyAudio bool  // Copy audio without re-encoding

    // NEW: Source stream for copy mode
    // Required when CopyVideo or CopyAudio is true
    SourceStreams *StreamCopySource
}

// StreamCopySource provides source codec parameters for stream copy
type StreamCopySource struct {
    VideoParams *CodecParameters
    AudioParams *CodecParameters
}

// WritePacket writes a packet directly (for stream copy)
// Packet timestamps are adjusted for the output container
func (e *Encoder) WritePacket(packet *Packet) error

// Example: Fast remux without re-encoding
func fastRemux(input, output string) error {
    decoder, _ := ffgo.NewDecoder(input)
    defer decoder.Close()

    encoder, _ := ffgo.NewEncoder(output, &ffgo.EncoderOptions{
        CopyVideo: true,
        CopyAudio: true,
        SourceStreams: &ffgo.StreamCopySource{
            VideoParams: decoder.VideoStream().CodecParameters(),
            AudioParams: decoder.AudioStream().CodecParameters(),
        },
    })
    defer encoder.Close()

    for {
        packet, _ := decoder.ReadPacket()
        if packet == nil {
            break
        }
        encoder.WritePacket(packet)
    }

    return nil
}
```

---

## 8. Hardware Acceleration (Complete)

### 8.1 Current Status

Only basic hook exists (`WithHWDevice`). No actual HW context setup.

### 8.2 Complete Design

```go
// ffgo/hwaccel.go
package ffgo

// HWDeviceType specifies hardware acceleration backend
type HWDeviceType int

const (
    HWDeviceNone        HWDeviceType = iota
    HWDeviceCUDA        // NVIDIA CUDA
    HWDeviceVAAPI       // Linux VA-API
    HWDeviceVDPAU       // Linux VDPAU (legacy)
    HWDeviceDXVA2       // Windows DirectX Video Acceleration
    HWDeviceD3D11VA     // Windows Direct3D 11
    HWDeviceVideoToolbox // macOS/iOS
    HWDeviceQSV         // Intel Quick Sync Video
    HWDeviceOpenCL      // OpenCL
    HWDeviceVulkan      // Vulkan Video
)

// HWDevice represents a hardware device for acceleration
type HWDevice struct {
    deviceType HWDeviceType
    deviceCtx  unsafe.Pointer  // AVBufferRef* to AVHWDeviceContext
    devicePath string          // e.g., "/dev/dri/renderD128"
}

// NewHWDevice creates a hardware device context
func NewHWDevice(deviceType HWDeviceType, device string) (*HWDevice, error) {
    var deviceCtx unsafe.Pointer

    ret := av_hwdevice_ctx_create(&deviceCtx,
        int32(deviceType),
        device, // Can be "" for default
        nil,    // Options dictionary
        0)      // Flags

    if ret < 0 {
        return nil, newError(ret, "av_hwdevice_ctx_create")
    }

    return &HWDevice{
        deviceType: deviceType,
        deviceCtx:  deviceCtx,
        devicePath: device,
    }, nil
}

// AvailableDevices returns available HW devices of given type
func AvailableDevices(deviceType HWDeviceType) []string {
    var devices []string
    // Use av_hwdevice_iterate_types and av_hwdevice_ctx_create to probe
    return devices
}

// Close releases the device
func (d *HWDevice) Close() error {
    if d.deviceCtx != nil {
        av_buffer_unref(&d.deviceCtx)
    }
    return nil
}


// HWAcceleratedDecoder wraps decoder with HW acceleration
type HWAcceleratedDecoder struct {
    *Decoder
    hwDevice   *HWDevice
    hwPixFmt   PixelFormat  // Hardware pixel format
    swPixFmt   PixelFormat  // Software pixel format for transfer
}

// NewHWDecoder creates a hardware-accelerated decoder
func NewHWDecoder(path string, hwDevice *HWDevice) (*HWAcceleratedDecoder, error) {
    decoder, err := NewDecoder(path)
    if err != nil {
        return nil, err
    }

    // Set hardware device on codec context
    decoder.codecCtx.hw_device_ctx = av_buffer_ref(hwDevice.deviceCtx)

    // Set get_format callback to select HW pixel format
    // This requires a callback - use handle system

    return &HWAcceleratedDecoder{
        Decoder:  decoder,
        hwDevice: hwDevice,
    }, nil
}

// ReadFrame reads and optionally transfers frame to system memory
func (d *HWAcceleratedDecoder) ReadFrame() (*Frame, error) {
    frame, err := d.Decoder.ReadFrame()
    if err != nil || frame == nil {
        return frame, err
    }

    // Frame is in HW memory - transfer to system memory if needed
    if frame.PixelFormat() == d.hwPixFmt {
        return d.TransferToSystem(frame)
    }

    return frame, nil
}

// ReadHWFrame reads frame without transfer (stays in GPU memory)
func (d *HWAcceleratedDecoder) ReadHWFrame() (*Frame, error) {
    return d.Decoder.ReadFrame()
}

// TransferToSystem copies frame from GPU to system memory
func (d *HWAcceleratedDecoder) TransferToSystem(hwFrame *Frame) (*Frame, error) {
    swFrame := av_frame_alloc()
    ret := av_hwframe_transfer_data(swFrame, hwFrame.ptr, 0)
    if ret < 0 {
        av_frame_free(&swFrame)
        return nil, newError(ret, "av_hwframe_transfer_data")
    }
    return &Frame{ptr: swFrame}, nil
}
```

### 8.3 Usage Examples

```go
// NVIDIA CUDA decoding
hwDevice, err := ffgo.NewHWDevice(ffgo.HWDeviceCUDA, "")
if err != nil {
    // Fallback to software
    decoder, _ = ffgo.NewDecoder(input)
} else {
    defer hwDevice.Close()
    decoder, _ = ffgo.NewHWDecoder(input, hwDevice)
}

// Process frames (automatically transferred to system memory)
for {
    frame, _ := decoder.ReadFrame()
    if frame == nil {
        break
    }
    // frame is in system memory, ready for processing
}

// VA-API on Linux
hwDevice, _ := ffgo.NewHWDevice(ffgo.HWDeviceVAAPI, "/dev/dri/renderD128")

// VideoToolbox on macOS
hwDevice, _ := ffgo.NewHWDevice(ffgo.HWDeviceVideoToolbox, "")

// Intel QSV
hwDevice, _ := ffgo.NewHWDevice(ffgo.HWDeviceQSV, "")
```

---

## 9. Multi-Stream Muxing

### 9.1 Design

```go
// ffgo/muxer.go
package ffgo

// Muxer combines multiple streams into a container
type Muxer struct {
    formatCtx    unsafe.Pointer
    streams      []*MuxerStream
    headerWritten bool
}

// MuxerStream represents a stream being muxed
type MuxerStream struct {
    index      int
    codecPar   *CodecParameters
    timeBase   Rational
    encoder    *Encoder      // Optional: for encoding
    copyMode   bool          // Stream copy mode
}

// NewMuxer creates a muxer for the given output format
func NewMuxer(path string, format string) (*Muxer, error)

// AddVideoStream adds a video stream
func (m *Muxer) AddVideoStream(config *VideoEncoderConfig) (*MuxerStream, error)

// AddAudioStream adds an audio stream
func (m *Muxer) AddAudioStream(config *AudioEncoderConfig) (*MuxerStream, error)

// AddSubtitleStream adds a subtitle stream
func (m *Muxer) AddSubtitleStream(format SubtitleFormat) (*MuxerStream, error)

// AddCopyStream adds a stream in copy mode (no re-encoding)
func (m *Muxer) AddCopyStream(codecPar *CodecParameters) (*MuxerStream, error)

// WriteHeader writes the container header
func (m *Muxer) WriteHeader() error

// WriteFrame writes an encoded frame to a stream
func (m *Muxer) WriteFrame(stream *MuxerStream, frame *Frame) error

// WritePacket writes a packet directly (for stream copy)
func (m *Muxer) WritePacket(stream *MuxerStream, packet *Packet) error

// WriteTrailer finalizes the container
func (m *Muxer) WriteTrailer() error

// Close releases all resources
func (m *Muxer) Close() error
```

### 9.2 Usage: Multiple Audio Tracks

```go
// Create output with multiple audio tracks
muxer, _ := ffgo.NewMuxer("output.mkv", "matroska")

// Add video
videoStream, _ := muxer.AddVideoStream(&ffgo.VideoEncoderConfig{
    Codec: ffgo.CodecH264,
    Width: 1920, Height: 1080,
})

// Add English audio
audioEN, _ := muxer.AddAudioStream(&ffgo.AudioEncoderConfig{
    Codec: ffgo.CodecAAC,
    SampleRate: 48000, Channels: 2,
})

// Add Spanish audio
audioES, _ := muxer.AddAudioStream(&ffgo.AudioEncoderConfig{
    Codec: ffgo.CodecAAC,
    SampleRate: 48000, Channels: 2,
})

// Add subtitles
subs, _ := muxer.AddSubtitleStream(ffgo.SubtitleFormatSRT)

muxer.WriteHeader()

// Write frames to appropriate streams
for {
    // Read from sources and write to streams
    muxer.WriteFrame(videoStream, videoFrame)
    muxer.WriteFrame(audioEN, audioENFrame)
    muxer.WriteFrame(audioES, audioESFrame)
}

muxer.WriteTrailer()
muxer.Close()
```

---

## 10. Advanced Seeking

### 10.1 Design

```go
// ffgo/seek.go
package ffgo

// SeekFlags control seeking behavior
type SeekFlags int

const (
    SeekBackward SeekFlags = 1 << iota // Seek to keyframe before target
    SeekByte                            // Seek by byte position
    SeekAny                             // Seek to any frame (not just keyframes)
    SeekFrame                           // Seek by frame number
)

// SeekOptions configures seek behavior
type SeekOptions struct {
    Flags       SeekFlags
    StreamIndex int  // -1 for default (usually video)
}

// Seek seeks to the specified position
func (d *Decoder) Seek(position time.Duration, opts ...SeekOption) error

// SeekToFrame seeks to a specific frame number
func (d *Decoder) SeekToFrame(frameNum int64) error

// SeekToByte seeks to a byte position
func (d *Decoder) SeekToByte(bytePos int64) error

// SeekPrecise performs frame-accurate seeking
// Seeks to keyframe before target, then decodes forward
func (d *Decoder) SeekPrecise(position time.Duration) error {
    // 1. Seek backward to keyframe
    err := d.Seek(position, WithSeekBackward())
    if err != nil {
        return err
    }

    // 2. Decode forward until we reach target
    targetPTS := position.Microseconds()
    for {
        frame, err := d.ReadFrame()
        if err != nil {
            return err
        }
        if frame == nil {
            return ErrEOF
        }

        if frame.PTS().Microseconds() >= targetPTS {
            d.pushBackFrame(frame)  // Put frame back for next read
            return nil
        }
        // Discard frames before target
    }
}

// GetKeyframes returns keyframe positions for building index
func (d *Decoder) GetKeyframes() ([]KeyframeInfo, error)

// KeyframeInfo describes a keyframe
type KeyframeInfo struct {
    PTS      time.Duration
    DTS      time.Duration
    Position int64  // Byte position
    Size     int    // Packet size
}

// GenerateThumbnails extracts thumbnails at intervals
func (d *Decoder) GenerateThumbnails(interval time.Duration, maxCount int) ([]*Frame, error)
```

---

## 11. Metadata Handling

### 11.1 Design

```go
// ffgo/metadata.go
package ffgo

// Metadata represents key-value metadata tags
type Metadata map[string]string

// Common metadata keys
const (
    MetadataTitle       = "title"
    MetadataArtist      = "artist"
    MetadataAlbum       = "album"
    MetadataAlbumArtist = "album_artist"
    MetadataComposer    = "composer"
    MetadataGenre       = "genre"
    MetadataDate        = "date"
    MetadataTrack       = "track"
    MetadataDisc        = "disc"
    MetadataComment     = "comment"
    MetadataEncoder     = "encoder"
    MetadataCopyright   = "copyright"
    MetadataLanguage    = "language"
)

// GetMetadata returns file-level metadata
func (d *Decoder) GetMetadata() Metadata

// GetStreamMetadata returns metadata for a specific stream
func (d *Decoder) GetStreamMetadata(streamIndex int) Metadata

// SetMetadata sets file-level metadata on encoder
func (e *Encoder) SetMetadata(meta Metadata) error

// SetStreamMetadata sets stream-level metadata
func (e *Encoder) SetStreamMetadata(streamIndex int, meta Metadata) error


// Chapter represents a chapter marker
type Chapter struct {
    ID        int
    Start     time.Duration
    End       time.Duration
    Title     string
    Metadata  Metadata
}

// GetChapters returns chapters from the file
func (d *Decoder) GetChapters() []Chapter

// SetChapters sets chapters on encoder
func (e *Encoder) SetChapters(chapters []Chapter) error


// Attachment represents an embedded file
type Attachment struct {
    Filename    string
    MimeType    string
    Description string
    Data        []byte
}

// GetAttachments returns attachments from MKV/other containers
func (d *Decoder) GetAttachments() []Attachment

// AddAttachment embeds a file in the output
func (e *Encoder) AddAttachment(att Attachment) error
```

### 11.2 Usage

```go
// Read metadata
decoder, _ := ffgo.NewDecoder("music.mp4")
meta := decoder.GetMetadata()
fmt.Printf("Title: %s\n", meta[ffgo.MetadataTitle])
fmt.Printf("Artist: %s\n", meta[ffgo.MetadataArtist])

// Copy with modified metadata
encoder, _ := ffgo.NewEncoder("output.mp4", opts)
encoder.SetMetadata(ffgo.Metadata{
    ffgo.MetadataTitle:   "New Title",
    ffgo.MetadataArtist:  meta[ffgo.MetadataArtist],
    ffgo.MetadataEncoder: "ffgo",
})

// Chapters
chapters := decoder.GetChapters()
for _, ch := range chapters {
    fmt.Printf("Chapter: %s (%v - %v)\n", ch.Title, ch.Start, ch.End)
}

// Add chapters to output
encoder.SetChapters([]ffgo.Chapter{
    {Start: 0, End: 60*time.Second, Title: "Introduction"},
    {Start: 60*time.Second, End: 300*time.Second, Title: "Main Content"},
})
```

---

## 12. Device I/O

### 12.1 FFmpeg API Analysis

```c
// Device enumeration
int avdevice_list_devices(AVFormatContext *s, AVDeviceInfoList **device_list); // 2 args ✓
void avdevice_free_list_devices(AVDeviceInfoList **device_list);               // 1 arg ✓

// Opening devices uses standard avformat API
avformat_open_input(&ctx, "device_name", input_format, &options);
```

### 12.2 Design

```go
// ffgo/device.go
package ffgo

// DeviceType specifies input device category
type DeviceType int

const (
    DeviceTypeVideo DeviceType = iota  // Camera, screen capture
    DeviceTypeAudio                     // Microphone, audio input
)

// Device represents an available input device
type Device struct {
    Name        string
    Description string
    Type        DeviceType
    IsDefault   bool
}

// ListDevices returns available devices of given type
// Platform-specific:
//   Linux: v4l2, alsa, pulse
//   macOS: avfoundation
//   Windows: dshow
func ListDevices(deviceType DeviceType) ([]Device, error)

// CaptureConfig configures device capture
type CaptureConfig struct {
    Device     string           // Device name or path
    DeviceType DeviceType

    // Video options
    Width      int
    Height     int
    FrameRate  Rational
    PixelFormat PixelFormat

    // Audio options
    SampleRate int
    Channels   int
}

// NewCapture creates a capture source from a device
func NewCapture(config CaptureConfig) (*Decoder, error) {
    // Build device URL based on platform
    var url string
    var format string

    switch runtime.GOOS {
    case "linux":
        if config.DeviceType == DeviceTypeVideo {
            url = config.Device  // e.g., "/dev/video0"
            format = "v4l2"
        } else {
            url = config.Device
            format = "pulse"  // or "alsa"
        }
    case "darwin":
        url = config.Device
        format = "avfoundation"
    case "windows":
        url = fmt.Sprintf("video=%s", config.Device)
        format = "dshow"
    }

    // Open with format hint
    return NewDecoder(url, WithFormat(format), WithAVOptions(map[string]string{
        "video_size": fmt.Sprintf("%dx%d", config.Width, config.Height),
        "framerate":  fmt.Sprintf("%d/%d", config.FrameRate.Num, config.FrameRate.Den),
    }))
}
```

### 12.3 Screen Capture

```go
// Screen capture (platform-specific)
func CaptureScreen(region *Rect, frameRate Rational) (*Decoder, error) {
    switch runtime.GOOS {
    case "linux":
        // x11grab
        url := fmt.Sprintf(":0.0+%d,%d", region.X, region.Y)
        return NewDecoder(url, WithFormat("x11grab"), WithAVOptions(map[string]string{
            "video_size": fmt.Sprintf("%dx%d", region.Width, region.Height),
            "framerate":  fmt.Sprintf("%d", frameRate.Num/frameRate.Den),
        }))
    case "darwin":
        // avfoundation with screen device
        return NewDecoder("1:", WithFormat("avfoundation"))
    case "windows":
        // gdigrab
        return NewDecoder("desktop", WithFormat("gdigrab"))
    }
    return nil, ErrUnsupportedPlatform
}
```

---

## 13. Network Protocols

### 13.1 Design

```go
// ffgo/protocol.go
package ffgo

// ProtocolOptions configures network protocol behavior
type ProtocolOptions struct {
    // Connection options
    Timeout        time.Duration  // Connection timeout
    ReconnectCount int            // Auto-reconnect attempts
    ReconnectDelay time.Duration  // Delay between reconnects

    // Buffer options
    BufferSize     int            // Protocol buffer size
    MaxDelay       time.Duration  // Maximum delay for buffering

    // RTMP specific
    RTMPApp        string         // RTMP application name
    RTMPPlayPath   string         // RTMP stream name
    RTMPLive       bool           // Live stream mode

    // HTTP specific
    HTTPHeaders    map[string]string
    HTTPCookies    string

    // TLS/SSL
    TLSVerify      bool           // Verify TLS certificates
    TLSCert        string         // Client certificate
    TLSKey         string         // Client key
}

// NewNetworkDecoder opens a network stream
func NewNetworkDecoder(url string, opts *ProtocolOptions) (*Decoder, error)

// Example usage
decoder, err := ffgo.NewNetworkDecoder("rtmp://server/live/stream", &ffgo.ProtocolOptions{
    Timeout:        10 * time.Second,
    ReconnectCount: 3,
    RTMPLive:       true,
})

// HLS playback
decoder, err := ffgo.NewNetworkDecoder("https://example.com/stream.m3u8", &ffgo.ProtocolOptions{
    Timeout: 10 * time.Second,
    HTTPHeaders: map[string]string{
        "User-Agent": "ffgo/1.0",
    },
})
```

---

## 14. Image Sequences

### 14.1 Design

```go
// ffgo/imageseq.go
package ffgo

// ImageSequenceConfig configures image sequence reading/writing
type ImageSequenceConfig struct {
    Pattern     string        // e.g., "frame_%04d.png"
    StartNumber int           // First frame number
    FrameRate   Rational      // Output frame rate
    PixelFormat PixelFormat   // Output pixel format
}

// NewImageSequenceDecoder reads an image sequence as video
func NewImageSequenceDecoder(config ImageSequenceConfig) (*Decoder, error) {
    return NewDecoder(config.Pattern,
        WithFormat("image2"),
        WithAVOptions(map[string]string{
            "start_number": fmt.Sprintf("%d", config.StartNumber),
            "framerate":    fmt.Sprintf("%d/%d", config.FrameRate.Num, config.FrameRate.Den),
        }))
}

// NewImageSequenceEncoder writes video as image sequence
func NewImageSequenceEncoder(config ImageSequenceConfig, pixFmt PixelFormat) (*Encoder, error) {
    return NewEncoder(config.Pattern, &EncoderOptions{
        Video: &VideoEncoderConfig{
            Codec:       CodecPNG,  // or CodecJPEG, CodecBMP
            PixelFormat: pixFmt,
            FrameRate:   config.FrameRate,
        },
    })
}

// ExtractFrame extracts a single frame at given time
func ExtractFrame(input string, timestamp time.Duration, output string) error {
    decoder, err := NewDecoder(input)
    if err != nil {
        return err
    }
    defer decoder.Close()

    err = decoder.SeekPrecise(timestamp)
    if err != nil {
        return err
    }

    frame, err := decoder.ReadFrame()
    if err != nil {
        return err
    }

    return SaveFrame(frame, output)
}

// SaveFrame saves a frame as an image
func SaveFrame(frame *Frame, path string) error
```

---

## 15. Color Space Handling

### 15.1 Design

```go
// ffgo/colorspace.go
package ffgo

// ColorSpace represents color space
type ColorSpace int

const (
    ColorSpaceUnspecified ColorSpace = 0
    ColorSpaceBT709       ColorSpace = 1   // HD (sRGB)
    ColorSpaceBT470BG     ColorSpace = 5   // PAL
    ColorSpaceSMPTE170M   ColorSpace = 6   // NTSC
    ColorSpaceBT2020NCL   ColorSpace = 9   // UHD non-constant luminance
    ColorSpaceBT2020CL    ColorSpace = 10  // UHD constant luminance
)

// ColorRange represents color range
type ColorRange int

const (
    ColorRangeUnspecified ColorRange = 0
    ColorRangeMPEG        ColorRange = 1  // Limited (16-235)
    ColorRangeJPEG        ColorRange = 2  // Full (0-255)
)

// ColorPrimaries represents color primaries
type ColorPrimaries int

const (
    ColorPrimariesUnspecified ColorPrimaries = 0
    ColorPrimariesBT709       ColorPrimaries = 1
    ColorPrimariesBT2020      ColorPrimaries = 9
    ColorPrimariesDCIP3       ColorPrimaries = 11
)

// ColorTransfer represents transfer characteristics
type ColorTransfer int

const (
    ColorTransferUnspecified ColorTransfer = 0
    ColorTransferBT709       ColorTransfer = 1
    ColorTransferSRGB        ColorTransfer = 13
    ColorTransferPQ          ColorTransfer = 16  // HDR10
    ColorTransferHLG         ColorTransfer = 18  // HLG HDR
)

// ColorProperties describes frame color characteristics
type ColorProperties struct {
    Space     ColorSpace
    Range     ColorRange
    Primaries ColorPrimaries
    Transfer  ColorTransfer
}

// Frame color accessors
func (f *Frame) ColorSpace() ColorSpace
func (f *Frame) ColorRange() ColorRange
func (f *Frame) ColorPrimaries() ColorPrimaries
func (f *Frame) ColorTransfer() ColorTransfer
func (f *Frame) ColorProperties() ColorProperties

// ColorConverter converts between color spaces
type ColorConverter struct {
    scaler *Scaler  // Uses swscale with color conversion flags
}

// NewColorConverter creates a color space converter
func NewColorConverter(src, dst ColorProperties, width, height int) (*ColorConverter, error)

// Convert converts frame color properties
func (c *ColorConverter) Convert(frame *Frame) (*Frame, error)
```

---

## Shim Updates Required

### Additional Shim Functions

The following FFmpeg functions require shim wrappers for variadics or struct returns:

```c
// Only if using avfilter_graph_parse_ptr (prefer parse2)
// void avfilter_graph_parse_ptr(...) - VARIADIC, need shim if used

// Most functions are already purego-compatible
// No additional shim functions required for these features
```

**Conclusion**: Most new features require **no shim updates** - they use non-variadic FFmpeg functions.

---

## Implementation Priority

| Priority | Feature | Effort | Impact |
|----------|---------|--------|--------|
| **P0** | Audio Encoding | Medium | Unblocks audio transcoding |
| **P0** | Audio Resampling | Medium | Required for audio encoding |
| **P1** | Advanced Codec Options | Low | Major quality improvement |
| **P1** | Multi-Stream Muxing | Medium | Complete transcoding |
| **P1** | Stream Copy Mode | Low | Fast remuxing |
| **P2** | Filter Graphs | High | Enables effects |
| **P2** | Hardware Acceleration | High | Performance |
| **P2** | Advanced Seeking | Low | Better navigation |
| **P2** | Metadata Handling | Low | Professional output |
| **P3** | Subtitles | Medium | Complete media support |
| **P3** | Bitstream Filters | Medium | Format compatibility |
| **P3** | Device I/O | Medium | Capture use cases |
| **P3** | Network Protocols | Low | Streaming support |
| **P4** | Image Sequences | Low | Niche use case |
| **P4** | Color Space | Medium | HDR support |

---

## Revision History

| Version | Date | Changes |
|---------|------|---------|
| 1.0.0 | 2026-01-14 | Initial comprehensive design |
