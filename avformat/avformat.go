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
	avformatOpenInput       func(ctx *unsafe.Pointer, url string, fmt uintptr, options *unsafe.Pointer) int32
	avformatCloseInput      func(ctx *unsafe.Pointer)
	avformatFindStreamInfo  func(ctx uintptr, options *unsafe.Pointer) int32
	avformatAllocContext    func() uintptr
	avformatFreeContext     func(ctx uintptr)
	avformatAllocOutputCtx2 func(ctx *unsafe.Pointer, oformat uintptr, formatName, filename string) int32
	avformatNewStream       func(ctx, codec uintptr) uintptr
	avformatWriteHeader     func(ctx uintptr, options *unsafe.Pointer) int32
	avWriteTrailer          func(ctx uintptr) int32

	avReadFrame             func(ctx, pkt uintptr) int32
	avWriteFrame            func(ctx, pkt uintptr) int32
	avInterleavedWriteFrame func(ctx, pkt uintptr) int32
	avSeekFrame             func(ctx uintptr, streamIndex int32, timestamp int64, flags int32) int32

	avFindBestStream  func(ctx uintptr, mediaType, wanted, related int32, decoder *unsafe.Pointer, flags int32) int32
	avFindInputFormat func(name string) uintptr
	avDemuxerIterate  func(opaque *unsafe.Pointer) uintptr

	avioOpen         func(ctx *unsafe.Pointer, url string, flags int32) int32
	avioOpen2        func(ctx *unsafe.Pointer, url string, flags int32, intCb uintptr, options *unsafe.Pointer) int32
	avioClose        func(ctx uintptr) int32
	avioClosep       func(ctx *unsafe.Pointer) int32
	avioAllocContext func(buffer uintptr, bufferSize, writeFlag int32, opaque uintptr, readPacket, writePacket, seek uintptr) uintptr
	avioContextFree  func(ctx *unsafe.Pointer)

	// Packet functions (in avcodec but often used with avformat)
	avPacketAlloc func() uintptr
	avPacketFree  func(pkt *unsafe.Pointer)
	avPacketUnref func(pkt uintptr)

	// Dictionary functions (from avutil, used for metadata)
	avDictGet func(m uintptr, key string, prev uintptr, flags int32) uintptr
	avDictSet func(pm *unsafe.Pointer, key, value string, flags int32) int32

	// Note: Chapter creation uses the shim (internal/shim.NewChapter) because avformat_new_chapter
	// has a complex signature that doesn't work well with purego's dynamic binding.

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
	purego.RegisterLibFunc(&avFindInputFormat, lib, "av_find_input_format")
	registerOptionalLibFunc(&avDemuxerIterate, lib, "av_demuxer_iterate")

	purego.RegisterLibFunc(&avioOpen, lib, "avio_open")
	registerOptionalLibFunc(&avioOpen2, lib, "avio_open2")
	purego.RegisterLibFunc(&avioClose, lib, "avio_close")
	purego.RegisterLibFunc(&avioClosep, lib, "avio_closep")
	purego.RegisterLibFunc(&avioAllocContext, lib, "avio_alloc_context")
	purego.RegisterLibFunc(&avioContextFree, lib, "avio_context_free")

	// Packet functions from avcodec
	if libCodec != 0 {
		purego.RegisterLibFunc(&avPacketAlloc, libCodec, "av_packet_alloc")
		purego.RegisterLibFunc(&avPacketFree, libCodec, "av_packet_free")
		purego.RegisterLibFunc(&avPacketUnref, libCodec, "av_packet_unref")
	}

	// Dictionary functions from avutil
	libUtil := bindings.LibAVUtil()
	if libUtil != 0 {
		purego.RegisterLibFunc(&avDictGet, libUtil, "av_dict_get")
		purego.RegisterLibFunc(&avDictSet, libUtil, "av_dict_set")
	}

	bindingsRegistered = true
}

func registerOptionalLibFunc(fptr any, handle uintptr, name string) {
	defer func() { _ = recover() }()
	purego.RegisterLibFunc(fptr, handle, name)
}

