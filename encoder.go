//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"sync"
	"unsafe"

	"github.com/obinnaokechukwu/ffgo/avcodec"
	"github.com/obinnaokechukwu/ffgo/avformat"
	"github.com/obinnaokechukwu/ffgo/avutil"
	"github.com/obinnaokechukwu/ffgo/internal/bindings"
)

// Encoder encodes video and/or audio frames to a file.
type Encoder struct {
	mu sync.Mutex

	formatCtx avformat.FormatContext
	ioCtx     avformat.IOContext

	// Video encoding
	videoCodecCtx avcodec.Context
	videoStream   avformat.Stream
	videoPacket   avcodec.Packet

	// Audio encoding
	audioCodecCtx  avcodec.Context
	audioStream    avformat.Stream
	audioPacket    avcodec.Packet
	audioFrameSize int // Number of samples per frame for codec

	// Stream copy mode
	copyVideo       bool
	copyAudio       bool
	videoTimeBase   Rational // Source video time base for rescaling
	audioTimeBase   Rational // Source audio time base for rescaling
	videoStreamIdx  int      // Index of video stream for WritePacket
	audioStreamIdx  int      // Index of audio stream for WritePacket

	// Deprecated: use videoCodecCtx
	codecCtx avcodec.Context
	// Deprecated: use videoStream
	stream avformat.Stream
	// Deprecated: use videoPacket
	packet avcodec.Packet

	width       int
	height      int
	pixFmt      PixelFormat
	frameCount  int64
	timeBaseNum int32
	timeBaseDen int32

	// Audio properties
	sampleRate    int
	channels      int
	sampleFormat  SampleFormat
	audioFrameCnt int64

	headerWritten bool
	closed        bool
	hasVideo      bool
	hasAudio      bool
}

// EncoderConfig configures encoder behavior (video-only, for compatibility).
// For new code, consider using EncoderOptions with VideoEncoderConfig.
type EncoderConfig struct {
	// Width is the video width in pixels.
	Width int

	// Height is the video height in pixels.
	Height int

	// PixelFormat is the pixel format (default: PixelFormatYUV420P).
	PixelFormat PixelFormat

	// CodecID is the codec to use (default: CodecIDH264).
	CodecID CodecID

	// BitRate is the target bit rate in bits/second (default: 2000000).
	BitRate int64

	// FrameRate is the target frame rate (default: 30).
	FrameRate int

	// GOPSize is the group of pictures size (default: 12).
	GOPSize int

	// MaxBFrames is the maximum number of B-frames (default: 0).
	MaxBFrames int
}

// VideoEncoderConfig configures video encoding parameters.
type VideoEncoderConfig struct {
	// Codec specifies the video codec (default: CodecIDH264).
	Codec CodecID

	// Width is the video width in pixels.
	Width int

	// Height is the video height in pixels.
	Height int

	// FrameRate is the target frame rate (default: 30/1).
	FrameRate Rational

	// Bitrate is the target bit rate in bits/second (default: 2000000).
	// Used for ABR/CBR rate control modes.
	Bitrate int64

	// PixelFormat is the pixel format (default: PixelFormatYUV420P).
	PixelFormat PixelFormat

	// GOPSize is the group of pictures size (default: 12).
	GOPSize int

	// MaxBFrames is the maximum number of B-frames (default: 0).
	MaxBFrames int

	// Preset controls speed/quality tradeoff (e.g., PresetMedium, PresetFast).
	// Slower presets produce smaller files. Empty string uses codec default.
	Preset EncoderPreset

	// Tune optimizes for specific content types (e.g., TuneFilm, TuneAnimation).
	// Empty string uses codec default.
	Tune EncoderTune

	// Profile specifies H.264/H.265 profile (e.g., ProfileHigh, ProfileMain).
	// Higher profiles support more features. Empty string uses codec default.
	Profile VideoProfile

	// Level specifies H.264/H.265 level (e.g., Level4_1 for 1080p60).
	// Higher levels support higher resolutions. Empty string uses auto.
	Level VideoLevel

	// RateControl specifies the rate control mode (default: RateControlABR).
	RateControl RateControlMode

	// CRF is the Constant Rate Factor (0-51 for x264, 0-63 for x265).
	// Used when RateControl is RateControlCRF.
	// Lower values = higher quality, larger files. Typical: 18-28.
	CRF int

	// CQP is the Constant Quantization Parameter.
	// Used when RateControl is RateControlCQP.
	CQP int

	// MinBitrate is the minimum bitrate for VBV (bits/second).
	// Used for rate-constrained encoding.
	MinBitrate int64

	// MaxBitrate is the maximum bitrate for VBV (bits/second).
	// Used for rate-constrained encoding.
	MaxBitrate int64

	// BufferSize is the VBV buffer size (bits).
	// Controls rate variation. Larger = more variation allowed.
	BufferSize int64

	// BFrameStrategy controls B-frame placement (0-2).
	// 0=off, 1=fast, 2=best (slower).
	BFrameStrategy int

	// RefFrames is the number of reference frames (1-16).
	// More reference frames = better compression, slower encoding.
	RefFrames int

	// Threads is the number of encoding threads (default: auto).
	// 0 = auto-detect based on CPU cores.
	Threads int

	// CodecOptions allows setting arbitrary codec-specific options.
	// Keys and values are passed directly to av_opt_set.
	// Example: {"x264-params": "rc-lookahead=40"}
	CodecOptions map[string]string
}

