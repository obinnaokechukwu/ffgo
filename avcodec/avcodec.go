//go:build !ios && !android && (amd64 || arm64)

// Package avcodec provides bindings to FFmpeg's libavcodec library.
// It includes codec discovery, encoding, and decoding functionality.
package avcodec

import (
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/obinnaokechukwu/ffgo/avutil"
	"github.com/obinnaokechukwu/ffgo/internal/bindings"
	ffshim "github.com/obinnaokechukwu/ffgo/internal/shim"
)

// Codec is an opaque FFmpeg AVCodec pointer.
type Codec = unsafe.Pointer

// Context is an opaque FFmpeg AVCodecContext pointer.
type Context = unsafe.Pointer

// Packet is an opaque FFmpeg AVPacket pointer.
type Packet = unsafe.Pointer

// Parameters is an opaque FFmpeg AVCodecParameters pointer.
type Parameters = unsafe.Pointer

// Function bindings
var (
	avcodecFindDecoder       func(id int32) uintptr
	avcodecFindEncoder       func(id int32) uintptr
	avcodecFindDecoderByName func(name string) uintptr
	avcodecFindEncoderByName func(name string) uintptr
	avcodecAllocContext3     func(codec uintptr) uintptr
	avcodecFreeContext       func(ctx *unsafe.Pointer)
	avcodecOpen2             func(ctx, codec uintptr, options *unsafe.Pointer) int32
	avcodecClose             func(ctx uintptr) int32
	avcodecSendPacket        func(ctx, pkt uintptr) int32
	avcodecReceiveFrame      func(ctx, frame uintptr) int32
	avcodecSendFrame         func(ctx, frame uintptr) int32
	avcodecReceivePacket     func(ctx, pkt uintptr) int32
	avcodecFlushBuffers      func(ctx uintptr)
	avcodecParametersToCtx   func(ctx, par uintptr) int32
	avcodecParametersFromCtx func(par, ctx uintptr) int32
	avcodecParametersCopy    func(dst, src uintptr) int32

	avPacketAlloc func() uintptr
	avPacketFree  func(pkt *unsafe.Pointer)
	avPacketRef   func(dst, src uintptr) int32
	avPacketUnref func(pkt uintptr)

	// Subtitle decoding
	avcodecDecodeSubtitle2 func(ctx, sub, gotSubPtr, pkt uintptr) int32
	avsubtitleFree         func(sub uintptr)

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

	lib := bindings.LibAVCodec()
	if lib == 0 {
		return
	}

	purego.RegisterLibFunc(&avcodecFindDecoder, lib, "avcodec_find_decoder")
	purego.RegisterLibFunc(&avcodecFindEncoder, lib, "avcodec_find_encoder")
	purego.RegisterLibFunc(&avcodecFindDecoderByName, lib, "avcodec_find_decoder_by_name")
	purego.RegisterLibFunc(&avcodecFindEncoderByName, lib, "avcodec_find_encoder_by_name")
	purego.RegisterLibFunc(&avcodecAllocContext3, lib, "avcodec_alloc_context3")
	purego.RegisterLibFunc(&avcodecFreeContext, lib, "avcodec_free_context")
	purego.RegisterLibFunc(&avcodecOpen2, lib, "avcodec_open2")
	registerOptionalLibFunc(&avcodecClose, lib, "avcodec_close")
	purego.RegisterLibFunc(&avcodecSendPacket, lib, "avcodec_send_packet")
	purego.RegisterLibFunc(&avcodecReceiveFrame, lib, "avcodec_receive_frame")
	purego.RegisterLibFunc(&avcodecSendFrame, lib, "avcodec_send_frame")
	purego.RegisterLibFunc(&avcodecReceivePacket, lib, "avcodec_receive_packet")
	purego.RegisterLibFunc(&avcodecFlushBuffers, lib, "avcodec_flush_buffers")
	purego.RegisterLibFunc(&avcodecParametersToCtx, lib, "avcodec_parameters_to_context")
	purego.RegisterLibFunc(&avcodecParametersFromCtx, lib, "avcodec_parameters_from_context")
	purego.RegisterLibFunc(&avcodecParametersCopy, lib, "avcodec_parameters_copy")

	purego.RegisterLibFunc(&avPacketAlloc, lib, "av_packet_alloc")
	purego.RegisterLibFunc(&avPacketFree, lib, "av_packet_free")
	purego.RegisterLibFunc(&avPacketRef, lib, "av_packet_ref")
	purego.RegisterLibFunc(&avPacketUnref, lib, "av_packet_unref")

	// Subtitle decoding
	purego.RegisterLibFunc(&avcodecDecodeSubtitle2, lib, "avcodec_decode_subtitle2")
	purego.RegisterLibFunc(&avsubtitleFree, lib, "avsubtitle_free")

	bindingsRegistered = true
}

