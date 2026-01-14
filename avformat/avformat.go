//go:build !ios && !android && (amd64 || arm64)

// Package avformat provides bindings to FFmpeg's libavformat library.
// It includes container format handling, demuxing, muxing, and I/O operations.
package avformat

import (
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/obinnaokechukwu/ffgo/avcodec"
	"github.com/obinnaokechukwu/ffgo/avutil"
	"github.com/obinnaokechukwu/ffgo/internal/bindings"
)

// FormatContext is an opaque FFmpeg AVFormatContext pointer.
type FormatContext = unsafe.Pointer

// InputFormat is an opaque FFmpeg AVInputFormat pointer.
type InputFormat = unsafe.Pointer

// OutputFormat is an opaque FFmpeg AVOutputFormat pointer.
type OutputFormat = unsafe.Pointer

// Stream is an opaque FFmpeg AVStream pointer.
type Stream = unsafe.Pointer

// IOContext is an opaque FFmpeg AVIOContext pointer.
type IOContext = unsafe.Pointer

// MediaType aliases for convenience
const (
	MediaTypeUnknown    = avutil.MediaTypeUnknown
	MediaTypeVideo      = avutil.MediaTypeVideo
	MediaTypeAudio      = avutil.MediaTypeAudio
	MediaTypeData       = avutil.MediaTypeData
	MediaTypeSubtitle   = avutil.MediaTypeSubtitle
	MediaTypeAttachment = avutil.MediaTypeAttachment
)

// Function bindings
var (
	avformatOpenInput          func(ctx *unsafe.Pointer, url string, fmt, options unsafe.Pointer) int32
	avformatCloseInput         func(ctx *unsafe.Pointer)
	avformatFindStreamInfo     func(ctx unsafe.Pointer, options *unsafe.Pointer) int32
	avformatAllocContext       func() unsafe.Pointer
	avformatFreeContext        func(ctx unsafe.Pointer)
	avformatAllocOutputCtx2    func(ctx *unsafe.Pointer, oformat unsafe.Pointer, formatName, filename string) int32
	avformatNewStream          func(ctx, codec unsafe.Pointer) unsafe.Pointer
	avformatWriteHeader        func(ctx unsafe.Pointer, options *unsafe.Pointer) int32
	avWriteTrailer             func(ctx unsafe.Pointer) int32

	avReadFrame              func(ctx, pkt unsafe.Pointer) int32
	avWriteFrame             func(ctx, pkt unsafe.Pointer) int32
	avInterleavedWriteFrame  func(ctx, pkt unsafe.Pointer) int32
	avSeekFrame              func(ctx unsafe.Pointer, streamIndex int32, timestamp int64, flags int32) int32

	avFindBestStream func(ctx unsafe.Pointer, mediaType, wanted, related int32, decoder *unsafe.Pointer, flags int32) int32

	avioOpen     func(ctx *unsafe.Pointer, url string, flags int32) int32
	avioClose    func(ctx unsafe.Pointer) int32
	avioClosep   func(ctx *unsafe.Pointer) int32

	// Packet functions (in avcodec but often used with avformat)
	avPacketAlloc func() unsafe.Pointer
	avPacketFree  func(pkt *unsafe.Pointer)
	avPacketUnref func(pkt unsafe.Pointer)

	bindingsRegistered bool
)

func init() {
	registerBindings()
}