// AudioEncoderConfig configures audio encoding parameters.
// Note: Audio encoding is not yet fully implemented.
type AudioEncoderConfig struct {
	// Codec specifies the audio codec (default: CodecIDAACj).
	Codec CodecID

	// SampleRate is the sample rate in Hz (default: 48000).
	SampleRate int

	// Channels is the number of audio channels (default: 2).
	Channels int

	// Bitrate is the target bit rate in bits/second (default: 128000).
	Bitrate int64
}

// StreamCopySource provides source codec parameters for stream copy mode.
type StreamCopySource struct {
	// VideoParams is the video codec parameters from the source stream.
	VideoParams avcodec.Parameters

	// AudioParams is the audio codec parameters from the source stream.
	AudioParams avcodec.Parameters

	// VideoTimeBase is the time base of the source video stream.
	VideoTimeBase Rational

	// AudioTimeBase is the time base of the source audio stream.
	AudioTimeBase Rational
}

// EncoderOptions configures encoder behavior with separate video and audio settings.
type EncoderOptions struct {
	// Video contains video encoding settings. Required for video output when not copying.
	Video *VideoEncoderConfig

	// Audio contains audio encoding settings. Optional.
	// Note: Audio encoding is not yet fully implemented.
	Audio *AudioEncoderConfig

	// CopyVideo enables video stream copy mode (no re-encoding).
	// When true, SourceStreams.VideoParams must be set.
	CopyVideo bool

	// CopyAudio enables audio stream copy mode (no re-encoding).
	// When true, SourceStreams.AudioParams must be set.
	CopyAudio bool

	// SourceStreams provides codec parameters from the source for stream copy.
	// Required when CopyVideo or CopyAudio is true.
	SourceStreams *StreamCopySource
}

// NewEncoder creates a new video encoder.
func NewEncoder(path string, cfg EncoderConfig) (*Encoder, error) {
	// Ensure FFmpeg is loaded
	if err := bindings.Load(); err != nil {
		return nil, err
	}

	// Apply defaults
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, errors.New("ffgo: width and height must be positive")
	}
	if cfg.PixelFormat == PixelFormatNone {
		cfg.PixelFormat = PixelFormatYUV420P
	}
	if cfg.CodecID == CodecIDNone {
		cfg.CodecID = CodecIDH264
	}
	if cfg.BitRate <= 0 {
		cfg.BitRate = 2000000
	}
	if cfg.FrameRate <= 0 {
		cfg.FrameRate = 30
	}
	if cfg.GOPSize <= 0 {
		cfg.GOPSize = 12
	}

	e := &Encoder{
		width:       cfg.Width,
		height:      cfg.Height,
		pixFmt:      cfg.PixelFormat,
		timeBaseNum: 1,
		timeBaseDen: int32(cfg.FrameRate),
		hasVideo:    true,
	}

	// Determine format from filename extension
	formatName := guessFormatFromPath(path)
	if formatName == "" {
		return nil, errors.New("ffgo: cannot determine output format from filename")
	}

	// Create output format context
	if err := avformat.AllocOutputContext2(&e.formatCtx, nil, formatName, path); err != nil {
		return nil, err
	}

	// Find encoder
	codec := avcodec.FindEncoder(cfg.CodecID)
	if codec == nil {
		e.cleanup()
		return nil, errors.New("ffgo: encoder not found")
	}

	// Create video stream
	e.videoStream = avformat.NewStream(e.formatCtx, codec)
	if e.videoStream == nil {
		e.cleanup()
		return nil, errors.New("ffgo: failed to create stream")
	}
	e.stream = e.videoStream // Backward compatibility

	// Create video codec context
	e.videoCodecCtx = avcodec.AllocContext3(codec)
	if e.videoCodecCtx == nil {
		e.cleanup()
		return nil, errors.New("ffgo: failed to allocate codec context")
	}
	e.codecCtx = e.videoCodecCtx // Backward compatibility

	// Configure codec context
	avcodec.SetCtxWidth(e.codecCtx, int32(cfg.Width))
	avcodec.SetCtxHeight(e.codecCtx, int32(cfg.Height))
	avcodec.SetCtxPixFmt(e.codecCtx, int32(cfg.PixelFormat))
	avcodec.SetCtxTimeBase(e.codecCtx, 1, int32(cfg.FrameRate))
	avcodec.SetCtxFramerate(e.codecCtx, int32(cfg.FrameRate), 1)
	avcodec.SetCtxBitRate(e.codecCtx, cfg.BitRate)
	avcodec.SetCtxGopSize(e.codecCtx, int32(cfg.GOPSize))
	avcodec.SetCtxMaxBFrames(e.codecCtx, int32(cfg.MaxBFrames))

	// Set global header flag if needed by container format
	if avformat.NeedsGlobalHeader(e.formatCtx) {
		flags := avcodec.GetCtxFlags(e.codecCtx)
		avcodec.SetCtxFlags(e.codecCtx, flags|avcodec.CodecFlagGlobalHeader)
	}

	// Open codec
	if err := avcodec.Open2(e.codecCtx, codec, nil); err != nil {
		e.cleanup()
		return nil, err
	}

	// Copy codec parameters to stream
	codecPar := avformat.GetStreamCodecPar(e.stream)
	if err := avcodec.ParametersFromContext(codecPar, e.codecCtx); err != nil {
		e.cleanup()
		return nil, err
	}

	// Set stream time base
	avformat.SetStreamTimeBase(e.stream, 1, int32(cfg.FrameRate))

	// Open output file if needed
	if !avformat.HasNoFile(e.formatCtx) {
		if err := avformat.IOOpen(&e.ioCtx, path, avformat.IOFlagWrite); err != nil {
			e.cleanup()
			return nil, err
		}
		avformat.SetIOContext(e.formatCtx, e.ioCtx)
	}

	// Allocate video packet
	e.videoPacket = avcodec.PacketAlloc()
	if e.videoPacket == nil {
		e.cleanup()
		return nil, errors.New("ffgo: failed to allocate packet")
	}
	e.packet = e.videoPacket // Backward compatibility

	return e, nil
}