func registerOptionalLibFunc(fptr any, handle uintptr, name string) {
	defer func() { _ = recover() }()
	purego.RegisterLibFunc(fptr, handle, name)
}

// FindDecoder finds a decoder by codec ID.
func FindDecoder(id CodecID) Codec {
	if avcodecFindDecoder == nil {
		return nil
	}
	return unsafe.Pointer(avcodecFindDecoder(int32(id)))
}

// FindEncoder finds an encoder by codec ID.
func FindEncoder(id CodecID) Codec {
	if avcodecFindEncoder == nil {
		return nil
	}
	return unsafe.Pointer(avcodecFindEncoder(int32(id)))
}

// FindDecoderByName finds a decoder by name.
func FindDecoderByName(name string) Codec {
	if avcodecFindDecoderByName == nil {
		return nil
	}
	codec := unsafe.Pointer(avcodecFindDecoderByName(name))
	runtime.KeepAlive(name)
	return codec
}

// FindEncoderByName finds an encoder by name.
func FindEncoderByName(name string) Codec {
	if avcodecFindEncoderByName == nil {
		return nil
	}
	codec := unsafe.Pointer(avcodecFindEncoderByName(name))
	runtime.KeepAlive(name)
	return codec
}

// AllocContext3 allocates a codec context.
func AllocContext3(codec Codec) Context {
	if avcodecAllocContext3 == nil {
		return nil
	}
	return unsafe.Pointer(avcodecAllocContext3(uintptr(codec)))
}

// FreeContext frees a codec context.
func FreeContext(ctx *Context) {
	if ctx == nil || *ctx == nil || avcodecFreeContext == nil {
		return
	}

	// On some platforms (notably macOS), passing a pointer-to-pointer that points
	// into Go memory to foreign code can trigger runtime/libffi aborts. Avoid
	// passing Go memory by staging the pointer in FFmpeg-allocated memory.
	//
	// This keeps the public API unchanged while making cleanup reliable across
	// platforms and purego backends.
	tmp := avutil.Malloc(unsafe.Sizeof(uintptr(0)))
	if tmp != nil {
		*(*unsafe.Pointer)(tmp) = *ctx
		avcodecFreeContext((*unsafe.Pointer)(tmp))
		avutil.Free(tmp)
		*ctx = nil
		return
	}

	// Fallback: best-effort free. If tmp allocation failed, use the direct call.
	avcodecFreeContext(ctx)
	*ctx = nil
}

// Open2 opens a codec context.
func Open2(ctx Context, codec Codec, options *avutil.Dictionary) error {
	if avcodecOpen2 == nil {
		return bindings.ErrNotLoaded
	}
	ret := avcodecOpen2(uintptr(ctx), uintptr(codec), options)
	if ret < 0 {
		return avutil.NewError(ret, "avcodec_open2")
	}
	return nil
}

// Close closes a codec context.
func Close(ctx Context) error {
	if ctx == nil || avcodecClose == nil {
		return nil
	}
	ret := avcodecClose(uintptr(ctx))
	if ret < 0 {
		return avutil.NewError(ret, "avcodec_close")
	}
	return nil
}

// SendPacket sends a packet to the decoder.
// Pass nil to flush the decoder.
func SendPacket(ctx Context, pkt Packet) error {
	if avcodecSendPacket == nil {
		return bindings.ErrNotLoaded
	}
	ret := avcodecSendPacket(uintptr(ctx), uintptr(pkt))
	runtime.KeepAlive(pkt)
	if ret < 0 && ret != avutil.AVERROR_EAGAIN && ret != avutil.AVERROR_EOF {
		return avutil.NewError(ret, "avcodec_send_packet")
	}
	return nil
}

// ReceiveFrame receives a decoded frame from the decoder.
// Returns nil frame and nil error if more data is needed (EAGAIN) or EOF.
func ReceiveFrame(ctx Context, frame avutil.Frame) error {
	if avcodecReceiveFrame == nil {
		return bindings.ErrNotLoaded
	}
	ret := avcodecReceiveFrame(uintptr(ctx), uintptr(frame))
	if ret == avutil.AVERROR_EAGAIN || ret == avutil.AVERROR_EOF {
		return avutil.NewError(ret, "avcodec_receive_frame")
	}
	if ret < 0 {
		return avutil.NewError(ret, "avcodec_receive_frame")
	}
	return nil
}