func registerBindings() {
	if bindingsRegistered {
		return
	}

	if err := bindings.Load(); err != nil {
		return
	}

	lib := bindings.LibAVFormat()
	if lib == 0 {
		return
	}

	libCodec := bindings.LibAVCodec()

	purego.RegisterLibFunc(&avformatOpenInput, lib, "avformat_open_input")
	purego.RegisterLibFunc(&avformatCloseInput, lib, "avformat_close_input")
	purego.RegisterLibFunc(&avformatFindStreamInfo, lib, "avformat_find_stream_info")
	purego.RegisterLibFunc(&avformatAllocContext, lib, "avformat_alloc_context")
	purego.RegisterLibFunc(&avformatFreeContext, lib, "avformat_free_context")
	purego.RegisterLibFunc(&avformatAllocOutputCtx2, lib, "avformat_alloc_output_context2")
	purego.RegisterLibFunc(&avformatNewStream, lib, "avformat_new_stream")
	purego.RegisterLibFunc(&avformatWriteHeader, lib, "avformat_write_header")
	purego.RegisterLibFunc(&avWriteTrailer, lib, "av_write_trailer")

	purego.RegisterLibFunc(&avReadFrame, lib, "av_read_frame")
	purego.RegisterLibFunc(&avWriteFrame, lib, "av_write_frame")
	purego.RegisterLibFunc(&avInterleavedWriteFrame, lib, "av_interleaved_write_frame")
	purego.RegisterLibFunc(&avSeekFrame, lib, "av_seek_frame")

	purego.RegisterLibFunc(&avFindBestStream, lib, "av_find_best_stream")

	purego.RegisterLibFunc(&avioOpen, lib, "avio_open")
	purego.RegisterLibFunc(&avioClose, lib, "avio_close")
	purego.RegisterLibFunc(&avioClosep, lib, "avio_closep")

	// Packet functions from avcodec
	if libCodec != 0 {
		purego.RegisterLibFunc(&avPacketAlloc, libCodec, "av_packet_alloc")
		purego.RegisterLibFunc(&avPacketFree, libCodec, "av_packet_free")
		purego.RegisterLibFunc(&avPacketUnref, libCodec, "av_packet_unref")
	}

	bindingsRegistered = true
}

// AllocContext allocates an AVFormatContext.
func AllocContext() FormatContext {
	if avformatAllocContext == nil {
		return nil
	}
	return avformatAllocContext()
}

// FreeContext frees an AVFormatContext.
func FreeContext(ctx FormatContext) {
	if ctx == nil || avformatFreeContext == nil {
		return
	}
	avformatFreeContext(ctx)
}

// OpenInput opens an input file.
func OpenInput(ctx *FormatContext, url string, fmt InputFormat, options *avutil.Dictionary) error {
	if avformatOpenInput == nil {
		return bindings.ErrNotLoaded
	}
	var opts unsafe.Pointer
	if options != nil {
		opts = *options
	}
	ret := avformatOpenInput(ctx, url, fmt, opts)
	runtime.KeepAlive(url)
	if ret < 0 {
		return avutil.NewError(ret, "avformat_open_input")
	}
	return nil
}

// CloseInput closes an input file and frees the context.
func CloseInput(ctx *FormatContext) {
	if ctx == nil || *ctx == nil || avformatCloseInput == nil {
		return
	}
	avformatCloseInput(ctx)
	*ctx = nil
}

// FindStreamInfo reads packets to get stream info.
func FindStreamInfo(ctx FormatContext, options *avutil.Dictionary) error {
	if avformatFindStreamInfo == nil {
		return bindings.ErrNotLoaded
	}
	ret := avformatFindStreamInfo(ctx, options)
	if ret < 0 {
		return avutil.NewError(ret, "avformat_find_stream_info")
	}
	return nil
}

// AllocOutputContext2 allocates an output context.
func AllocOutputContext2(ctx *FormatContext, oformat OutputFormat, formatName, filename string) error {
	if avformatAllocOutputCtx2 == nil {
		return bindings.ErrNotLoaded
	}
	ret := avformatAllocOutputCtx2(ctx, oformat, formatName, filename)
	runtime.KeepAlive(formatName)
	runtime.KeepAlive(filename)
	if ret < 0 {
		return avutil.NewError(ret, "avformat_alloc_output_context2")
	}
	return nil
}

// NewStream creates a new stream in the format context.
func NewStream(ctx FormatContext, codec avcodec.Codec) Stream {
	if avformatNewStream == nil {
		return nil
	}
	return avformatNewStream(ctx, codec)
}

// WriteHeader writes the file header.
func WriteHeader(ctx FormatContext, options *avutil.Dictionary) error {
	if avformatWriteHeader == nil {
		return bindings.ErrNotLoaded
	}
	ret := avformatWriteHeader(ctx, options)
	if ret < 0 {
		return avutil.NewError(ret, "avformat_write_header")
	}
	return nil
}

