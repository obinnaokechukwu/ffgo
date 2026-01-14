//go:build !ios && !android && (amd64 || arm64)

// Package ffgo provides high-level bindings to FFmpeg for media processing.
// It enables decoding, encoding, muxing, demuxing, and scaling of audio/video
// without CGO using purego.
//
// For most use cases, use the high-level types: Decoder, Encoder, and Scaler.
// For advanced use cases, the low-level packages (avutil, avcodec, avformat, swscale)
// are available.
package ffgo

import (
	"github.com/obinnaokechukwu/ffgo/avcodec"
	"github.com/obinnaokechukwu/ffgo/avformat"
	"github.com/obinnaokechukwu/ffgo/avutil"
	"github.com/obinnaokechukwu/ffgo/internal/bindings"
)

// Init initializes FFmpeg libraries. This is called automatically when using
// the high-level API, but can be called explicitly to check for errors.
// It is safe to call multiple times.
func Init() error {
	return bindings.Load()
}

// IsLoaded returns true if FFmpeg libraries have been successfully loaded.
func IsLoaded() bool {
	return bindings.IsLoaded()
}

// Version returns FFmpeg library versions.
func Version() (avutil, avcodec, avformat uint32) {
	return bindings.AVUtilVersion(), bindings.AVCodecVersion(), bindings.AVFormatVersion()
}

// Re-export common types for convenience
type (
	// Frame is a decoded video or audio frame.
	Frame = avutil.Frame

	// Packet is an encoded packet of data.
	Packet = avcodec.Packet

	// Rational represents a rational number (fraction).
	Rational = avutil.Rational

	// PixelFormat represents video pixel formats.
	PixelFormat = avutil.PixelFormat

	// SampleFormat represents audio sample formats.
	SampleFormat = avutil.SampleFormat

	// MediaType represents stream types (video, audio, etc.).
	MediaType = avutil.MediaType

	// CodecID represents codec identifiers.
	CodecID = avcodec.CodecID
)

// Re-export common constants
const (
	// Pixel formats
	PixelFormatNone     = avutil.PixelFormatNone
	PixelFormatYUV420P  = avutil.PixelFormatYUV420P
	PixelFormatYUVJ420P = avutil.PixelFormatYUVJ420P // Full-range YUV 4:2:0 (JPEG)
	PixelFormatRGB24    = avutil.PixelFormatRGB24
	PixelFormatBGR24    = avutil.PixelFormatBGR24
	PixelFormatRGBA     = avutil.PixelFormatRGBA
	PixelFormatBGRA     = avutil.PixelFormatBGRA
	PixelFormatNV12     = avutil.PixelFormatNV12

	// Media types
	MediaTypeUnknown  = avutil.MediaTypeUnknown
	MediaTypeVideo    = avutil.MediaTypeVideo
	MediaTypeAudio    = avutil.MediaTypeAudio
	MediaTypeSubtitle = avutil.MediaTypeSubtitle

	// Common codec IDs
	CodecIDNone  = avcodec.CodecIDNone
	CodecIDH264  = avcodec.CodecIDH264
	CodecIDHEVC  = avcodec.CodecIDHEVC
	CodecIDAV1   = avcodec.CodecIDAV1
	CodecIDVP8   = avcodec.CodecIDVP8
	CodecIDVP9   = avcodec.CodecIDVP9
	CodecIDAAC   = avcodec.CodecIDAAC
	CodecIDMP3   = avcodec.CodecIDMP3
	CodecIDOPUS  = avcodec.CodecIDOPUS
	CodecIDMJPEG = avcodec.CodecIDMJPEG
	CodecIDPNG   = avcodec.CodecIDPNG
	CodecIDBMP   = avcodec.CodecIDBMP
	CodecIDGIF   = avcodec.CodecIDGIF

	// Codec aliases (shorter names for convenience, as shown in user-guide)
	CodecH264 = CodecIDH264
	CodecHEVC = CodecIDHEVC
	CodecAV1  = CodecIDAV1
	CodecVP8  = CodecIDVP8
	CodecVP9  = CodecIDVP9
	CodecAAC  = CodecIDAAC
	CodecMP3  = CodecIDMP3
	CodecOpus = CodecIDOPUS

	// Sample formats
	SampleFormatNone = avutil.SampleFormatNone
	SampleFormatU8   = avutil.SampleFormatU8
	SampleFormatS16  = avutil.SampleFormatS16
	SampleFormatS32  = avutil.SampleFormatS32
	SampleFormatFlt  = avutil.SampleFormatFlt
	SampleFormatDbl  = avutil.SampleFormatDbl
	SampleFormatU8P  = avutil.SampleFormatU8P
	SampleFormatS16P = avutil.SampleFormatS16P
	SampleFormatS32P = avutil.SampleFormatS32P
	SampleFormatFLTP = avutil.SampleFormatFltP // Float 32-bit planar (AAC default)
	SampleFormatFltP = avutil.SampleFormatFltP // Alias
	SampleFormatDblP = avutil.SampleFormatDblP
	SampleFormatS64  = avutil.SampleFormatS64
	SampleFormatS64P = avutil.SampleFormatS64P
)

