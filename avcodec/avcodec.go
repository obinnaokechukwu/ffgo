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
	avcodecFindDecoder       func(id int32) unsafe.Pointer
	avcodecFindEncoder       func(id int32) unsafe.Pointer
	avcodecFindDecoderByName func(name string) unsafe.Pointer
	avcodecFindEncoderByName func(name string) unsafe.Pointer
	avcodecAllocContext3     func(codec unsafe.Pointer) unsafe.Pointer
	avcodecFreeContext       func(ctx *unsafe.Pointer)
	avcodecOpen2             func(ctx, codec unsafe.Pointer, options *unsafe.Pointer) int32
	avcodecClose             func(ctx unsafe.Pointer) int32
	avcodecSendPacket        func(ctx, pkt unsafe.Pointer) int32
	avcodecReceiveFrame      func(ctx, frame unsafe.Pointer) int32
	avcodecSendFrame         func(ctx, frame unsafe.Pointer) int32
	avcodecReceivePacket     func(ctx, pkt unsafe.Pointer) int32
	avcodecFlushBuffers      func(ctx unsafe.Pointer)
	avcodecParametersToCtx   func(ctx, par unsafe.Pointer) int32
	avcodecParametersFromCtx func(par, ctx unsafe.Pointer) int32

	avPacketAlloc func() unsafe.Pointer
	avPacketFree  func(pkt *unsafe.Pointer)
	avPacketRef   func(dst, src unsafe.Pointer) int32
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
	purego.RegisterLibFunc(&avcodecClose, lib, "avcodec_close")
	purego.RegisterLibFunc(&avcodecSendPacket, lib, "avcodec_send_packet")
	purego.RegisterLibFunc(&avcodecReceiveFrame, lib, "avcodec_receive_frame")
	purego.RegisterLibFunc(&avcodecSendFrame, lib, "avcodec_send_frame")
	purego.RegisterLibFunc(&avcodecReceivePacket, lib, "avcodec_receive_packet")
	purego.RegisterLibFunc(&avcodecFlushBuffers, lib, "avcodec_flush_buffers")
	purego.RegisterLibFunc(&avcodecParametersToCtx, lib, "avcodec_parameters_to_context")
	purego.RegisterLibFunc(&avcodecParametersFromCtx, lib, "avcodec_parameters_from_context")

	purego.RegisterLibFunc(&avPacketAlloc, lib, "av_packet_alloc")
	purego.RegisterLibFunc(&avPacketFree, lib, "av_packet_free")
	purego.RegisterLibFunc(&avPacketRef, lib, "av_packet_ref")
	purego.RegisterLibFunc(&avPacketUnref, lib, "av_packet_unref")

	bindingsRegistered = true
}

// FindDecoder finds a decoder by codec ID.
func FindDecoder(id CodecID) Codec {
	if avcodecFindDecoder == nil {
		return nil
	}
	return avcodecFindDecoder(int32(id))
}

// FindEncoder finds an encoder by codec ID.
func FindEncoder(id CodecID) Codec {
	if avcodecFindEncoder == nil {
		return nil
	}
	return avcodecFindEncoder(int32(id))
}

// FindDecoderByName finds a decoder by name.
func FindDecoderByName(name string) Codec {
	if avcodecFindDecoderByName == nil {
		return nil
	}
	codec := avcodecFindDecoderByName(name)
	runtime.KeepAlive(name)
	return codec
}

// FindEncoderByName finds an encoder by name.
func FindEncoderByName(name string) Codec {
	if avcodecFindEncoderByName == nil {
		return nil
	}
	codec := avcodecFindEncoderByName(name)
	runtime.KeepAlive(name)
	return codec
}

// AllocContext3 allocates a codec context.
func AllocContext3(codec Codec) Context {
	if avcodecAllocContext3 == nil {
		return nil
	}
	return avcodecAllocContext3(codec)
}

// FreeContext frees a codec context.
func FreeContext(ctx *Context) {
	if ctx == nil || *ctx == nil || avcodecFreeContext == nil {
		return
	}
	avcodecFreeContext(ctx)
	*ctx = nil
}