// NewEncoderWithOptions creates a new encoder with separate video and audio configuration.
// This is the recommended way to create encoders in new code.
// It supports advanced codec options like presets, profiles, CRF, etc.
// For stream copy mode, set CopyVideo/CopyAudio and provide SourceStreams.
func NewEncoderWithOptions(path string, opts *EncoderOptions) (*Encoder, error) {
	if opts == nil {
		return nil, errors.New("ffgo: EncoderOptions is required")
	}

	// Validate options - must have either encoding config or stream copy
	hasVideoEncode := opts.Video != nil
	hasAudioEncode := opts.Audio != nil
	hasVideoCopy := opts.CopyVideo
	hasAudioCopy := opts.CopyAudio

	if !hasVideoEncode && !hasAudioEncode && !hasVideoCopy && !hasAudioCopy {
		return nil, errors.New("ffgo: must specify Video config, Audio config, CopyVideo, or CopyAudio")
	}

	// Validate stream copy options
	if hasVideoCopy && (opts.SourceStreams == nil || opts.SourceStreams.VideoParams == nil) {
		return nil, errors.New("ffgo: SourceStreams.VideoParams required when CopyVideo is true")
	}
	if hasAudioCopy && (opts.SourceStreams == nil || opts.SourceStreams.AudioParams == nil) {
		return nil, errors.New("ffgo: SourceStreams.AudioParams required when CopyAudio is true")
	}

	// Ensure FFmpeg is loaded
	if err := bindings.Load(); err != nil {
		return nil, err
	}

	// Handle stream copy mode
	if hasVideoCopy || hasAudioCopy {
		return newEncoderStreamCopy(path, opts)
	}

	video := opts.Video

	// Apply defaults for encoding mode
	if video.Width <= 0 || video.Height <= 0 {
		return nil, errors.New("ffgo: width and height must be positive")
	}
	pixFmt := video.PixelFormat
	if pixFmt == PixelFormatNone {
		pixFmt = PixelFormatYUV420P
	}
	codecID := video.Codec
	if codecID == CodecIDNone {
		codecID = CodecIDH264
	}
	bitrate := video.Bitrate
	if bitrate <= 0 && video.RateControl != RateControlCRF && video.RateControl != RateControlCQP {
		bitrate = 2000000
	}
	gopSize := video.GOPSize
	if gopSize <= 0 {
		gopSize = 12
	}

	// Handle frame rate
	frameRateNum := video.FrameRate.Num
	frameRateDen := video.FrameRate.Den
	if frameRateDen <= 0 {
		frameRateNum = 30
		frameRateDen = 1
	}

	e := &Encoder{
		width:       video.Width,
		height:      video.Height,
		pixFmt:      pixFmt,
		timeBaseNum: 1,
		timeBaseDen: int32(frameRateNum / frameRateDen),
		hasVideo:    true,
	}

	// Determine format from filename extension
	formatName := guessFormatFromPath(path)
	if formatName == "" {
		return nil, errors.New("ffgo: cannot determine output format from filename")
	}

	// Create output format context
	if err := avformat.AllocOutputContext2(&e.formatCtx, nil, formatName, path); err != nil {
		return nil, err
	}

	// Find encoder
	codec := avcodec.FindEncoder(codecID)
	if codec == nil {
		e.cleanup()
		return nil, errors.New("ffgo: encoder not found")
	}

	// Create video stream
	e.videoStream = avformat.NewStream(e.formatCtx, codec)
	if e.videoStream == nil {
		e.cleanup()
		return nil, errors.New("ffgo: failed to create stream")
	}
	e.stream = e.videoStream // Backward compatibility

	// Create video codec context
	e.videoCodecCtx = avcodec.AllocContext3(codec)
	if e.videoCodecCtx == nil {
		e.cleanup()
		return nil, errors.New("ffgo: failed to allocate codec context")
	}
	e.codecCtx = e.videoCodecCtx // Backward compatibility

	// Configure basic codec context parameters
	avcodec.SetCtxWidth(e.codecCtx, int32(video.Width))
	avcodec.SetCtxHeight(e.codecCtx, int32(video.Height))
	avcodec.SetCtxPixFmt(e.codecCtx, int32(pixFmt))
	avcodec.SetCtxTimeBase(e.codecCtx, 1, int32(frameRateNum/frameRateDen))
	avcodec.SetCtxFramerate(e.codecCtx, int32(frameRateNum), int32(frameRateDen))
	avcodec.SetCtxGopSize(e.codecCtx, int32(gopSize))
	avcodec.SetCtxMaxBFrames(e.codecCtx, int32(video.MaxBFrames))

	// Set bitrate for ABR/CBR modes
	if bitrate > 0 {
		avcodec.SetCtxBitRate(e.codecCtx, bitrate)
	}

	// Apply advanced codec options via av_opt_set (before opening codec)
	if err := applyVideoOptions(unsafe.Pointer(e.codecCtx), video); err != nil {
		e.cleanup()
		return nil, err
	}

	// Set global header flag if needed by container format
	if avformat.NeedsGlobalHeader(e.formatCtx) {
		flags := avcodec.GetCtxFlags(e.codecCtx)
		avcodec.SetCtxFlags(e.codecCtx, flags|avcodec.CodecFlagGlobalHeader)
	}

	// Open codec
	if err := avcodec.Open2(e.codecCtx, codec, nil); err != nil {
		e.cleanup()
		return nil, err
	}

	// Copy codec parameters to stream
	codecPar := avformat.GetStreamCodecPar(e.stream)
	if err := avcodec.ParametersFromContext(codecPar, e.codecCtx); err != nil {
		e.cleanup()
		return nil, err
	}

	// Set stream time base
	avformat.SetStreamTimeBase(e.stream, 1, int32(frameRateNum/frameRateDen))

	// Open output file if needed
	if !avformat.HasNoFile(e.formatCtx) {
		if err := avformat.IOOpen(&e.ioCtx, path, avformat.IOFlagWrite); err != nil {
			e.cleanup()
			return nil, err
		}
		avformat.SetIOContext(e.formatCtx, e.ioCtx)
	}

	// Allocate video packet
	e.videoPacket = avcodec.PacketAlloc()
	if e.videoPacket == nil {
		e.cleanup()
		return nil, errors.New("ffgo: failed to allocate packet")
	}
	e.packet = e.videoPacket // Backward compatibility

	// Setup audio if configured
	if opts.Audio != nil {
		if err := e.setupAudio(opts.Audio); err != nil {
			e.Close()
			return nil, err
		}
	}

	return e, nil
}