// WriteTrailer writes the file trailer.
func WriteTrailer(ctx FormatContext) error {
	if avWriteTrailer == nil {
		return bindings.ErrNotLoaded
	}
	ret := avWriteTrailer(ctx)
	if ret < 0 {
		return avutil.NewError(ret, "av_write_trailer")
	}
	return nil
}

// ReadFrame reads the next frame of a stream.
func ReadFrame(ctx FormatContext, pkt avcodec.Packet) error {
	if avReadFrame == nil {
		return bindings.ErrNotLoaded
	}
	ret := avReadFrame(ctx, pkt)
	if ret < 0 {
		return avutil.NewError(ret, "av_read_frame")
	}
	return nil
}

// WriteFrame writes a packet to the output file.
func WriteFrame(ctx FormatContext, pkt avcodec.Packet) error {
	if avWriteFrame == nil {
		return bindings.ErrNotLoaded
	}
	ret := avWriteFrame(ctx, pkt)
	if ret < 0 {
		return avutil.NewError(ret, "av_write_frame")
	}
	return nil
}

// InterleavedWriteFrame writes an interleaved packet to the output file.
func InterleavedWriteFrame(ctx FormatContext, pkt avcodec.Packet) error {
	if avInterleavedWriteFrame == nil {
		return bindings.ErrNotLoaded
	}
	ret := avInterleavedWriteFrame(ctx, pkt)
	runtime.KeepAlive(pkt)
	if ret < 0 {
		return avutil.NewError(ret, "av_interleaved_write_frame")
	}
	return nil
}

// SeekFlags for SeekFrame
const (
	SeekFlagBackward = 1 // Seek to keyframe before target
	SeekFlagByte     = 2 // Seek by byte position
	SeekFlagAny      = 4 // Seek to any frame (not just keyframe)
	SeekFlagFrame    = 8 // Seek by frame number
)

// SeekFrame seeks to a position in the stream.
func SeekFrame(ctx FormatContext, streamIndex int32, timestamp int64, flags int32) error {
	if avSeekFrame == nil {
		return bindings.ErrNotLoaded
	}
	ret := avSeekFrame(ctx, streamIndex, timestamp, flags)
	if ret < 0 {
		return avutil.NewError(ret, "av_seek_frame")
	}
	return nil
}

// FindBestStream finds the best stream of a given type.
// Returns the stream index, or < 0 if not found.
func FindBestStream(ctx FormatContext, mediaType avutil.MediaType, wanted, related int32, decoder *avcodec.Codec, flags int32) int32 {
	if avFindBestStream == nil {
		return -1
	}
	return avFindBestStream(ctx, int32(mediaType), wanted, related, decoder, flags)
}

// IOOpen opens an I/O context.
func IOOpen(ctx *IOContext, url string, flags int32) error {
	if avioOpen == nil {
		return bindings.ErrNotLoaded
	}
	ret := avioOpen(ctx, url, flags)
	runtime.KeepAlive(url)
	if ret < 0 {
		return avutil.NewError(ret, "avio_open")
	}
	return nil
}

// IOClose closes an I/O context.
func IOClose(ctx IOContext) error {
	if ctx == nil || avioClose == nil {
		return nil
	}
	ret := avioClose(ctx)
	if ret < 0 {
		return avutil.NewError(ret, "avio_close")
	}
	return nil
}

// IOCloseP closes an I/O context and sets the pointer to nil.
func IOCloseP(ctx *IOContext) error {
	if ctx == nil || *ctx == nil || avioClosep == nil {
		return nil
	}
	ret := avioClosep(ctx)
	*ctx = nil
	if ret < 0 {
		return avutil.NewError(ret, "avio_closep")
	}
	return nil
}

// AVIO flags
const (
	IOFlagRead      = 1
	IOFlagWrite     = 2
	IOFlagReadWrite = IOFlagRead | IOFlagWrite
)

// AllocPacket allocates a packet.
func AllocPacket() avcodec.Packet {
	if avPacketAlloc == nil {
		return nil
	}
	return avPacketAlloc()
}

// FreePacket frees a packet.
func FreePacket(pkt *avcodec.Packet) {
	if pkt == nil || *pkt == nil || avPacketFree == nil {
		return
	}
	avPacketFree(pkt)
	*pkt = nil
}