// AllocContext allocates an AVFormatContext.
func AllocContext() FormatContext {
	if avformatAllocContext == nil {
		return nil
	}
	return unsafe.Pointer(avformatAllocContext())
}

// FreeContext frees an AVFormatContext.
func FreeContext(ctx FormatContext) {
	if ctx == nil || avformatFreeContext == nil {
		return
	}
	avformatFreeContext(uintptr(ctx))
}

// FindInputFormat finds an input format by short name.
// Returns nil if the format is not found.
func FindInputFormat(name string) InputFormat {
	if avFindInputFormat == nil {
		return nil
	}
	f := unsafe.Pointer(avFindInputFormat(name))
	runtime.KeepAlive(name)
	return f
}

// OpenInput opens an input file.
// options is a pointer to an AVDictionary that may be modified by FFmpeg.
func OpenInput(ctx *FormatContext, url string, fmt InputFormat, options *avutil.Dictionary) error {
	if avformatOpenInput == nil {
		return bindings.ErrNotLoaded
	}
	// Pass nil or a pointer to the dictionary pointer
	var optsPtr *unsafe.Pointer
	if options != nil {
		optsPtr = options
	}
	ret := avformatOpenInput(ctx, url, uintptr(fmt), optsPtr)
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
	ret := avformatFindStreamInfo(uintptr(ctx), options)
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
	ret := avformatAllocOutputCtx2(ctx, uintptr(oformat), formatName, filename)
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
	return unsafe.Pointer(avformatNewStream(uintptr(ctx), uintptr(codec)))
}