// newEncoderStreamCopy creates an encoder in stream copy mode.
// Packets are copied directly without decoding/encoding.
func newEncoderStreamCopy(path string, opts *EncoderOptions) (*Encoder, error) {
	// Determine format from filename extension
	formatName := guessFormatFromPath(path)
	if formatName == "" {
		return nil, errors.New("ffgo: cannot determine output format from filename")
	}

	e := &Encoder{
		copyVideo:      opts.CopyVideo,
		copyAudio:      opts.CopyAudio,
		videoStreamIdx: -1,
		audioStreamIdx: -1,
	}

	// Create output format context
	if err := avformat.AllocOutputContext2(&e.formatCtx, nil, formatName, path); err != nil {
		return nil, err
	}

	streamIdx := 0

	// Setup video stream for copy mode
	if opts.CopyVideo && opts.SourceStreams != nil && opts.SourceStreams.VideoParams != nil {
		// Create stream without codec
		stream := avformat.NewStream(e.formatCtx, nil)
		if stream == nil {
			e.cleanup()
			return nil, errors.New("ffgo: failed to create video stream for copy")
		}
		e.videoStream = stream
		e.videoStreamIdx = streamIdx
		streamIdx++

		// Copy codec parameters from source
		codecPar := avformat.GetStreamCodecPar(stream)
		if err := avcodec.ParametersCopy(codecPar, opts.SourceStreams.VideoParams); err != nil {
			e.cleanup()
			return nil, errors.New("ffgo: failed to copy video codec parameters")
		}

		// Store time base for timestamp rescaling
		e.videoTimeBase = opts.SourceStreams.VideoTimeBase
		e.hasVideo = true
	}

	// Setup audio stream for copy mode
	if opts.CopyAudio && opts.SourceStreams != nil && opts.SourceStreams.AudioParams != nil {
		// Create stream without codec
		stream := avformat.NewStream(e.formatCtx, nil)
		if stream == nil {
			e.cleanup()
			return nil, errors.New("ffgo: failed to create audio stream for copy")
		}
		e.audioStream = stream
		e.audioStreamIdx = streamIdx
		streamIdx++

		// Copy codec parameters from source
		codecPar := avformat.GetStreamCodecPar(stream)
		if err := avcodec.ParametersCopy(codecPar, opts.SourceStreams.AudioParams); err != nil {
			e.cleanup()
			return nil, errors.New("ffgo: failed to copy audio codec parameters")
		}

		// Store time base for timestamp rescaling
		e.audioTimeBase = opts.SourceStreams.AudioTimeBase
		e.hasAudio = true
	}

	// Setup audio encoding if CopyVideo but encoding audio
	if opts.CopyVideo && opts.Audio != nil && !opts.CopyAudio {
		if err := e.setupAudio(opts.Audio); err != nil {
			e.Close()
			return nil, err
		}
	}

	// Open output file if needed
	if !avformat.HasNoFile(e.formatCtx) {
		if err := avformat.IOOpen(&e.ioCtx, path, avformat.IOFlagWrite); err != nil {
			e.cleanup()
			return nil, err
		}
		avformat.SetIOContext(e.formatCtx, e.ioCtx)
	}

	// Allocate packet for WritePacket
	e.videoPacket = avcodec.PacketAlloc()
	if e.videoPacket == nil {
		e.cleanup()
		return nil, errors.New("ffgo: failed to allocate packet")
	}

	return e, nil
}