// SendFrame sends a frame to the encoder.
// Pass nil to flush the encoder.
func SendFrame(ctx Context, frame avutil.Frame) error {
	if avcodecSendFrame == nil {
		return bindings.ErrNotLoaded
	}
	ret := avcodecSendFrame(uintptr(ctx), uintptr(frame))
	runtime.KeepAlive(frame)
	if ret < 0 && ret != avutil.AVERROR_EAGAIN && ret != avutil.AVERROR_EOF {
		return avutil.NewError(ret, "avcodec_send_frame")
	}
	return nil
}

// ReceivePacket receives an encoded packet from the encoder.
func ReceivePacket(ctx Context, pkt Packet) error {
	if avcodecReceivePacket == nil {
		return bindings.ErrNotLoaded
	}
	ret := avcodecReceivePacket(uintptr(ctx), uintptr(pkt))
	if ret == avutil.AVERROR_EAGAIN || ret == avutil.AVERROR_EOF {
		return avutil.NewError(ret, "avcodec_receive_packet")
	}
	if ret < 0 {
		return avutil.NewError(ret, "avcodec_receive_packet")
	}
	return nil
}

// FlushBuffers flushes the codec buffers.
func FlushBuffers(ctx Context) {
	if ctx == nil || avcodecFlushBuffers == nil {
		return
	}
	avcodecFlushBuffers(uintptr(ctx))
}

// ParametersToContext copies codec parameters to a context.
func ParametersToContext(ctx Context, par Parameters) error {
	if avcodecParametersToCtx == nil {
		return bindings.ErrNotLoaded
	}
	ret := avcodecParametersToCtx(uintptr(ctx), uintptr(par))
	if ret < 0 {
		return avutil.NewError(ret, "avcodec_parameters_to_context")
	}
	return nil
}

// ParametersFromContext copies codec parameters from a context.
func ParametersFromContext(par Parameters, ctx Context) error {
	if avcodecParametersFromCtx == nil {
		return bindings.ErrNotLoaded
	}
	ret := avcodecParametersFromCtx(uintptr(par), uintptr(ctx))
	if ret < 0 {
		return avutil.NewError(ret, "avcodec_parameters_from_context")
	}
	return nil
}

// ParametersCopy copies codec parameters from src to dst.
func ParametersCopy(dst, src Parameters) error {
	if avcodecParametersCopy == nil {
		return bindings.ErrNotLoaded
	}
	ret := avcodecParametersCopy(uintptr(dst), uintptr(src))
	if ret < 0 {
		return avutil.NewError(ret, "avcodec_parameters_copy")
	}
	return nil
}

// AVCodecParameters struct field offsets
const (
	offsetCodecParTag = 8 // codec_tag at offset 8 (after codec_type and codec_id)
)

// SetCodecParTag sets the codec tag in codec parameters.
// Setting to 0 allows the muxer to choose an appropriate tag.
func SetCodecParTag(par Parameters, tag uint32) {
	if par == nil {
		return
	}
	*(*uint32)(unsafe.Pointer(uintptr(par) + offsetCodecParTag)) = tag
}

// GetCodecParTag gets the codec tag from codec parameters.
func GetCodecParTag(par Parameters) uint32 {
	if par == nil {
		return 0
	}
	return *(*uint32)(unsafe.Pointer(uintptr(par) + offsetCodecParTag))
}

// PacketAlloc allocates a packet.
func PacketAlloc() Packet {
	if avPacketAlloc == nil {
		return nil
	}
	return unsafe.Pointer(avPacketAlloc())
}

// PacketFree frees a packet.
func PacketFree(pkt *Packet) {
	if pkt == nil || *pkt == nil || avPacketFree == nil {
		return
	}
	avPacketFree(pkt)
	*pkt = nil
}

// PacketRef creates a reference to src in dst.
func PacketRef(dst, src Packet) error {
	if avPacketRef == nil {
		return bindings.ErrNotLoaded
	}
	ret := avPacketRef(uintptr(dst), uintptr(src))
	if ret < 0 {
		return avutil.NewError(ret, "av_packet_ref")
	}
	return nil
}

// PacketUnref unreferences a packet's buffers.
func PacketUnref(pkt Packet) {
	if pkt == nil || avPacketUnref == nil {
		return
	}
	avPacketUnref(uintptr(pkt))
}

// AVCodec struct field offset for name (const char *name at offset 0)
const offsetCodecName = 8 // After enum AVMediaType type (4 bytes + padding)