// WriteHeader writes the file header.
func WriteHeader(ctx FormatContext, options *avutil.Dictionary) error {
	if avformatWriteHeader == nil {
		return bindings.ErrNotLoaded
	}
	ret := avformatWriteHeader(uintptr(ctx), options)
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
	ret := avWriteTrailer(uintptr(ctx))
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
	ret := avReadFrame(uintptr(ctx), uintptr(pkt))
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
	ret := avWriteFrame(uintptr(ctx), uintptr(pkt))
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
	ret := avInterleavedWriteFrame(uintptr(ctx), uintptr(pkt))
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
	ret := avSeekFrame(uintptr(ctx), streamIndex, timestamp, flags)
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
	return avFindBestStream(uintptr(ctx), int32(mediaType), wanted, related, (*unsafe.Pointer)(unsafe.Pointer(decoder)), flags)
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

// IOOpen2 opens an I/O context with options (avio_open2).
// options is a pointer to an AVDictionary that may be modified by FFmpeg.
func IOOpen2(ctx *IOContext, url string, flags int32, options *avutil.Dictionary) error {
	if avioOpen2 == nil {
		return bindings.ErrNotLoaded
	}
	var optsPtr *unsafe.Pointer
	if options != nil {
		optsPtr = options
	}
	ret := avioOpen2(ctx, url, flags, 0, optsPtr)
	runtime.KeepAlive(url)
	if ret < 0 {
		return avutil.NewError(ret, "avio_open2")
	}
	return nil
}

// IOClose closes an I/O context.
func IOClose(ctx IOContext) error {
	if ctx == nil || avioClose == nil {
		return nil
	}
	ret := avioClose(uintptr(ctx))
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
	return unsafe.Pointer(avPacketAlloc())
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
	avPacketUnref(uintptr(pkt))
}

// AVFormatContext struct field offsets (for FFmpeg 6.x / avformat 60.x)
// Verified with offsetof() on FFmpeg 60.16.100
const (
	offsetIOContext       = 32  // AVIOContext *pb
	offsetInputFormat     = 8   // AVInputFormat *iformat
	offsetNumStreams      = 44  // unsigned int nb_streams
	offsetStreams         = 48  // AVStream **streams
	offsetDuration        = 72  // int64_t duration
	offsetBitRate         = 80  // int64_t bit_rate
	offsetNbPrograms      = 132 // unsigned int nb_programs
	offsetPrograms        = 136 // AVProgram **programs
	offsetNbChapters      = 164 // unsigned int nb_chapters
	offsetChapters        = 168 // AVChapter **chapters
	offsetContextMetadata = 176 // AVDictionary *metadata
	offsetProbeScore      = 300 // int probe_score
)

// AVChapter struct field offsets (for FFmpeg 6.x)
const (
	offsetChapterID       = 0  // int64_t id (actually stored as int64_t in FFmpeg 6.x)
	offsetChapterTimeBase = 8  // AVRational time_base (num at +8, den at +12)
	offsetChapterStart    = 16 // int64_t start
	offsetChapterEnd      = 24 // int64_t end
	offsetChapterMetadata = 32 // AVDictionary *metadata
)

// Chapter is an opaque FFmpeg AVChapter pointer.
type Chapter = unsafe.Pointer

// Program is an opaque FFmpeg AVProgram pointer.
type Program = unsafe.Pointer

// AVProgram struct field offsets (FFmpeg 6.x/7.x)
const (
	offsetProgramID            = 0  // int id
	offsetProgramStreamIndex   = 16 // int *stream_index
	offsetProgramNbStreamIndex = 24 // unsigned nb_stream_indexes
	offsetProgramMetadata      = 32 // AVDictionary *metadata
)

// GetNumPrograms returns the number of programs in the context (e.g. MPEG-TS).
func GetNumPrograms(ctx FormatContext) int {
	if ctx == nil {
		return 0
	}
	return int(*(*uint32)(unsafe.Pointer(uintptr(ctx) + offsetNbPrograms)))
}

// GetProgram returns the program at the given index.
func GetProgram(ctx FormatContext, index int) Program {
	if ctx == nil || index < 0 {
		return nil
	}
	n := GetNumPrograms(ctx)
	if index >= n {
		return nil
	}
	programsPtr := *(*unsafe.Pointer)(unsafe.Pointer(uintptr(ctx) + offsetPrograms))
	if programsPtr == nil {
		return nil
	}
	programArray := (*[1024]unsafe.Pointer)(programsPtr)
	return programArray[index]
}

// GetProgramID returns the program id.
func GetProgramID(p Program) int {
	if p == nil {
		return 0
	}
	return int(*(*int32)(unsafe.Pointer(uintptr(p) + offsetProgramID)))
}

// GetProgramStreamIndexes returns a copy of the program's stream indexes.
func GetProgramStreamIndexes(p Program) []int {
	if p == nil {
		return nil
	}
	nb := int(*(*uint32)(unsafe.Pointer(uintptr(p) + offsetProgramNbStreamIndex)))
	if nb <= 0 {
		return nil
	}
	ptr := *(*unsafe.Pointer)(unsafe.Pointer(uintptr(p) + offsetProgramStreamIndex))
	if ptr == nil {
		return nil
	}
	// stream_index is int*, but int is 32-bit in FFmpeg structs.
	raw := unsafe.Slice((*int32)(ptr), nb)
	out := make([]int, 0, nb)
	for _, v := range raw {
		out = append(out, int(v))
	}
	return out
}

// GetProgramMetadata returns the program-level metadata dictionary.
func GetProgramMetadata(p Program) avutil.Dictionary {
	if p == nil {
		return nil
	}
	return *(*unsafe.Pointer)(unsafe.Pointer(uintptr(p) + offsetProgramMetadata))
}

// AVInputFormat struct field offsets (FFmpeg 6.x/7.x)
const (
	offsetInputFormatName     = 0 // const char *name
	offsetInputFormatLongName = 8 // const char *long_name
)

// GetInputFormat returns the input format (demuxer) selected for the context.
func GetInputFormat(ctx FormatContext) InputFormat {
	if ctx == nil {
		return nil
	}
	return *(*unsafe.Pointer)(unsafe.Pointer(uintptr(ctx) + offsetInputFormat))
}

// GetProbeScore returns FFmpeg's probe confidence score for the selected demuxer.
func GetProbeScore(ctx FormatContext) int {
	if ctx == nil {
		return 0
	}
	return int(*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetProbeScore)))
}

// InputFormatName returns the demuxer short name.
func InputFormatName(f InputFormat) string {
	if f == nil {
		return ""
	}
	namePtr := *(*unsafe.Pointer)(unsafe.Pointer(uintptr(f) + offsetInputFormatName))
	return goString(namePtr)
}

