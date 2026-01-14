//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"sync"

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
	Bitrate int64

	// PixelFormat is the pixel format (default: PixelFormatYUV420P).
	PixelFormat PixelFormat

	// GOPSize is the group of pictures size (default: 12).
	GOPSize int

	// MaxBFrames is the maximum number of B-frames (default: 0).
	MaxBFrames int
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

// EncoderOptions configures encoder behavior with separate video and audio settings.
type EncoderOptions struct {
	// Video contains video encoding settings. Required for video output.
	Video *VideoEncoderConfig

	// Audio contains audio encoding settings. Optional.
	// Note: Audio encoding is not yet fully implemented.
	Audio *AudioEncoderConfig
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
func NewEncoderWithOptions(path string, opts *EncoderOptions) (*Encoder, error) {
	if opts == nil || opts.Video == nil {
		return nil, errors.New("ffgo: EncoderOptions.Video is required")
	}

	video := opts.Video

	// Convert VideoEncoderConfig to EncoderConfig
	cfg := EncoderConfig{
		Width:       video.Width,
		Height:      video.Height,
		PixelFormat: video.PixelFormat,
		CodecID:     video.Codec,
		BitRate:     video.Bitrate,
		GOPSize:     video.GOPSize,
		MaxBFrames:  video.MaxBFrames,
	}

	// Handle frame rate
	if video.FrameRate.Den > 0 {
		cfg.FrameRate = int(video.FrameRate.Num / video.FrameRate.Den)
	}
	if cfg.FrameRate <= 0 {
		cfg.FrameRate = 30
	}

	// Apply defaults from VideoEncoderConfig if not set in EncoderConfig
	if cfg.CodecID == CodecIDNone {
		cfg.CodecID = CodecIDH264
	}
	if cfg.BitRate <= 0 {
		cfg.BitRate = 2000000
	}

	// Create video encoder
	enc, err := NewEncoder(path, cfg)
	if err != nil {
		return nil, err
	}

	// Setup audio if configured
	if opts.Audio != nil {
		if err := enc.setupAudio(opts.Audio); err != nil {
			enc.Close()
			return nil, err
		}
	}

	return enc, nil
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
	if frame != nil {
		avutil.SetFramePTS(frame, e.frameCount)
		e.frameCount++
	}

	// Send frame to encoder
	if err := avcodec.SendFrame(e.codecCtx, frame); err != nil {
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
	if frame != nil {
		pts := e.audioFrameCnt
		avutil.SetFramePTS(frame, pts)
		e.audioFrameCnt += int64(avutil.GetFrameNbSamples(frame))
	}

	// Send frame to encoder
	if err := avcodec.SendFrame(e.audioCodecCtx, frame); err != nil {
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
	return e.WriteFrame(nil)
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
	default:
		return ""
	}
}