// PacketUnref unreferences a packet's buffers.
func PacketUnref(pkt avcodec.Packet) {
	if pkt == nil || avPacketUnref == nil {
		return
	}
	avPacketUnref(pkt)
}

// AVFormatContext struct field offsets (for FFmpeg 6.x / avformat 60.x)
// Verified with offsetof() on FFmpeg 60.16.100
const (
	offsetIOContext   = 32 // AVIOContext *pb
	offsetNumStreams  = 44 // unsigned int nb_streams
	offsetStreams     = 48 // AVStream **streams
	offsetDuration    = 72 // int64_t duration
	offsetBitRate     = 80 // int64_t bit_rate
)

// GetNumStreams returns the number of streams in the context.
func GetNumStreams(ctx FormatContext) int {
	if ctx == nil {
		return 0
	}
	return int(*(*uint32)(unsafe.Pointer(uintptr(ctx) + offsetNumStreams)))
}

// GetStream returns the stream at the given index.
func GetStream(ctx FormatContext, index int) Stream {
	if ctx == nil || index < 0 {
		return nil
	}
	numStreams := GetNumStreams(ctx)
	if index >= numStreams {
		return nil
	}
	streamsPtr := *(*unsafe.Pointer)(unsafe.Pointer(uintptr(ctx) + offsetStreams))
	if streamsPtr == nil {
		return nil
	}
	streamArray := (*[1024]unsafe.Pointer)(streamsPtr)
	return streamArray[index]
}

// GetDuration returns the duration in AV_TIME_BASE units.
func GetDuration(ctx FormatContext) int64 {
	if ctx == nil {
		return 0
	}
	return *(*int64)(unsafe.Pointer(uintptr(ctx) + offsetDuration))
}

// GetBitRate returns the bit rate.
func GetBitRate(ctx FormatContext) int64 {
	if ctx == nil {
		return 0
	}
	return *(*int64)(unsafe.Pointer(uintptr(ctx) + offsetBitRate))
}

// GetIOContext returns the I/O context.
func GetIOContext(ctx FormatContext) IOContext {
	if ctx == nil {
		return nil
	}
	return *(*unsafe.Pointer)(unsafe.Pointer(uintptr(ctx) + offsetIOContext))
}

// SetIOContext sets the I/O context.
func SetIOContext(ctx FormatContext, pb IOContext) {
	if ctx == nil {
		return
	}
	*(*unsafe.Pointer)(unsafe.Pointer(uintptr(ctx) + offsetIOContext)) = pb
}

// AVStream struct field offsets (for FFmpeg 6.x / avformat 60.x)
// Verified with offsetof() on FFmpeg 60.16.100
const (
	offsetStreamIndex    = 8  // int index
	offsetStreamID       = 12 // int id
	offsetStreamCodecPar = 16 // AVCodecParameters *codecpar
	offsetStreamTimeBase = 32 // AVRational time_base
)

// GetStreamIndex returns the stream index.
func GetStreamIndex(stream Stream) int32 {
	if stream == nil {
		return -1
	}
	return *(*int32)(unsafe.Pointer(uintptr(stream) + offsetStreamIndex))
}

// GetStreamCodecPar returns the codec parameters for the stream.
func GetStreamCodecPar(stream Stream) avcodec.Parameters {
	if stream == nil {
		return nil
	}
	return *(*unsafe.Pointer)(unsafe.Pointer(uintptr(stream) + offsetStreamCodecPar))
}

// AVCodecParameters struct field offsets (for FFmpeg 6.x / avcodec 60.x)
// Verified with offsetof() on FFmpeg 60.x
const (
	offsetCodecParType       = 0   // enum AVMediaType codec_type
	offsetCodecParCodecID    = 4   // enum AVCodecID codec_id
	offsetCodecParFormat     = 28  // int format (pixel format or sample format)
	offsetCodecParWidth      = 56  // int width
	offsetCodecParHeight     = 60  // int height
	offsetCodecParSampleRate = 116 // int sample_rate
)