// InputFormatLongName returns the demuxer long name.
func InputFormatLongName(f InputFormat) string {
	if f == nil {
		return ""
	}
	namePtr := *(*unsafe.Pointer)(unsafe.Pointer(uintptr(f) + offsetInputFormatLongName))
	return goString(namePtr)
}

// DemuxerNames returns the names of available input formats (demuxers), if supported by the FFmpeg build.
// On older FFmpeg builds where av_demuxer_iterate is missing, it returns nil.
func DemuxerNames() []string {
	if avDemuxerIterate == nil {
		return nil
	}
	var opaque unsafe.Pointer
	var out []string
	for {
		f := unsafe.Pointer(avDemuxerIterate(&opaque))
		if f == nil {
			break
		}
		if n := InputFormatName(f); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// GetNumStreams returns the number of streams in the context.
func GetNumStreams(ctx FormatContext) int {
	if ctx == nil {
		return 0
	}
	return int(*(*uint32)(unsafe.Pointer(uintptr(ctx) + offsetNumStreams)))
}

// GetNbStreams is an alias for GetNumStreams (matches FFmpeg naming).
func GetNbStreams(ctx FormatContext) int {
	return GetNumStreams(ctx)
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

// AVStream struct field offsets (for FFmpeg 6.x/7.x)
// Verified with offsetof() on FFmpeg 7.1.1
const (
	offsetStreamIndex        = 8  // int index
	offsetStreamID           = 12 // int id
	offsetStreamCodecPar     = 16 // AVCodecParameters *codecpar
	offsetStreamTimeBase     = 32 // AVRational time_base
	offsetStreamMetadata     = 80 // AVDictionary *metadata
	offsetStreamAvgFrameRate = 88 // AVRational avg_frame_rate
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

// AVCodecParameters struct field offsets (for FFmpeg 6.x/7.x)
// Verified with offsetof() on FFmpeg 7.1.1
const (
	offsetCodecParType          = 0   // enum AVMediaType codec_type
	offsetCodecParCodecID       = 4   // enum AVCodecID codec_id
	offsetCodecParExtradata     = 16  // uint8_t *extradata
	offsetCodecParExtradataSize = 24  // int extradata_size
	offsetCodecParFormat        = 28  // int format (pixel format or sample format)
	offsetCodecParWidth         = 56  // int width
	offsetCodecParHeight        = 60  // int height
	offsetCodecParSampleRate    = 116 // int sample_rate
	offsetCodecParChannels      = 148 // ch_layout.nb_channels (int in AVChannelLayout at offset 136 + 12)
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

// GetCodecParChannels returns the number of audio channels.
func GetCodecParChannels(par avcodec.Parameters) int32 {
	if par == nil {
		return 0
	}
	return *(*int32)(unsafe.Pointer(uintptr(par) + offsetCodecParChannels))
}

// GetCodecParExtradata returns the extradata bytes from codec parameters.
// This is used for attachment data in MKV/other containers.
func GetCodecParExtradata(par avcodec.Parameters) []byte {
	if par == nil {
		return nil
	}
	dataPtr := *(*unsafe.Pointer)(unsafe.Pointer(uintptr(par) + offsetCodecParExtradata))
	if dataPtr == nil {
		return nil
	}
	size := *(*int32)(unsafe.Pointer(uintptr(par) + offsetCodecParExtradataSize))
	if size <= 0 {
		return nil
	}
	// Make a copy of the data to return to Go
	result := make([]byte, size)
	copy(result, unsafe.Slice((*byte)(dataPtr), size))
	return result
}

// GetCodecParExtradataSize returns the size of extradata.
func GetCodecParExtradataSize(par avcodec.Parameters) int {
	if par == nil {
		return 0
	}
	return int(*(*int32)(unsafe.Pointer(uintptr(par) + offsetCodecParExtradataSize)))
}

// SetCodecParType sets the media type in codec parameters.
func SetCodecParType(par avcodec.Parameters, mediaType avutil.MediaType) {
	if par == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(par) + offsetCodecParType)) = int32(mediaType)
}

// SetCodecParCodecID sets the codec ID in codec parameters.
func SetCodecParCodecID(par avcodec.Parameters, codecID avcodec.CodecID) {
	if par == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(par) + offsetCodecParCodecID)) = int32(codecID)
}

// SetCodecParExtradata sets the extradata in codec parameters.
// The data is copied into memory allocated by FFmpeg's allocator.
// Any existing extradata is freed first.
func SetCodecParExtradata(par avcodec.Parameters, data []byte) {
	if par == nil {
		return
	}

	// Get existing extradata pointer to free it
	existingPtr := *(*unsafe.Pointer)(unsafe.Pointer(uintptr(par) + offsetCodecParExtradata))
	if existingPtr != nil {
		avutil.Free(existingPtr)
	}

	if len(data) == 0 {
		*(*unsafe.Pointer)(unsafe.Pointer(uintptr(par) + offsetCodecParExtradata)) = nil
		*(*int32)(unsafe.Pointer(uintptr(par) + offsetCodecParExtradataSize)) = 0
		return
	}

	// Allocate memory using FFmpeg's allocator
	newPtr := avutil.Malloc(uintptr(len(data)))
	if newPtr == nil {
		return
	}

	// Copy data
	dst := unsafe.Slice((*byte)(newPtr), len(data))
	copy(dst, data)

	// Set pointer and size
	*(*unsafe.Pointer)(unsafe.Pointer(uintptr(par) + offsetCodecParExtradata)) = newPtr
	*(*int32)(unsafe.Pointer(uintptr(par) + offsetCodecParExtradataSize)) = int32(len(data))
}

// GetStreamAvgFrameRate returns the average frame rate (num/den).
func GetStreamAvgFrameRate(stream Stream) (num, den int32) {
	if stream == nil {
		return 0, 1
	}
	num = *(*int32)(unsafe.Pointer(uintptr(stream) + offsetStreamAvgFrameRate))
	den = *(*int32)(unsafe.Pointer(uintptr(stream) + offsetStreamAvgFrameRate + 4))
	return
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

// Format context flag constants
const (
	AVFMT_FLAG_GENPTS       = 0x0001  // Generate missing pts
	AVFMT_FLAG_IGNIDX       = 0x0002  // Ignore index
	AVFMT_FLAG_NONBLOCK     = 0x0004  // Do not block when reading packets
	AVFMT_FLAG_IGNDTS       = 0x0008  // Ignore DTS on frames that contain both DTS & PTS
	AVFMT_FLAG_NOFILLIN     = 0x0010  // Do not infer values from other values
	AVFMT_FLAG_NOPARSE      = 0x0020  // Do not use AVParsers
	AVFMT_FLAG_NOBUFFER     = 0x0040  // Do not buffer frames
	AVFMT_FLAG_CUSTOM_IO    = 0x0080  // The caller has supplied a custom AVIOContext
	AVFMT_FLAG_DISCARD_CORR = 0x0100  // Discard corrupted frames
	AVFMT_FLAG_FLUSH_PKTS   = 0x0200  // Flush AVIOContext every packet
	AVFMT_FLAG_BITEXACT     = 0x0400  // Deterministic output
	AVFMT_FLAG_SORT_DTS     = 0x10000 // Try to interleave output packets by dts
	AVFMT_FLAG_FAST_SEEK    = 0x80000 // Enable fast seeking
)

// AVFormatContext flags field offset (for FFmpeg 6.x/7.x)
const offsetFlags = 96

// GetFlags returns the flags from a format context.
func GetFlags(ctx FormatContext) int32 {
	if ctx == nil {
		return 0
	}
	return *(*int32)(unsafe.Pointer(uintptr(ctx) + offsetFlags))
}

// SetFlags sets the flags on a format context.
func SetFlags(ctx FormatContext, flags int32) {
	if ctx == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetFlags)) = flags
}

