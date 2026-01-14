//go:build !ios && !android && (amd64 || arm64)

package ffgo

// EncoderPreset specifies encoding speed/quality tradeoff.
// Slower presets produce smaller files at the cost of encoding speed.
type EncoderPreset string

// Video encoder presets (x264/x265)
const (
	PresetUltrafast EncoderPreset = "ultrafast" // Fastest, largest files
	PresetSuperfast EncoderPreset = "superfast"
	PresetVeryfast  EncoderPreset = "veryfast"
	PresetFaster    EncoderPreset = "faster"
	PresetFast      EncoderPreset = "fast"
	PresetMedium    EncoderPreset = "medium" // Default
	PresetSlow      EncoderPreset = "slow"
	PresetSlower    EncoderPreset = "slower"
	PresetVeryslow  EncoderPreset = "veryslow"
	PresetPlacebo   EncoderPreset = "placebo" // Slowest, smallest files
)

// EncoderTune optimizes encoding for specific content types.
type EncoderTune string

// Video encoder tune options (x264/x265)
const (
	TuneNone        EncoderTune = ""           // No tuning
	TuneFilm        EncoderTune = "film"       // Live action content
	TuneAnimation   EncoderTune = "animation"  // Animated content
	TuneGrain       EncoderTune = "grain"      // Preserve film grain
	TuneStillimage  EncoderTune = "stillimage" // Static images
	TuneFastdecode  EncoderTune = "fastdecode" // Optimize for fast decoding
	TuneZerolatency EncoderTune = "zerolatency" // Streaming/real-time
	TunePsnr        EncoderTune = "psnr"       // Optimize for PSNR metric
	TuneSsim        EncoderTune = "ssim"       // Optimize for SSIM metric
)

// VideoProfile specifies the H.264/H.265 profile.
// Higher profiles support more features but require more decoder capability.
type VideoProfile string

// H.264 profiles
const (
	ProfileBaseline VideoProfile = "baseline" // Baseline profile (most compatible)
	ProfileMain     VideoProfile = "main"     // Main profile
	ProfileHigh     VideoProfile = "high"     // High profile (default for HD)
	ProfileHigh10   VideoProfile = "high10"   // High 10-bit profile
	ProfileHigh422  VideoProfile = "high422"  // High 4:2:2 profile
	ProfileHigh444  VideoProfile = "high444"  // High 4:4:4 profile
)

// H.265/HEVC profiles
const (
	ProfileHEVCMain   VideoProfile = "main"   // Main profile
	ProfileHEVCMain10 VideoProfile = "main10" // Main 10-bit profile
)

// VideoLevel specifies the H.264/H.265 level.
// Higher levels support higher resolution and bitrates.
type VideoLevel string

// Common H.264 levels
const (
	Level1   VideoLevel = "1"
	Level1_1 VideoLevel = "1.1"
	Level1_2 VideoLevel = "1.2"
	Level1_3 VideoLevel = "1.3"
	Level2   VideoLevel = "2"
	Level2_1 VideoLevel = "2.1"
	Level2_2 VideoLevel = "2.2"
	Level3   VideoLevel = "3"
	Level3_1 VideoLevel = "3.1"
	Level3_2 VideoLevel = "3.2"
	Level4   VideoLevel = "4"     // 1080p30
	Level4_1 VideoLevel = "4.1"   // 1080p60
	Level4_2 VideoLevel = "4.2"
	Level5   VideoLevel = "5"     // 4K30
	Level5_1 VideoLevel = "5.1"   // 4K60
	Level5_2 VideoLevel = "5.2"
	Level6   VideoLevel = "6"     // 8K30
	Level6_1 VideoLevel = "6.1"   // 8K60
	Level6_2 VideoLevel = "6.2"
)

// RateControlMode specifies how the encoder manages bitrate.
type RateControlMode int

const (
	// RateControlABR uses Average Bit Rate (target bitrate).
	// Good for streaming with bandwidth constraints.
	RateControlABR RateControlMode = iota

	// RateControlCBR uses Constant Bit Rate.
	// Good for streaming where consistent bitrate is required.
	RateControlCBR

	// RateControlCRF uses Constant Rate Factor (quality-based).
	// Good for local encoding where file size is flexible.
	// Lower CRF = higher quality, larger file.
	RateControlCRF

	// RateControlCQP uses Constant Quantization Parameter.
	// Similar to CRF but uses fixed QP values.
	RateControlCQP
)

// String returns the string representation of the rate control mode.
func (r RateControlMode) String() string {
	switch r {
	case RateControlABR:
		return "ABR"
	case RateControlCBR:
		return "CBR"
	case RateControlCRF:
		return "CRF"
	case RateControlCQP:
		return "CQP"
	default:
		return "unknown"
	}
}