// WritePacket writes a packet directly to the output (for stream copy mode).
// The packet's stream index should match the source stream.
// For video packets, set streamIndex to match the source video stream.
// For audio packets, set streamIndex to match the source audio stream.
func (e *Encoder) WritePacket(packet *Packet) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return errors.New("ffgo: encoder is closed")
	}

	if !e.copyVideo && !e.copyAudio {
		return errors.New("ffgo: WritePacket only available in stream copy mode")
	}

	if packet == nil || packet.ptr == nil {
		return errors.New("ffgo: packet cannot be nil")
	}

	// Write header if not yet written
	if !e.headerWritten {
		if err := avformat.WriteHeader(e.formatCtx, nil); err != nil {
			return err
		}
		e.headerWritten = true
	}

	// Determine output stream index based on packet media type
	packetStreamIdx := avcodec.GetPacketStreamIndex(packet.ptr)

	var outputStreamIdx int
	var srcTimeBase, dstTimeBase Rational

	// Check if this is a video or audio packet based on context
	// In copy mode, we need to map source stream to output stream
	if e.copyVideo && e.videoStreamIdx >= 0 && packetStreamIdx == 0 {
		// Video packet
		outputStreamIdx = e.videoStreamIdx
		srcTimeBase = e.videoTimeBase
		stream := avformat.GetStream(e.formatCtx, e.videoStreamIdx)
		tbNum, tbDen := avformat.GetStreamTimeBase(stream)
		dstTimeBase = NewRational(tbNum, tbDen)
	} else if e.copyAudio && e.audioStreamIdx >= 0 {
		// Audio packet
		outputStreamIdx = e.audioStreamIdx
		srcTimeBase = e.audioTimeBase
		stream := avformat.GetStream(e.formatCtx, e.audioStreamIdx)
		tbNum, tbDen := avformat.GetStreamTimeBase(stream)
		dstTimeBase = NewRational(tbNum, tbDen)
	} else {
		return errors.New("ffgo: cannot determine output stream for packet")
	}

	// Rescale timestamps
	avcodec.RescalePacketTS(packet.ptr, srcTimeBase, dstTimeBase)

	// Set output stream index
	avcodec.SetPacketStreamIndex(packet.ptr, int32(outputStreamIdx))

	// Write packet
	return avformat.InterleavedWriteFrame(e.formatCtx, packet.ptr)
}