// AddFlags adds flags to a format context (OR operation).
func AddFlags(ctx FormatContext, flags int32) {
	if ctx == nil {
		return
	}
	current := GetFlags(ctx)
	SetFlags(ctx, current|flags)
}

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

// IOAllocContext allocates and initializes an AVIOContext for custom I/O.
// buffer should be allocated with av_malloc and will be freed by avio_context_free.
// bufferSize is the size of buffer.
// writeFlag is 1 if the buffer should be writable, 0 if readable.
// opaque is passed to all callbacks.
// readPacket, writePacket, seek are callback function pointers (use purego.NewCallback).
func IOAllocContext(buffer unsafe.Pointer, bufferSize int, writeFlag bool, opaque unsafe.Pointer, readPacket, writePacket, seek uintptr) IOContext {
	if avioAllocContext == nil {
		return nil
	}
	wf := int32(0)
	if writeFlag {
		wf = 1
	}
	return unsafe.Pointer(avioAllocContext(uintptr(buffer), int32(bufferSize), wf, uintptr(opaque), readPacket, writePacket, seek))
}

// IOContextFree frees an AVIOContext allocated with IOAllocContext.
func IOContextFree(ctx *IOContext) {
	if ctx == nil || *ctx == nil || avioContextFree == nil {
		return
	}
	avioContextFree(ctx)
	*ctx = nil
}