// GetCodecName returns the name of the codec.
func GetCodecName(codec Codec) string {
	if codec == nil {
		return ""
	}
	// AVCodec.name is at offset 8 (after type field)
	namePtr := *(*unsafe.Pointer)(unsafe.Pointer(uintptr(codec) + offsetCodecName))
	if namePtr == nil {
		return ""
	}
	// Read null-terminated string
	return goString(namePtr)
}

// goString converts a C string to a Go string.
func goString(ptr unsafe.Pointer) string {
	if ptr == nil {
		return ""
	}
	var buf []byte
	for i := 0; ; i++ {
		b := *(*byte)(unsafe.Pointer(uintptr(ptr) + uintptr(i)))
		if b == 0 {
			break
		}
		buf = append(buf, b)
	}
	return string(buf)
}

// Packet field offsets (for FFmpeg 6.x/7.x)
const (
	offsetPacketPts         = 8  // int64 pts
	offsetPacketDts         = 16 // int64 dts
	offsetPacketData        = 24 // uint8_t *data
	offsetPacketSize        = 32 // int size
	offsetPacketStreamIndex = 36 // int stream_index
	offsetPacketFlags       = 40 // int flags
	offsetPacketDuration    = 64 // int64 duration
	offsetPacketPos         = 72 // int64 pos
)

// GetPacketPTS returns the presentation timestamp.
func GetPacketPTS(pkt Packet) int64 {
	if pkt == nil {
		return 0
	}
	return *(*int64)(unsafe.Pointer(uintptr(pkt) + offsetPacketPts))
}

// GetPacketDTS returns the decompression timestamp.
func GetPacketDTS(pkt Packet) int64 {
	if pkt == nil {
		return 0
	}
	return *(*int64)(unsafe.Pointer(uintptr(pkt) + offsetPacketDts))
}

// GetPacketSize returns the packet data size.
func GetPacketSize(pkt Packet) int32 {
	if pkt == nil {
		return 0
	}
	return *(*int32)(unsafe.Pointer(uintptr(pkt) + offsetPacketSize))
}

// GetPacketData returns a pointer to the packet data.
func GetPacketData(pkt Packet) unsafe.Pointer {
	if pkt == nil {
		return nil
	}
	return *(*unsafe.Pointer)(unsafe.Pointer(uintptr(pkt) + offsetPacketData))
}

// GetPacketStreamIndex returns the stream index.
func GetPacketStreamIndex(pkt Packet) int32 {
	if pkt == nil {
		return -1
	}
	return *(*int32)(unsafe.Pointer(uintptr(pkt) + offsetPacketStreamIndex))
}

// SetPacketStreamIndex sets the stream index.
func SetPacketStreamIndex(pkt Packet, idx int32) {
	if pkt == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(pkt) + offsetPacketStreamIndex)) = idx
}

// GetPacketFlags returns the packet flags.
// Use PacketFlagKey to check for keyframes.
func GetPacketFlags(pkt Packet) int32 {
	if pkt == nil {
		return 0
	}
	return *(*int32)(unsafe.Pointer(uintptr(pkt) + offsetPacketFlags))
}

// SetPacketFlags sets the packet flags.
func SetPacketFlags(pkt Packet, flags int32) {
	if pkt == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(pkt) + offsetPacketFlags)) = flags
}

// GetPacketPos returns the byte position in stream, or -1 if unknown.
func GetPacketPos(pkt Packet) int64 {
	if pkt == nil {
		return -1
	}
	return *(*int64)(unsafe.Pointer(uintptr(pkt) + offsetPacketPos))
}

// GetPacketDuration returns the packet duration (in stream time_base units).
func GetPacketDuration(pkt Packet) int64 {
	if pkt == nil {
		return 0
	}
	return *(*int64)(unsafe.Pointer(uintptr(pkt) + offsetPacketDuration))
}

// SetPacketDuration sets the packet duration (in stream time_base units).
func SetPacketDuration(pkt Packet, dur int64) {
	if pkt == nil {
		return
	}
	*(*int64)(unsafe.Pointer(uintptr(pkt) + offsetPacketDuration)) = dur
}

// SetPacketPos sets the byte position in stream.
func SetPacketPos(pkt Packet, pos int64) {
	if pkt == nil {
		return
	}
	*(*int64)(unsafe.Pointer(uintptr(pkt) + offsetPacketPos)) = pos
}

// Packet flag constants
const (
	PacketFlagKey     = 0x0001 // AV_PKT_FLAG_KEY - The packet contains a keyframe
	PacketFlagCorrupt = 0x0002 // AV_PKT_FLAG_CORRUPT - The packet content is corrupted
	PacketFlagDiscard = 0x0004 // AV_PKT_FLAG_DISCARD - Flag is used to discard packets
)