// applyVideoOptions applies advanced video encoding options via av_opt_set.
// This must be called BEFORE avcodec_open2.
func applyVideoOptions(ctx unsafe.Pointer, cfg *VideoEncoderConfig) error {
	if ctx == nil {
		return nil
	}

	// Preset (speed/quality tradeoff)
	if cfg.Preset != "" {
		if err := avutil.OptSet(ctx, "preset", string(cfg.Preset), avutil.AV_OPT_SEARCH_CHILDREN); err != nil {
			// Some codecs don't support preset, ignore error
			_ = err
		}
	}

	// Tune (content-specific optimization)
	if cfg.Tune != "" {
		if err := avutil.OptSet(ctx, "tune", string(cfg.Tune), avutil.AV_OPT_SEARCH_CHILDREN); err != nil {
			_ = err
		}
	}

	// Profile
	if cfg.Profile != "" {
		if err := avutil.OptSet(ctx, "profile", string(cfg.Profile), avutil.AV_OPT_SEARCH_CHILDREN); err != nil {
			_ = err
		}
	}

	// Level
	if cfg.Level != "" {
		if err := avutil.OptSet(ctx, "level", string(cfg.Level), avutil.AV_OPT_SEARCH_CHILDREN); err != nil {
			_ = err
		}
	}

	// Rate control
	switch cfg.RateControl {
	case RateControlCRF:
		if cfg.CRF > 0 {
			if err := avutil.OptSetInt(ctx, "crf", int64(cfg.CRF), avutil.AV_OPT_SEARCH_CHILDREN); err != nil {
				_ = err
			}
		}
	case RateControlCQP:
		if cfg.CQP > 0 {
			if err := avutil.OptSetInt(ctx, "qp", int64(cfg.CQP), avutil.AV_OPT_SEARCH_CHILDREN); err != nil {
				_ = err
			}
		}
	}

	// VBV buffer settings (for CBR/constrained VBR)
	if cfg.MaxBitrate > 0 {
		if err := avutil.OptSetInt(ctx, "maxrate", cfg.MaxBitrate, avutil.AV_OPT_SEARCH_CHILDREN); err != nil {
			_ = err
		}
	}
	if cfg.BufferSize > 0 {
		if err := avutil.OptSetInt(ctx, "bufsize", cfg.BufferSize, avutil.AV_OPT_SEARCH_CHILDREN); err != nil {
			_ = err
		}
	}

	// Reference frames
	if cfg.RefFrames > 0 {
		if err := avutil.OptSetInt(ctx, "refs", int64(cfg.RefFrames), avutil.AV_OPT_SEARCH_CHILDREN); err != nil {
			_ = err
		}
	}

	// Threading
	if cfg.Threads > 0 {
		if err := avutil.OptSetInt(ctx, "threads", int64(cfg.Threads), avutil.AV_OPT_SEARCH_CHILDREN); err != nil {
			_ = err
		}
	}

	// Custom codec options
	for key, value := range cfg.CodecOptions {
		if err := avutil.OptSet(ctx, key, value, avutil.AV_OPT_SEARCH_CHILDREN); err != nil {
			// Don't fail on unknown options, just skip
			_ = err
		}
	}

	return nil
}

// setupAudio adds an audio stream to the encoder.
func (e *Encoder) setupAudio(cfg *AudioEncoderConfig) error {
	// Apply defaults
	codecID := cfg.Codec
	if codecID == CodecIDNone {
		codecID = CodecIDAAC
	}
	sampleRate := cfg.SampleRate
	if sampleRate <= 0 {
		sampleRate = 48000
	}
	channels := cfg.Channels
	if channels <= 0 {
		channels = 2
	}
	bitrate := cfg.Bitrate
	if bitrate <= 0 {
		bitrate = 128000
	}

	// Find audio encoder
	audioCodec := avcodec.FindEncoder(codecID)
	if audioCodec == nil {
		return errors.New("ffgo: audio encoder not found")
	}

	// Create audio stream
	e.audioStream = avformat.NewStream(e.formatCtx, audioCodec)
	if e.audioStream == nil {
		return errors.New("ffgo: failed to create audio stream")
	}

	// Create audio codec context
	e.audioCodecCtx = avcodec.AllocContext3(audioCodec)
	if e.audioCodecCtx == nil {
		return errors.New("ffgo: failed to allocate audio codec context")
	}

	// Configure audio codec context
	avcodec.SetCtxSampleRate(e.audioCodecCtx, int32(sampleRate))
	avcodec.SetCtxChannelLayout(e.audioCodecCtx, int32(channels)) // FFmpeg 5.1+ requires ch_layout
	avcodec.SetCtxSampleFmt(e.audioCodecCtx, int32(SampleFormatFLTP)) // AAC requires FLTP
	avcodec.SetCtxBitRate(e.audioCodecCtx, bitrate)
	avcodec.SetCtxTimeBase(e.audioCodecCtx, 1, int32(sampleRate))

	// Set global header flag if needed
	if avformat.NeedsGlobalHeader(e.formatCtx) {
		flags := avcodec.GetCtxFlags(e.audioCodecCtx)
		avcodec.SetCtxFlags(e.audioCodecCtx, flags|avcodec.CodecFlagGlobalHeader)
	}

	// Open audio codec
	if err := avcodec.Open2(e.audioCodecCtx, audioCodec, nil); err != nil {
		avcodec.FreeContext(&e.audioCodecCtx)
		return err
	}

	// Copy codec parameters to stream
	codecPar := avformat.GetStreamCodecPar(e.audioStream)
	if err := avcodec.ParametersFromContext(codecPar, e.audioCodecCtx); err != nil {
		return err
	}

	// Set stream time base
	avformat.SetStreamTimeBase(e.audioStream, 1, int32(sampleRate))

	// Allocate audio packet
	e.audioPacket = avcodec.PacketAlloc()
	if e.audioPacket == nil {
		return errors.New("ffgo: failed to allocate audio packet")
	}

	// Store audio properties
	e.sampleRate = sampleRate
	e.channels = channels
	e.sampleFormat = SampleFormatFLTP
	e.hasAudio = true

	// Get frame size from codec (needed for encoding)
	e.audioFrameSize = avcodec.GetCtxFrameSize(e.audioCodecCtx)

	return nil
}

