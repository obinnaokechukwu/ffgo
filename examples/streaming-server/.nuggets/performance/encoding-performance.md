---
title: Encoding Performance Optimization
keywords: [encoding, performance, gpu, tuning, benchmarking]
tags: [guide, internal, advanced]
related: [../transcoding/adaptive-bitrate.md, ./memory-efficiency.md, ../gotchas/encoder-bottlenecks.md]
source: [server/performance/encoding.go, server/transcoding/variant_manager.go]
---

# Encoding Performance Optimization

Encoding is the CPU/GPU bottleneck in transcoding pipelines. A single stream in real-time requires 100% of one CPU core. Streaming 10 concurrent 720p streams exhausts an 8-core server. Strategic optimization is essential.

## Performance Tiers

Different codecs have vastly different encoding speeds:

```
Codec         Speed (FPS)  Quality/Bitrate        Use Case
─────────────────────────────────────────────────────────────
H.264 (fast)  ~100 FPS     Baseline             Live streaming
H.264 (slow)  ~20 FPS      High quality         VOD
H.265 (fast)  ~60 FPS      40% bitrate savings  Live streaming
H.265 (slow)  ~10 FPS      Premium quality      VOD
VP9 (fast)    ~40 FPS      High quality         WebRTC
AV1           ~5 FPS       Premium quality      Offline processing
```

**Real-time streaming requires:** Input FPS ≤ Encoding FPS

## Preset Selection

Most codecs support quality/speed presets:

```go
// H.264 presets (ffmpeg -preset)
type H264Preset string

const (
    H264Fast   H264Preset = "fast"      // ~100 FPS, lower quality
    H264Medium H264Preset = "medium"    // ~70 FPS, balanced
    H264Slow   H264Preset = "slow"      // ~30 FPS, high quality
    H264Slower H264Preset = "slower"    // ~15 FPS, very high quality
)

// CPU core usage per preset
//   fast:    50% of one core
//   medium:  80% of one core
//   slow:   120% (uses 1.2 cores)
//   slower: 200% (uses 2 cores)
```

**Guidance:**

```go
// For real-time streaming: choose preset based on available CPU
func selectPreset(videoBitrate int64, numConcurrentStreams int, cpuCores int) string {
    // Estimate CPU usage
    cpuNeeded := numConcurrentStreams * int(videoBitrate) / 1_000_000

    if cpuNeeded < cpuCores/2 {
        return "medium"  // Good quality, safe
    } else if cpuNeeded < cpuCores {
        return "fast"    // Keep up with real-time
    } else {
        return "veryfast"  // Fallback to maintain real-time
    }
}

// Example: 3x 2.5Mbps streams on 4-core CPU
// CPU: 3 * 2.5 = 7.5 core-units → use "fast"
```

## Quality Settings (CRF)

Constant Rate Factor (CRF) provides quality-based encoding instead of bitrate targets:

```go
// H.264 CRF range: 0-51 (lower = better quality)
// H.265 CRF range: 0-51 (same scale as H.264)

type CRFQuality int

const (
    CRFLossless  CRFQuality = 0      // Lossless, huge bitrate
    CRFHighQual  CRFQuality = 18     // Visually lossless
    CRFHighQual2 CRFQuality = 20
    CRFHighQual3 CRFQuality = 22     // High quality
    CRFBalanced  CRFQuality = 28     // Good quality, reasonable size
    CRFLowQual   CRFQuality = 35     // Lower quality, small size
    CRFVeryLow   CRFQuality = 45     // Very low quality
)

// Bitrate impact per CRF point
// Changing CRF by 6 roughly halves/doubles bitrate
// CRF 18: ~8 Mbps for 1080p
// CRF 24: ~4 Mbps for 1080p
// CRF 30: ~2 Mbps for 1080p
```

**CRF vs Bitrate:**

```go
// Option 1: Fixed bitrate (VBR)
encoder.AddVideoStream(ffgo.VideoEncoderConfig{
    BitRate: 5_000_000,  // Always target 5 Mbps
})

// Option 2: Quality-based (CRF)
encoder.AddVideoStream(ffgo.VideoEncoderConfig{
    CRF: 28,  // Variable bitrate, target quality
})
```

CRF is preferred for live streaming because bitrate naturally varies with scene complexity. Fixed bitrate encoding wastes bits on simple scenes and underestimates complex ones.

## GPU Acceleration

GPU encoding (NVENC, VAAPI, VideoToolbox) is 5-10x faster than CPU:

```go
// Check available GPUs
var availableDevices []string

func DetectGPUs() []string {
    devices := []string{}

    // NVIDIA CUDA
    if canLoadCUDA() {
        for i := 0; i < getCUDADeviceCount(); i++ {
            devices = append(devices, fmt.Sprintf("cuda:%d", i))
        }
    }

    // AMD/Intel VAAPI (Linux)
    if canLoadVAAPI() {
        devices = append(devices, "vaapi")
    }

    // Apple VideoToolbox
    if isRunningOnMacOS() {
        devices = append(devices, "videotoolbox")
    }

    return devices
}

// Per-variant GPU assignment
func (abr *ABRTranscoder) createVariantEncoder(vp VariantProfile) (*VariantEncoder, error) {
    hwDevice := ""

    // Assign GPUs in round-robin
    if len(abr.availableGPUs) > 0 {
        hwDevice = abr.availableGPUs[abr.gpuIndex%len(abr.availableGPUs)]
        abr.gpuIndex++
    }

    config := ffgo.VideoEncoderConfig{
        Codec:    vp.Codec,
        HWDevice: hwDevice,  // "cuda:0", "cuda:1", "vaapi", etc.
        // ... other settings
    }

    return ffgo.NewEncoder(...).AddVideoStream(config)
}
```

