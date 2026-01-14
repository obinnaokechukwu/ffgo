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

// Encoder encodes video frames to a file.
type Encoder struct {
	mu sync.Mutex

	formatCtx avformat.FormatContext
	codecCtx  avcodec.Context
	stream    avformat.Stream
	packet    avcodec.Packet
	ioCtx     avformat.IOContext

	width       int
	height      int
	pixFmt      PixelFormat
	frameCount  int64
	timeBaseNum int32
	timeBaseDen int32

	headerWritten bool
	closed        bool
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

	// Create stream
	e.stream = avformat.NewStream(e.formatCtx, codec)
	if e.stream == nil {
		e.cleanup()
		return nil, errors.New("ffgo: failed to create stream")
	}

	// Create codec context
	e.codecCtx = avcodec.AllocContext3(codec)
	if e.codecCtx == nil {
		e.cleanup()
		return nil, errors.New("ffgo: failed to allocate codec context")
	}

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

	// Allocate packet
	e.packet = avcodec.PacketAlloc()
	if e.packet == nil {
		e.cleanup()
		return nil, errors.New("ffgo: failed to allocate packet")
	}

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

	return NewEncoder(path, cfg)
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
// Note: Audio encoding is not yet fully implemented. This function is provided
// for API compatibility and will return an error if called.
func (e *Encoder) WriteAudioFrame(frame Frame) error {
	return errors.New("ffgo: audio encoding not yet implemented")
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

// Close finalizes and closes the encoder.
func (e *Encoder) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.closed {
		return nil
	}
	e.closed = true

	var firstErr error

	// Flush encoder
	if e.codecCtx != nil && e.headerWritten {
		// Flush by sending nil frame
		avcodec.SendFrame(e.codecCtx, nil)

		// Drain remaining packets
		for {
			avcodec.PacketUnref(e.packet)
			err := avcodec.ReceivePacket(e.codecCtx, e.packet)
			if err != nil {
				break
			}
			avcodec.SetPacketStreamIndex(e.packet, avformat.GetStreamIndex(e.stream))
			avformat.InterleavedWriteFrame(e.formatCtx, e.packet)
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
	// Free packet
	if e.packet != nil {
		avcodec.PacketFree(&e.packet)
	}

	// Free codec context
	if e.codecCtx != nil {
		avcodec.FreeContext(&e.codecCtx)
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