// GetCodecParType returns the media type from codec parameters.
func GetCodecParType(par avcodec.Parameters) avutil.MediaType {
	if par == nil {
		return avutil.MediaTypeUnknown
	}
	return avutil.MediaType(*(*int32)(unsafe.Pointer(uintptr(par) + offsetCodecParType)))
}

// GetCodecParCodecID returns the codec ID from codec parameters.
func GetCodecParCodecID(par avcodec.Parameters) avcodec.CodecID {
	if par == nil {
		return avcodec.CodecIDNone
	}
	return avcodec.CodecID(*(*int32)(unsafe.Pointer(uintptr(par) + offsetCodecParCodecID)))
}

// GetCodecParWidth returns the video width from codec parameters.
func GetCodecParWidth(par avcodec.Parameters) int32 {
	if par == nil {
		return 0
	}
	return *(*int32)(unsafe.Pointer(uintptr(par) + offsetCodecParWidth))
}

// GetCodecParHeight returns the video height from codec parameters.
func GetCodecParHeight(par avcodec.Parameters) int32 {
	if par == nil {
		return 0
	}
	return *(*int32)(unsafe.Pointer(uintptr(par) + offsetCodecParHeight))
}

// GetCodecParFormat returns the pixel format (video) or sample format (audio).
func GetCodecParFormat(par avcodec.Parameters) int32 {
	if par == nil {
		return -1
	}
	return *(*int32)(unsafe.Pointer(uintptr(par) + offsetCodecParFormat))
}

// GetCodecParSampleRate returns the audio sample rate.
func GetCodecParSampleRate(par avcodec.Parameters) int32 {
	if par == nil {
		return 0
	}
	return *(*int32)(unsafe.Pointer(uintptr(par) + offsetCodecParSampleRate))
}

// AVFormatContext output field offsets (for FFmpeg 6.x)
const (
	offsetOformat = 16 // AVOutputFormat *oformat
)

// AVOutputFormat field offsets (for FFmpeg 6.x)
const (
	offsetOutputFormatFlags = 44 // int flags
)

// Output format flag constants
const (
	AVFMT_NOFILE       = 0x0001 // No file, can be custom I/O
	AVFMT_GLOBALHEADER = 0x0040 // Format wants global header
)

// GetOutputFormat returns the output format from a format context.
func GetOutputFormat(ctx FormatContext) OutputFormat {
	if ctx == nil {
		return nil
	}
	return *(*unsafe.Pointer)(unsafe.Pointer(uintptr(ctx) + offsetOformat))
}

// GetOutputFormatFlags returns the flags from an output format.
func GetOutputFormatFlags(oformat OutputFormat) int32 {
	if oformat == nil {
		return 0
	}
	return *(*int32)(unsafe.Pointer(uintptr(oformat) + offsetOutputFormatFlags))
}

// NeedsGlobalHeader returns true if the output format needs global header.
func NeedsGlobalHeader(ctx FormatContext) bool {
	oformat := GetOutputFormat(ctx)
	if oformat == nil {
		return false
	}
	flags := GetOutputFormatFlags(oformat)
	return flags&AVFMT_GLOBALHEADER != 0
}

// HasNoFile returns true if the output format doesn't need a file (custom I/O).
func HasNoFile(ctx FormatContext) bool {
	oformat := GetOutputFormat(ctx)
	if oformat == nil {
		return false
	}
	flags := GetOutputFormatFlags(oformat)
	return flags&AVFMT_NOFILE != 0
}

// SetStreamTimeBase sets the time base for a stream.
func SetStreamTimeBase(stream Stream, num, den int32) {
	if stream == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(stream) + offsetStreamTimeBase)) = num
	*(*int32)(unsafe.Pointer(uintptr(stream) + offsetStreamTimeBase + 4)) = den
}

// GetStreamTimeBase returns the time base for a stream.
func GetStreamTimeBase(stream Stream) (num, den int32) {
	if stream == nil {
		return 0, 1
	}
	num = *(*int32)(unsafe.Pointer(uintptr(stream) + offsetStreamTimeBase))
	den = *(*int32)(unsafe.Pointer(uintptr(stream) + offsetStreamTimeBase + 4))
	return
}