// Dictionary constants
const (
	AV_DICT_MATCH_CASE      = 1  // Only get an entry with exact-case key match
	AV_DICT_IGNORE_SUFFIX   = 2  // Return first entry in a dictionary whose first part matches the search key
	AV_DICT_DONT_STRDUP     = 4  // Take ownership of a key/value that has been allocated with av_malloc()
	AV_DICT_DONT_STRDUP_KEY = 4  // Same as AV_DICT_DONT_STRDUP
	AV_DICT_DONT_STRDUP_VAL = 8  // Take ownership of value
	AV_DICT_DONT_OVERWRITE  = 16 // Don't overwrite existing entries
	AV_DICT_APPEND          = 32 // Append to existing entry value
	AV_DICT_MULTIKEY        = 64 // Allow to store several equal keys in the dictionary
)

// AVDictionaryEntry struct field offsets
const (
	offsetDictEntryKey   = 0 // char *key
	offsetDictEntryValue = 8 // char *value
)

// GetMetadata returns the metadata dictionary from a format context.
func GetMetadata(ctx FormatContext) avutil.Dictionary {
	if ctx == nil {
		return nil
	}
	return *(*unsafe.Pointer)(unsafe.Pointer(uintptr(ctx) + offsetContextMetadata))
}

// GetStreamMetadata returns the metadata dictionary from a stream.
func GetStreamMetadata(stream Stream) avutil.Dictionary {
	if stream == nil {
		return nil
	}
	return *(*unsafe.Pointer)(unsafe.Pointer(uintptr(stream) + offsetStreamMetadata))
}

// SetMetadata sets a metadata key-value pair on a format context.
func SetMetadata(ctx FormatContext, key, value string) error {
	if ctx == nil {
		return bindings.ErrNotLoaded
	}
	if avDictSet == nil {
		return bindings.ErrNotLoaded
	}
	metaPtr := unsafe.Pointer(uintptr(ctx) + offsetContextMetadata)
	ret := avDictSet((*unsafe.Pointer)(metaPtr), key, value, 0)
	runtime.KeepAlive(key)
	runtime.KeepAlive(value)
	if ret < 0 {
		return avutil.NewError(ret, "av_dict_set")
	}
	return nil
}

// SetStreamMetadata sets a metadata key-value pair on a stream.
func SetStreamMetadata(stream Stream, key, value string) error {
	if stream == nil {
		return bindings.ErrNotLoaded
	}
	if avDictSet == nil {
		return bindings.ErrNotLoaded
	}
	metaPtr := unsafe.Pointer(uintptr(stream) + offsetStreamMetadata)
	ret := avDictSet((*unsafe.Pointer)(metaPtr), key, value, 0)
	runtime.KeepAlive(key)
	runtime.KeepAlive(value)
	if ret < 0 {
		return avutil.NewError(ret, "av_dict_set")
	}
	return nil
}