// WriteHeader writes the file header. Must be called before WriteFrame.
func (e *Encoder) WriteHeader() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return errors.New("ffgo: encoder is closed")
	}
	if e.headerWritten {
		return nil
	}

	if err := avformat.WriteHeader(e.formatCtx, nil); err != nil {
		return err
	}
	e.headerWritten = true
	return nil
}

// WriteFrame encodes and writes a frame.
// The frame must have the correct format, width, and height.
func (e *Encoder) WriteFrame(frame Frame) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return errors.New("ffgo: encoder is closed")
	}

	// Auto-write header if not done
	if !e.headerWritten {
		if err := avformat.WriteHeader(e.formatCtx, nil); err != nil {
			return err
		}
		e.headerWritten = true
	}

	// Set frame PTS
	if frame.ptr != nil {
		avutil.SetFramePTS(frame.ptr, e.frameCount)
		e.frameCount++
	}

	// Send frame to encoder
	if err := avcodec.SendFrame(e.codecCtx, frame.ptr); err != nil {
		// EAGAIN means we need to receive packets first
		if !avutil.IsAgain(err) {
			return err
		}
	}

	// Receive and write encoded packets
	for {
		avcodec.PacketUnref(e.packet)

		err := avcodec.ReceivePacket(e.codecCtx, e.packet)
		if err != nil {
			if avutil.IsAgain(err) || avutil.IsEOF(err) {
				return nil // No more packets available
			}
			return err
		}

		// Rescale timestamps
		avcodec.SetPacketStreamIndex(e.packet, avformat.GetStreamIndex(e.stream))

		// Write packet
		if err := avformat.InterleavedWriteFrame(e.formatCtx, e.packet); err != nil {
			return err
		}
	}
}

// WriteVideoFrame encodes and writes a video frame.
// This is an alias for WriteFrame for semantic clarity.
func (e *Encoder) WriteVideoFrame(frame Frame) error {
	return e.WriteFrame(frame)
}

// WriteAudioFrame encodes and writes an audio frame.
func (e *Encoder) WriteAudioFrame(frame Frame) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return errors.New("ffgo: encoder is closed")
	}
	if !e.hasAudio {
		return errors.New("ffgo: encoder was not configured with audio")
	}
	if e.audioCodecCtx == nil {
		return errors.New("ffgo: audio codec context not initialized")
	}

	// Ensure header is written
	if !e.headerWritten {
		if err := avformat.WriteHeader(e.formatCtx, nil); err != nil {
			return err
		}
		e.headerWritten = true
	}

	// Set PTS for audio frame
	if frame.ptr != nil {
		pts := e.audioFrameCnt
		avutil.SetFramePTS(frame.ptr, pts)
		e.audioFrameCnt += int64(avutil.GetFrameNbSamples(frame.ptr))
	}

	// Send frame to encoder
	if err := avcodec.SendFrame(e.audioCodecCtx, frame.ptr); err != nil {
		if avutil.IsEOF(err) {
			return nil
		}
		return err
	}

	// Receive and write packets
	for {
		avcodec.PacketUnref(e.audioPacket)

		err := avcodec.ReceivePacket(e.audioCodecCtx, e.audioPacket)
		if err != nil {
			if avutil.IsEOF(err) || avutil.IsAgain(err) {
				break
			}
			return err
		}

		// Set stream index
		avcodec.SetPacketStreamIndex(e.audioPacket, avformat.GetStreamIndex(e.audioStream))

		// Rescale timestamps to stream time base
		streamTbNum, streamTbDen := avformat.GetStreamTimeBase(e.audioStream)
		avcodec.RescalePacketTS(e.audioPacket,
			avcodec.GetCtxTimeBase(e.audioCodecCtx),
			avutil.NewRational(streamTbNum, streamTbDen))

		// Write packet
		if err := avformat.InterleavedWriteFrame(e.formatCtx, e.audioPacket); err != nil {
			return err
		}
	}

	return nil
}

// Flush flushes the encoder and writes remaining frames.
func (e *Encoder) Flush() error {
	// Send nil frame to flush encoder
	return e.WriteFrame(Frame{})
}

// Width returns the encoder width.
func (e *Encoder) Width() int {
	return e.width
}

// Height returns the encoder height.
func (e *Encoder) Height() int {
	return e.height
}

// PixelFormat returns the encoder pixel format.
func (e *Encoder) PixelFormat() PixelFormat {
	return e.pixFmt
}

// FrameCount returns the number of frames written.
func (e *Encoder) FrameCount() int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.frameCount
}

// HasAudio returns true if the encoder has audio.
func (e *Encoder) HasAudio() bool {
	return e.hasAudio
}

// HasVideo returns true if the encoder has video.
func (e *Encoder) HasVideo() bool {
	return e.hasVideo
}

// AudioFrameSize returns the number of samples per audio frame.
// This is needed when creating audio frames for encoding.
// Returns 0 if no audio is configured.
func (e *Encoder) AudioFrameSize() int {
	return e.audioFrameSize
}