// Open2 opens a codec context.
func Open2(ctx Context, codec Codec, options *avutil.Dictionary) error {
	if avcodecOpen2 == nil {
		return bindings.ErrNotLoaded
	}
	ret := avcodecOpen2(ctx, codec, options)
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
	ret := avcodecClose(ctx)
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
	ret := avcodecSendPacket(ctx, pkt)
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
	ret := avcodecReceiveFrame(ctx, frame)
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
	ret := avcodecSendFrame(ctx, frame)
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
	ret := avcodecReceivePacket(ctx, pkt)
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
	avcodecFlushBuffers(ctx)
}

// ParametersToContext copies codec parameters to a context.
func ParametersToContext(ctx Context, par Parameters) error {
	if avcodecParametersToCtx == nil {
		return bindings.ErrNotLoaded
	}
	ret := avcodecParametersToCtx(ctx, par)
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
	ret := avcodecParametersFromCtx(par, ctx)
	if ret < 0 {
		return avutil.NewError(ret, "avcodec_parameters_from_context")
	}
	return nil
}

// PacketAlloc allocates a packet.
func PacketAlloc() Packet {
	if avPacketAlloc == nil {
		return nil
	}
	return avPacketAlloc()
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
	ret := avPacketRef(dst, src)
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
	avPacketUnref(pkt)
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

// Packet field offsets (for FFmpeg 6.x)
const (
	offsetPacketPts      = 8  // int64 pts
	offsetPacketDts      = 16 // int64 dts
	offsetPacketData     = 24 // uint8_t *data
	offsetPacketSize     = 32 // int size
	offsetPacketStreamIndex = 36 // int stream_index
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

// AVCodecContext struct field offsets (for FFmpeg 6.x / avcodec 60.x)
// Verified with offsetof()
const (
	offsetCtxCodecType  = 12  // enum AVMediaType codec_type
	offsetCtxCodecID    = 24  // enum AVCodecID codec_id
	offsetCtxBitRate    = 56  // int64_t bit_rate
	offsetCtxFlags      = 76  // int flags
	offsetCtxTimeBase   = 100 // AVRational time_base
	offsetCtxWidth      = 116 // int width
	offsetCtxHeight     = 120 // int height
	offsetCtxGopSize    = 132 // int gop_size
	offsetCtxPixFmt     = 136 // enum AVPixelFormat pix_fmt
	offsetCtxMaxBFrames = 160 // int max_b_frames
	offsetCtxFramerate  = 704 // AVRational framerate
)

// GetCtxWidth returns the width from codec context.
func GetCtxWidth(ctx Context) int32 {
	if ctx == nil {
		return 0
	}
	return *(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxWidth))
}

// SetCtxWidth sets the width in codec context.
func SetCtxWidth(ctx Context, width int32) {
	if ctx == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxWidth)) = width
}

// GetCtxHeight returns the height from codec context.
func GetCtxHeight(ctx Context) int32 {
	if ctx == nil {
		return 0
	}
	return *(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxHeight))
}

// SetCtxHeight sets the height in codec context.
func SetCtxHeight(ctx Context, height int32) {
	if ctx == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxHeight)) = height
}

// GetCtxPixFmt returns the pixel format from codec context.
func GetCtxPixFmt(ctx Context) int32 {
	if ctx == nil {
		return -1
	}
	return *(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxPixFmt))
}

// SetCtxPixFmt sets the pixel format in codec context.
func SetCtxPixFmt(ctx Context, fmt int32) {
	if ctx == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxPixFmt)) = fmt
}

// SetCtxTimeBase sets the time base in codec context.
func SetCtxTimeBase(ctx Context, num, den int32) {
	if ctx == nil {
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
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxGopSize)) = size
}

// SetCtxMaxBFrames sets the max B-frames in codec context.
func SetCtxMaxBFrames(ctx Context, max int32) {
	if ctx == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxMaxBFrames)) = max
}

// SetCtxBitRate sets the bit rate in codec context.
func SetCtxBitRate(ctx Context, bitRate int64) {
	if ctx == nil {
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
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxFlags)) = flags
}

// SetCtxFramerate sets the framerate in codec context.
func SetCtxFramerate(ctx Context, num, den int32) {
	if ctx == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxFramerate)) = num
	*(*int32)(unsafe.Pointer(uintptr(ctx) + offsetCtxFramerate + 4)) = den
}

// Codec flag constants
const (
	CodecFlagGlobalHeader = 1 << 22 // AV_CODEC_FLAG_GLOBAL_HEADER (4194304)
)