// StreamInfo contains information about a media stream.
type StreamInfo struct {
	Index      int
	Type       MediaType
	CodecID    CodecID
	CodecName  string
	Width      int         // Video only
	Height     int         // Video only
	PixelFmt   PixelFormat // Video only
	FrameRate  Rational    // Video only - frames per second
	SampleRate int         // Audio only
	Channels   int         // Audio only
	TimeBase   Rational
	Duration   int64 // In time_base units
	BitRate    int64
}

// FrameInfo contains information about a decoded frame.
type FrameInfo struct {
	Width     int
	Height    int
	Format    int32
	PTS       int64
	KeyFrame  bool
	MediaType MediaType
}

// GetFrameInfo returns information about a frame.
func GetFrameInfo(frame Frame) FrameInfo {
	return FrameInfo{
		Width:  int(avutil.GetFrameWidth(frame)),
		Height: int(avutil.GetFrameHeight(frame)),
		Format: avutil.GetFrameFormat(frame),
		PTS:    avutil.GetFramePTS(frame),
	}
}

// NewRational creates a new rational number.
func NewRational(num, den int32) Rational {
	return avutil.NewRational(num, den)
}

// FrameAlloc allocates a new frame.
func FrameAlloc() Frame {
	return avutil.FrameAlloc()
}

// FrameFree frees a frame.
func FrameFree(frame *Frame) {
	avutil.FrameFree(frame)
}

// FrameRef creates a reference to src in dst.
func FrameRef(dst, src Frame) error {
	return avutil.FrameRef(dst, src)
}

// FrameUnref unreferences a frame's buffers.
func FrameUnref(frame Frame) {
	avutil.FrameUnref(frame)
}

// Error helpers

// IsEOF returns true if the error indicates end of file.
func IsEOF(err error) bool {
	return avutil.IsEOF(err)
}

// IsAgain returns true if the error indicates to try again (EAGAIN).
func IsAgain(err error) bool {
	return avutil.IsAgain(err)
}

// Low-level package access (for advanced users)
var (
	// AVUtil provides access to avutil package functions.
	AVUtil = struct {
		FrameAlloc        func() Frame
		FrameFree         func(frame *Frame)
		FrameRef          func(dst, src Frame) error
		FrameUnref        func(frame Frame)
		FrameGetBuffer    func(frame Frame, align int32) error
		FrameMakeWritable func(frame Frame) error
		GetFrameWidth     func(frame Frame) int32
		GetFrameHeight    func(frame Frame) int32
		GetFrameFormat    func(frame Frame) int32
		SetFrameWidth     func(frame Frame, width int32)
		SetFrameHeight    func(frame Frame, height int32)
		SetFrameFormat    func(frame Frame, format int32)
	}{
		FrameAlloc:        avutil.FrameAlloc,
		FrameFree:         avutil.FrameFree,
		FrameRef:          avutil.FrameRef,
		FrameUnref:        avutil.FrameUnref,
		FrameGetBuffer:    avutil.FrameGetBufferErr,
		FrameMakeWritable: avutil.FrameMakeWritable,
		GetFrameWidth:     avutil.GetFrameWidth,
		GetFrameHeight:    avutil.GetFrameHeight,
		GetFrameFormat:    avutil.GetFrameFormat,
		SetFrameWidth:     avutil.SetFrameWidth,
		SetFrameHeight:    avutil.SetFrameHeight,
		SetFrameFormat:    avutil.SetFrameFormat,
	}

	// AVFormat provides access to avformat package functions.
	AVFormat = struct {
		OpenInput       func(ctx *avformat.FormatContext, url string, fmt avformat.InputFormat, options *avutil.Dictionary) error
		CloseInput      func(ctx *avformat.FormatContext)
		FindStreamInfo  func(ctx avformat.FormatContext, options *avutil.Dictionary) error
		ReadFrame       func(ctx avformat.FormatContext, pkt Packet) error
		FindBestStream  func(ctx avformat.FormatContext, mediaType MediaType, wanted, related int32, decoder *avcodec.Codec, flags int32) int32
		GetNumStreams   func(ctx avformat.FormatContext) int
		GetStream       func(ctx avformat.FormatContext, index int) avformat.Stream
		GetStreamCodecPar func(stream avformat.Stream) avcodec.Parameters
	}{
		OpenInput:       avformat.OpenInput,
		CloseInput:      avformat.CloseInput,
		FindStreamInfo:  avformat.FindStreamInfo,
		ReadFrame:       avformat.ReadFrame,
		FindBestStream:  avformat.FindBestStream,
		GetNumStreams:   avformat.GetNumStreams,
		GetStream:       avformat.GetStream,
		GetStreamCodecPar: avformat.GetStreamCodecPar,
	}
)