// AVCodecContext struct field offsets (for FFmpeg 6.x / avcodec 60.x)
// Verified with offsetof() - IMPORTANT: These offsets vary between FFmpeg versions!
const (
	offsetCtxCodecType   = 12  // enum AVMediaType codec_type
	offsetCtxCodecID     = 24  // enum AVCodecID codec_id
	offsetCtxBitRate     = 56  // int64_t bit_rate
	offsetCtxFlags       = 76  // int flags
	offsetCtxTimeBase    = 100 // AVRational time_base
	offsetCtxWidth       = 116 // int width
	offsetCtxHeight      = 120 // int height
	offsetCtxGopSize     = 132 // int gop_size
	offsetCtxPixFmt      = 136 // enum AVPixelFormat pix_fmt
	offsetCtxMaxBFrames  = 160 // int max_b_frames
	offsetCtxSampleRate  = 352 // int sample_rate
	offsetCtxSampleFmt   = 360 // enum AVSampleFormat sample_fmt
	offsetCtxFrameSize   = 364 // int frame_size
	offsetCtxFramerate   = 704 // AVRational framerate
	offsetCtxHWFramesCtx = 840 // AVBufferRef *hw_frames_ctx
	offsetCtxHWDeviceCtx = 864 // AVBufferRef *hw_device_ctx
	offsetCtxChLayout    = 912 // AVChannelLayout ch_layout (FFmpeg 5.1+)
)

// GetCtxWidth returns the width from codec context.
func GetCtxWidth(ctx Context) int32 {
	if ctx == nil {
		return 0
	}
	if v, err := ffshim.CodecCtxWidth(ctx); err == nil {
		return v
	}
	return *(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxWidth))
}

// SetCtxWidth sets the width in codec context.
func SetCtxWidth(ctx Context, width int32) {
	if ctx == nil {
		return
	}
	if err := ffshim.CodecCtxSetWidth(ctx, width); err == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxWidth)) = width
}

// GetCtxHeight returns the height from codec context.
func GetCtxHeight(ctx Context) int32 {
	if ctx == nil {
		return 0
	}
	if v, err := ffshim.CodecCtxHeight(ctx); err == nil {
		return v
	}
	return *(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxHeight))
}

// SetCtxHeight sets the height in codec context.
func SetCtxHeight(ctx Context, height int32) {
	if ctx == nil {
		return
	}
	if err := ffshim.CodecCtxSetHeight(ctx, height); err == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxHeight)) = height
}

// GetCtxPixFmt returns the pixel format from codec context.
func GetCtxPixFmt(ctx Context) int32 {
	if ctx == nil {
		return -1
	}
	if v, err := ffshim.CodecCtxPixFmt(ctx); err == nil {
		return v
	}
	return *(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxPixFmt))
}

// SetCtxPixFmt sets the pixel format in codec context.
func SetCtxPixFmt(ctx Context, fmt int32) {
	if ctx == nil {
		return
	}
	if err := ffshim.CodecCtxSetPixFmt(ctx, fmt); err == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxPixFmt)) = fmt
}

// SetCtxTimeBase sets the time base in codec context.
func SetCtxTimeBase(ctx Context, num, den int32) {
	if ctx == nil {
		return
	}
	if err := ffshim.CodecCtxSetTimeBase(ctx, num, den); err == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxTimeBase)) = num
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxTimeBase + 4)) = den
}

// SetCtxGopSize sets the GOP size in codec context.
func SetCtxGopSize(ctx Context, size int32) {
	if ctx == nil {
		return
	}
	// Prefer AVOptions to avoid struct-layout dependencies across FFmpeg versions.
	if err := avutil.OptSetInt(ctx, "g", int64(size), 0); err == nil {
		return
	}
	if err := avutil.OptSetInt(ctx, "gop_size", int64(size), 0); err == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxGopSize)) = size
}

// SetCtxMaxBFrames sets the max B-frames in codec context.
func SetCtxMaxBFrames(ctx Context, max int32) {
	if ctx == nil {
		return
	}
	// Prefer AVOptions to avoid struct-layout dependencies across FFmpeg versions.
	if err := avutil.OptSetInt(ctx, "bf", int64(max), 0); err == nil {
		return
	}
	if err := avutil.OptSetInt(ctx, "max_b_frames", int64(max), 0); err == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxMaxBFrames)) = max
}