// SampleRate returns the audio sample rate.
// Returns 0 if no audio is configured.
func (e *Encoder) SampleRate() int {
	return e.sampleRate
}

// Channels returns the number of audio channels.
// Returns 0 if no audio is configured.
func (e *Encoder) Channels() int {
	return e.channels
}

// SampleFormat returns the audio sample format.
// Returns SampleFormatNone if no audio is configured.
func (e *Encoder) AudioSampleFormat() SampleFormat {
	return e.sampleFormat
}

// Close finalizes and closes the encoder.
func (e *Encoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil
	}
	e.closed = true

	var firstErr error

	// Flush video encoder
	if e.videoCodecCtx != nil && e.headerWritten {
		// Flush by sending nil frame
		avcodec.SendFrame(e.videoCodecCtx, nil)

		// Drain remaining packets
		for {
			avcodec.PacketUnref(e.videoPacket)
			err := avcodec.ReceivePacket(e.videoCodecCtx, e.videoPacket)
			if err != nil {
				break
			}
			avcodec.SetPacketStreamIndex(e.videoPacket, avformat.GetStreamIndex(e.videoStream))
			avformat.InterleavedWriteFrame(e.formatCtx, e.videoPacket)
		}
	}

	// Flush audio encoder
	if e.audioCodecCtx != nil && e.headerWritten {
		// Flush by sending nil frame
		avcodec.SendFrame(e.audioCodecCtx, nil)

		// Drain remaining packets
		for {
			avcodec.PacketUnref(e.audioPacket)
			err := avcodec.ReceivePacket(e.audioCodecCtx, e.audioPacket)
			if err != nil {
				break
			}
			avcodec.SetPacketStreamIndex(e.audioPacket, avformat.GetStreamIndex(e.audioStream))
			streamTbNum, streamTbDen := avformat.GetStreamTimeBase(e.audioStream)
			avcodec.RescalePacketTS(e.audioPacket,
				avcodec.GetCtxTimeBase(e.audioCodecCtx),
				avutil.NewRational(streamTbNum, streamTbDen))
			avformat.InterleavedWriteFrame(e.formatCtx, e.audioPacket)
		}
	}

	// Write trailer
	if e.formatCtx != nil && e.headerWritten {
		if err := avformat.WriteTrailer(e.formatCtx); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	e.cleanup()
	return firstErr
}

// cleanup releases all resources.
func (e *Encoder) cleanup() {
	// Free video packet
	if e.videoPacket != nil {
		avcodec.PacketFree(&e.videoPacket)
	}
	// Also clear deprecated alias
	e.packet = nil

	// Free video codec context
	if e.videoCodecCtx != nil {
		avcodec.FreeContext(&e.videoCodecCtx)
	}
	// Also clear deprecated alias
	e.codecCtx = nil

	// Free audio packet
	if e.audioPacket != nil {
		avcodec.PacketFree(&e.audioPacket)
	}

	// Free audio codec context
	if e.audioCodecCtx != nil {
		avcodec.FreeContext(&e.audioCodecCtx)
	}

	// Close I/O context
	if e.ioCtx != nil && e.formatCtx != nil {
		avformat.IOCloseP(&e.ioCtx)
	}

	// Free format context
	if e.formatCtx != nil {
		avformat.FreeContext(e.formatCtx)
		e.formatCtx = nil
	}
}

// guessFormatFromPath determines the output format from filename extension.
func guessFormatFromPath(path string) string {
	// Check for image sequence pattern (contains %d, %04d, etc.)
	if isImageSequencePattern(path) {
		return "image2"
	}

	// Get extension
	ext := ""
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			ext = path[i+1:]
			break
		}
		if path[i] == '/' || path[i] == '\\' {
			break
		}
	}

	// Map common extensions to FFmpeg format names
	switch ext {
	case "mp4", "m4v":
		return "mp4"
	case "mkv":
		return "matroska"
	case "webm":
		return "webm"
	case "avi":
		return "avi"
	case "mov":
		return "mov"
	case "flv":
		return "flv"
	case "ts", "m2ts":
		return "mpegts"
	case "mpg", "mpeg":
		return "mpeg"
	case "ogg", "ogv":
		return "ogg"
	case "gif":
		return "gif"
	case "png", "PNG":
		return "image2"
	case "jpg", "jpeg", "JPG", "JPEG":
		return "image2"
	case "bmp", "BMP":
		return "image2"
	default:
		return ""
	}
}

// isImageSequencePattern checks if path contains printf-style format specifiers
// like %d, %04d, etc. that indicate an image sequence pattern.
func isImageSequencePattern(path string) bool {
	for i := 0; i < len(path)-1; i++ {
		if path[i] == '%' {
			// Check if followed by digits and 'd'
			j := i + 1
			// Skip width specifier (e.g., "04" in "%04d")
			for j < len(path) && path[j] >= '0' && path[j] <= '9' {
				j++
			}
			// Must end with 'd'
			if j < len(path) && path[j] == 'd' {
				return true
			}
		}
	}
	return false
}