**GPU Performance Comparison:**

```
GPU                Speed        Memory    Cost
─────────────────────────────────────────────────
NVIDIA RTX 3090    ~800 FPS    24GB      $1,500
NVIDIA RTX 3080    ~600 FPS    10GB      $700
NVIDIA RTX 4000    ~400 FPS    24GB      $5,000 (datacenter)
Intel Arc A770     ~300 FPS    8GB       $329
AMD RDNA2          ~400 FPS    16GB      $700+

Encoding 10x 1080p 30fps streams:
  CPU (8-core):   Impossible (requires 100+ cores)
  Single RTX 3090: Trivial (can do 50+ streams)
```

## Multi-Pass Encoding

Some use cases allow offline encoding in multiple passes for optimal quality:

```go
type MultiPassEncoder struct {
    sourceFile string
    outputFile string

    // Pass 1: Analysis
    stats StatsCollector

    // Pass 2: Final encoding
    encodedFile string
}

func (mpe *MultiPassEncoder) EncodeMultiPass() error {
    // Pass 1: Analyze source to estimate bitrate curve
    log.Println("Pass 1: Analyzing source...")
    if err := mpe.analyzeSource(); err != nil {
        return err
    }

    // Pass 2: Encode with optimal settings
    log.Println("Pass 2: Encoding with optimized settings...")
    if err := mpe.encodeWithOptimization(); err != nil {
        return err
    }

    return nil
}
```

**Trade-off:** 2x slower, significantly better quality. Only viable for VOD, not live.

## Benchmark and Profiling

Profile encoding to identify bottlenecks:

```go
import "time"

type EncodingStats struct {
    framesProcessed  int64
    totalTime        time.Duration
    peakFrameTime    time.Duration
    avgFrameTime     time.Duration
    gpuUtilization   float64
}

func (es *EncodingStats) TrackFrame(encoder *ffgo.Encoder, frame ffgo.Frame) error {
    start := time.Now()

    if err := encoder.EncodeFrame(frame); err != nil {
        return err
    }

    elapsed := time.Since(start)
    es.totalTime += elapsed
    es.framesProcessed++

    if elapsed > es.peakFrameTime {
        es.peakFrameTime = elapsed
    }

    es.avgFrameTime = es.totalTime / time.Duration(es.framesProcessed)

    // Log spikes
    if elapsed > es.avgFrameTime*2 {
        log.Warnf("Slow frame encode: %v (avg: %v)", elapsed, es.avgFrameTime)
    }

    return nil
}

func (es *EncodingStats) Report() {
    fps := float64(es.framesProcessed) / es.totalTime.Seconds()
    log.Printf("Encoding Stats: %d frames in %v (%.0f FPS avg)",
        es.framesProcessed,
        es.totalTime,
        fps,
    )
    log.Printf("Frame time: avg=%v peak=%v", es.avgFrameTime, es.peakFrameTime)
}
```

## Adaptive Quality

In overload conditions, reduce quality dynamically:

```go
type AdaptiveQuality struct {
    targetFrameRate  int
    measuredFrameRate float64
    currentCRF       int

    coresCPU int
}

func (aq *AdaptiveQuality) Adjust() {
    if aq.measuredFrameRate < float64(aq.targetFrameRate)*0.95 {
        // Falling behind, reduce quality
        aq.currentCRF = aq.Min(51, aq.currentCRF+2)
        log.Printf("Quality reduction: CRF=%d (FPS: %.1f)", aq.currentCRF, aq.measuredFrameRate)
    } else if aq.measuredFrameRate > float64(aq.targetFrameRate)*1.1 {
        // Ahead of schedule, improve quality
        if aq.currentCRF > 20 {
            aq.currentCRF--
            log.Printf("Quality improvement: CRF=%d (FPS: %.1f)", aq.currentCRF, aq.measuredFrameRate)
        }
    }
}

// In encoding loop
if time.Since(lastAdjust) > 10*time.Second {
    aq.Adjust()
    lastAdjust = time.Now()
}
```

## Resource Limits

Prevent encoding from consuming all system resources:

```go
type EncodingResourceLimit struct {
    maxConcurrentEncoders int
    maxConcurrentGPU      int
    cpuLimitPercent       int
}

func (erl *EncodingResourceLimit) CanStartEncoding() bool {
    // Check concurrent limit
    if atomic.LoadInt32(&erl.activeEncoders) >= int32(erl.maxConcurrentEncoders) {
        return false
    }

    // Check CPU usage
    cpuUsage := getCPUUsagePercent()
    if cpuUsage > erl.cpuLimitPercent {
        return false
    }

    return true
}

func (erl *EncodingResourceLimit) StartEncoding() {
    if !erl.CanStartEncoding() {
        // Queue for later
        erl.pendingQueue.Push(...)
    }
}
```

See [adaptive bitrate](../transcoding/adaptive-bitrate.md) for variant-level optimization and [encoder bottlenecks](../gotchas/encoder-bottlenecks.md) for common performance issues.