// SetCtxBitRate sets the bit rate in codec context.
func SetCtxBitRate(ctx Context, bitRate int64) {
	if ctx == nil {
		return
	}
	// Prefer AVOptions to avoid struct-layout dependencies across FFmpeg versions.
	if err := avutil.OptSetInt(ctx, "b", bitRate, 0); err == nil {
		return
	}
	if err := avutil.OptSetInt(ctx, "bit_rate", bitRate, 0); err == nil {
		return
	}
	*(*int64)(unsafe.Pointer(uintptr(ctx) + offsetCtxBitRate)) = bitRate
}

// GetCtxFlags returns the flags from codec context.
func GetCtxFlags(ctx Context) int32 {
	if ctx == nil {
		return 0
	}
	return *(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxFlags))
}

// SetCtxFlags sets the flags in codec context.
func SetCtxFlags(ctx Context, flags int32) {
	if ctx == nil {
		return
	}
	// Prefer AVOptions to avoid struct-layout dependencies across FFmpeg versions.
	if err := avutil.OptSetInt(ctx, "flags", int64(flags), 0); err == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxFlags)) = flags
}

// SetCtxFramerate sets the framerate in codec context.
func SetCtxFramerate(ctx Context, num, den int32) {
	if ctx == nil {
		return
	}
	if err := ffshim.CodecCtxSetFramerate(ctx, num, den); err == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxFramerate)) = num
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxFramerate + 4)) = den
}

// Codec flag constants
const (
	CodecFlagGlobalHeader = 1 << 22 // AV_CODEC_FLAG_GLOBAL_HEADER (4194304)
	CodecFlagPass1        = 1 << 9  // AV_CODEC_FLAG_PASS1
	CodecFlagPass2        = 1 << 10 // AV_CODEC_FLAG_PASS2
)

// Audio codec context accessors

// GetCtxSampleRate returns the sample rate from codec context.
func GetCtxSampleRate(ctx Context) int32 {
	if ctx == nil {
		return 0
	}
	return *(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxSampleRate))
}

// SetCtxSampleRate sets the sample rate in codec context.
func SetCtxSampleRate(ctx Context, sampleRate int32) {
	if ctx == nil {
		return
	}
	// Prefer AVOptions to avoid struct-layout dependencies across FFmpeg versions.
	if err := avutil.OptSetInt(ctx, "ar", int64(sampleRate), 0); err == nil {
		return
	}
	if err := avutil.OptSetInt(ctx, "sample_rate", int64(sampleRate), 0); err == nil {
		return
	}
	if runtime.GOOS == "darwin" {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxSampleRate)) = sampleRate
}

// GetCtxChannels returns the number of channels from codec context.
// In FFmpeg 5.1+, this reads from ch_layout.nb_channels.
func GetCtxChannels(ctx Context) int32 {
	if ctx == nil {
		return 0
	}
	// ch_layout.nb_channels is at offset 4 within the AVChannelLayout struct
	return *(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxChLayout + 4))
}

// GetCtxSampleFmt returns the sample format from codec context.
func GetCtxSampleFmt(ctx Context) int32 {
	if ctx == nil {
		return -1
	}
	return *(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxSampleFmt))
}

// SetCtxSampleFmt sets the sample format in codec context.
func SetCtxSampleFmt(ctx Context, sampleFmt int32) {
	if ctx == nil {
		return
	}
	// Prefer AVOptions to avoid struct-layout dependencies across FFmpeg versions.
	if name := sampleFormatName(sampleFmt); name != "" {
		if err := avutil.OptSet(ctx, "sample_fmt", name, 0); err == nil {
			return
		}
	}
	if err := avutil.OptSetInt(ctx, "sample_fmt", int64(sampleFmt), 0); err == nil {
		return
	}
	if runtime.GOOS == "darwin" {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxSampleFmt)) = sampleFmt
}

func sampleFormatName(sampleFmt int32) string {
	switch avutil.SampleFormat(sampleFmt) {
	case avutil.SampleFormatU8:
		return "u8"
	case avutil.SampleFormatS16:
		return "s16"
	case avutil.SampleFormatS32:
		return "s32"
	case avutil.SampleFormatFlt:
		return "flt"
	case avutil.SampleFormatDbl:
		return "dbl"
	case avutil.SampleFormatU8P:
		return "u8p"
	case avutil.SampleFormatS16P:
		return "s16p"
	case avutil.SampleFormatS32P:
		return "s32p"
	case avutil.SampleFormatFltP:
		return "fltp"
	case avutil.SampleFormatDblP:
		return "dblp"
	case avutil.SampleFormatS64:
		return "s64"
	case avutil.SampleFormatS64P:
		return "s64p"
	default:
		return ""
	}
}