// GetNumChapters returns the number of chapters in the context.
func GetNumChapters(ctx FormatContext) int {
	if ctx == nil {
		return 0
	}
	return int(*(*uint32)(unsafe.Pointer(uintptr(ctx) + offsetNbChapters)))
}

// GetChapter returns the chapter at the given index.
func GetChapter(ctx FormatContext, index int) Chapter {
	if ctx == nil || index < 0 {
		return nil
	}
	numChapters := GetNumChapters(ctx)
	if index >= numChapters {
		return nil
	}
	chaptersPtr := *(*unsafe.Pointer)(unsafe.Pointer(uintptr(ctx) + offsetChapters))
	if chaptersPtr == nil {
		return nil
	}
	chapterArray := (*[1024]unsafe.Pointer)(chaptersPtr)
	return chapterArray[index]
}

// GetChapterID returns the chapter ID.
func GetChapterID(ch Chapter) int64 {
	if ch == nil {
		return 0
	}
	return *(*int64)(unsafe.Pointer(uintptr(ch) + offsetChapterID))
}

// GetChapterTimeBase returns the time base (numerator and denominator) of the chapter.
func GetChapterTimeBase(ch Chapter) (num, den int32) {
	if ch == nil {
		return 0, 1
	}
	num = *(*int32)(unsafe.Pointer(uintptr(ch) + offsetChapterTimeBase))
	den = *(*int32)(unsafe.Pointer(uintptr(ch) + offsetChapterTimeBase + 4))
	return num, den
}

// GetChapterStart returns the start time of the chapter in time_base units.
func GetChapterStart(ch Chapter) int64 {
	if ch == nil {
		return 0
	}
	return *(*int64)(unsafe.Pointer(uintptr(ch) + offsetChapterStart))
}

// GetChapterEnd returns the end time of the chapter in time_base units.
func GetChapterEnd(ch Chapter) int64 {
	if ch == nil {
		return 0
	}
	return *(*int64)(unsafe.Pointer(uintptr(ch) + offsetChapterEnd))
}

// GetChapterMetadata returns the metadata dictionary from a chapter.
func GetChapterMetadata(ch Chapter) avutil.Dictionary {
	if ch == nil {
		return nil
	}
	return *(*unsafe.Pointer)(unsafe.Pointer(uintptr(ch) + offsetChapterMetadata))
}

// DictGet retrieves a dictionary entry.
// Pass nil for prev to get the first entry, or the previous entry to iterate.
// Use AV_DICT_IGNORE_SUFFIX with empty key to iterate all entries.
func DictGet(dict avutil.Dictionary, key string, prev unsafe.Pointer, flags int32) unsafe.Pointer {
	if dict == nil || avDictGet == nil {
		return nil
	}
	result := unsafe.Pointer(avDictGet(uintptr(dict), key, uintptr(prev), flags))
	runtime.KeepAlive(key)
	return result
}

// DictEntryKey returns the key from a dictionary entry.
func DictEntryKey(entry unsafe.Pointer) string {
	if entry == nil {
		return ""
	}
	keyPtr := *(*unsafe.Pointer)(unsafe.Pointer(uintptr(entry) + offsetDictEntryKey))
	if keyPtr == nil {
		return ""
	}
	return goString(keyPtr)
}

// DictEntryValue returns the value from a dictionary entry.
func DictEntryValue(entry unsafe.Pointer) string {
	if entry == nil {
		return ""
	}
	valuePtr := *(*unsafe.Pointer)(unsafe.Pointer(uintptr(entry) + offsetDictEntryValue))
	if valuePtr == nil {
		return ""
	}
	return goString(valuePtr)
}

// goString converts a C string to a Go string.
func goString(ptr unsafe.Pointer) string {
	if ptr == nil {
		return ""
	}
	// Find null terminator
	var length int
	for {
		b := *(*byte)(unsafe.Pointer(uintptr(ptr) + uintptr(length)))
		if b == 0 {
			break
		}
		length++
		// Safety limit
		if length > 4096 {
			break
		}
	}
	if length == 0 {
		return ""
	}
	return string((*[4096]byte)(ptr)[:length:length])
}