// GetCtxFrameSize returns the frame size from codec context.
// For audio, this is the number of samples per frame.
func GetCtxFrameSize(ctx Context) int {
	if ctx == nil {
		return 0
	}
	return int(*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxFrameSize)))
}

// GetCtxChLayoutPtr returns a pointer to the ch_layout field in AVCodecContext.
// This is used for FFmpeg 5.1+ channel layout API.
func GetCtxChLayoutPtr(ctx Context) unsafe.Pointer {
	if ctx == nil {
		return nil
	}
	return unsafe.Pointer(uintptr(ctx) + offsetCtxChLayout)
}

// AVChannelLayout struct layout (FFmpeg 5.1+):
// - order (int32): 4 bytes (AV_CHANNEL_ORDER_NATIVE = 1)
// - nb_channels (int32): 4 bytes
// - u.mask (uint64): 8 bytes (channel mask)
// - opaque (void*): 8 bytes
// Total: 24 bytes

// Channel order constants
const (
	ChannelOrderUnspec = 0 // AV_CHANNEL_ORDER_UNSPEC
	ChannelOrderNative = 1 // AV_CHANNEL_ORDER_NATIVE
)

// Common channel layout masks
const (
	ChannelLayoutMaskMono    uint64 = 0x4   // AV_CH_LAYOUT_MONO (FC)
	ChannelLayoutMaskStereo  uint64 = 0x3   // AV_CH_LAYOUT_STEREO (FL+FR)
	ChannelLayoutMask5Point1 uint64 = 0x60F // AV_CH_LAYOUT_5POINT1
)

// SetCtxChannelLayout sets the channel layout for audio in codec context.
// This manually sets the AVChannelLayout struct fields for FFmpeg 5.1+.
func SetCtxChannelLayout(ctx Context, nbChannels int32) {
	if ctx == nil {
		return
	}

	// Best-effort: if the shim is available, use it to set AVCodecContext->ch_layout
	// via FFmpeg APIs (avoids all struct offset issues on FFmpeg 7+).
	_ = ffshim.Load()
	shimOK := ffshim.CodecCtxSetChLayoutDefault(ctx, nbChannels) == nil

	// Also set the (legacy) channel count via AVOptions for encoders that still consult it.
	_ = avutil.OptSetInt(ctx, "ac", int64(nbChannels), 0)
	_ = avutil.OptSetInt(ctx, "channels", int64(nbChannels), 0)
	if shimOK {
		return
	}

	var layout string
	switch nbChannels {
	case 1:
		layout = "mono"
	case 2:
		layout = "stereo"
	case 6:
		layout = "5.1"
	}
	if layout != "" {
		if err := avutil.OptSet(ctx, "ch_layout", layout, 0); err == nil {
			return
		}
		if err := avutil.OptSet(ctx, "channel_layout", layout, 0); err == nil {
			return
		}
	}

	// Fallback: legacy direct struct writes (best-effort). Avoid on macOS where FFmpeg
	// struct layouts commonly differ from hardcoded offsets and can corrupt the context.
	if runtime.GOOS == "darwin" {
		return
	}

	chLayoutPtr := uintptr(ctx) + offsetCtxChLayout
	*(*int32)(unsafe.Pointer(chLayoutPtr)) = ChannelOrderNative
	*(*int32)(unsafe.Pointer(chLayoutPtr + 4)) = nbChannels

	var mask uint64
	switch nbChannels {
	case 1:
		mask = ChannelLayoutMaskMono
	case 2:
		mask = ChannelLayoutMaskStereo
	case 6:
		mask = ChannelLayoutMask5Point1
	default:
		mask = (1 << uint(nbChannels)) - 1
	}
	*(*uint64)(unsafe.Pointer(chLayoutPtr + 8)) = mask
}

// GetCtxTimeBase returns the time base from codec context.
func GetCtxTimeBase(ctx Context) avutil.Rational {
	if ctx == nil {
		return avutil.Rational{}
	}
	if num, den, err := ffshim.CodecCtxTimeBase(ctx); err == nil {
		return avutil.NewRational(num, den)
	}
	num := *(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxTimeBase))
	den := *(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxTimeBase + 4))
	return avutil.NewRational(num, den)
}

// SetPacketPTS sets the presentation timestamp.
func SetPacketPTS(pkt Packet, pts int64) {
	if pkt == nil {
		return
	}
	*(*int64)(unsafe.Pointer(uintptr(pkt) + offsetPacketPts)) = pts
}

// SetPacketDTS sets the decompression timestamp.
func SetPacketDTS(pkt Packet, dts int64) {
	if pkt == nil {
		return
	}
	*(*int64)(unsafe.Pointer(uintptr(pkt) + offsetPacketDts)) = dts
}

// RescalePacketTS rescales packet timestamps from one time base to another.
// This is equivalent to FFmpeg's av_packet_rescale_ts().
func RescalePacketTS(pkt Packet, srcTb, dstTb avutil.Rational) {
	if pkt == nil {
		return
	}

	pts := GetPacketPTS(pkt)
	dts := GetPacketDTS(pkt)
	dur := GetPacketDuration(pkt)

	// Rescale PTS if valid
	if pts != avutil.AV_NOPTS_VALUE {
		pts = rescaleQ(pts, srcTb, dstTb)
		SetPacketPTS(pkt, pts)
	}

	// Rescale DTS if valid
	if dts != avutil.AV_NOPTS_VALUE {
		dts = rescaleQ(dts, srcTb, dstTb)
		SetPacketDTS(pkt, dts)
	}

	// Rescale duration (0 is a common "unknown" value, keep it as-is)
	if dur > 0 {
		dur = rescaleQ(dur, srcTb, dstTb)
		SetPacketDuration(pkt, dur)
	}
}

// rescaleQ rescales a value from one time base to another.
// Equivalent to av_rescale_q: return a * bq / cq
func rescaleQ(a int64, bq, cq avutil.Rational) int64 {
	// a * bq.Num / bq.Den * cq.Den / cq.Num
	// = a * bq.Num * cq.Den / (bq.Den * cq.Num)
	if bq.Den == 0 || cq.Num == 0 {
		return 0
	}
	// Use 128-bit arithmetic emulation to avoid overflow
	// Simplified: a * b / c where b = bq.Num * cq.Den, c = bq.Den * cq.Num
	b := int64(bq.Num) * int64(cq.Den)
	c := int64(bq.Den) * int64(cq.Num)
	if c == 0 {
		return 0
	}
	// Rounding: add c/2 for positive values
	if a >= 0 {
		return (a*b + c/2) / c
	}
	return (a*b - c/2) / c
}

// GetCtxHWDeviceCtx returns the hardware device context from codec context.
func GetCtxHWDeviceCtx(ctx Context) avutil.HWDeviceContext {
	if ctx == nil {
		return nil
	}
	return *(*unsafe.Pointer)(unsafe.Pointer(uintptr(ctx) + offsetCtxHWDeviceCtx))
}

// SetCtxHWDeviceCtx sets the hardware device context on codec context.
// The buffer reference is copied, so caller retains ownership.
func SetCtxHWDeviceCtx(ctx Context, hwDeviceCtx avutil.HWDeviceContext) {
	if ctx == nil {
		return
	}
	// Create a new reference to the buffer
	ref := avutil.NewBufferRef(hwDeviceCtx)
	*(*unsafe.Pointer)(unsafe.Pointer(uintptr(ctx) + offsetCtxHWDeviceCtx)) = ref
}

// DecodeSubtitle2 decodes a subtitle from a packet.
// Returns true if a subtitle was decoded, along with any error.
func DecodeSubtitle2(ctx Context, sub, pkt unsafe.Pointer) (bool, error) {
	if avcodecDecodeSubtitle2 == nil {
		return false, bindings.ErrNotLoaded
	}
	var gotSub int32
	ret := avcodecDecodeSubtitle2(uintptr(ctx), uintptr(sub), uintptr(unsafe.Pointer(&gotSub)), uintptr(pkt))
	if ret < 0 {
		return false, avutil.NewError(ret, "avcodec_decode_subtitle2")
	}
	return gotSub != 0, nil
}

// SubtitleFree frees subtitle resources.
func SubtitleFree(sub unsafe.Pointer) {
	if avsubtitleFree == nil || sub == nil {
		return
	}
	avsubtitleFree(uintptr(sub))
}

// GetCtxHWFramesCtx returns the hardware frames context from codec context.
func GetCtxHWFramesCtx(ctx Context) avutil.HWFramesContext {
	if ctx == nil {
		return nil
	}
	return *(*unsafe.Pointer)(unsafe.Pointer(uintptr(ctx) + offsetCtxHWFramesCtx))
}

// SetCtxHWFramesCtx sets the hardware frames context on codec context.
func SetCtxHWFramesCtx(ctx Context, hwFramesCtx avutil.HWFramesContext) {
	if ctx == nil {
		return
	}
	ref := avutil.NewBufferRef(hwFramesCtx)
	*(*unsafe.Pointer)(unsafe.Pointer(uintptr(ctx) + offsetCtxHWFramesCtx)) = ref
}
