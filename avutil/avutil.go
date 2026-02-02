//go:build !ios && !android && (amd64 || arm64)

// Package avutil provides bindings to FFmpeg's libavutil library.
// It includes frame management, error handling, dictionary operations,
// and rational number handling.
package avutil

import (
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/obinnaokechukwu/ffgo/internal/bindings"
)

// Frame is an opaque FFmpeg AVFrame pointer.
type Frame = unsafe.Pointer

// Dictionary is an opaque FFmpeg AVDictionary pointer.
type Dictionary = unsafe.Pointer

// AVBufferRef is an opaque FFmpeg AVBufferRef pointer.
type AVBufferRef = unsafe.Pointer

// HWDeviceContext is an opaque FFmpeg AVBufferRef for hardware device context.
type HWDeviceContext = unsafe.Pointer

// HWFramesContext is an opaque FFmpeg AVBufferRef for hardware frames context.
type HWFramesContext = unsafe.Pointer

// Function bindings - registered when init() is called
var (
	avFrameAlloc        func() uintptr
	avFrameFree         func(frame *unsafe.Pointer)
	avFrameRef          func(dst, src uintptr) int32
	avFrameUnref        func(frame uintptr)
	avFrameGetBuffer    func(frame uintptr, align int32) int32
	avFrameMakeWritable func(frame uintptr) int32

	avMalloc func(size uintptr) uintptr
	avFree   func(ptr uintptr)
	avFreep  func(ptr *unsafe.Pointer)

	avDictSet  func(pm *unsafe.Pointer, key, value string, flags int32) int32
	avDictGet  func(m uintptr, key string, prev uintptr, flags int32) uintptr
	avDictFree func(pm *unsafe.Pointer)

	avStrerror func(errnum int32, errbuf *byte, errbufSize uintptr) int32

	// Channel layout functions (FFmpeg 5.1+)
	avChannelLayoutDefault func(chLayout uintptr, nbChannels int32)
	avChannelLayoutCopy    func(dst, src uintptr) int32

	// AVOptions API (for setting codec options like preset, profile, etc.)
	avOptSet       func(obj uintptr, name, val string, searchFlags int32) int32
	avOptSetInt    func(obj uintptr, name string, val int64, searchFlags int32) int32
	avOptSetDouble func(obj uintptr, name string, val float64, searchFlags int32) int32

	// Hardware context functions
	avHWDeviceCtxCreate      func(deviceCtx *unsafe.Pointer, deviceType int32, device string, opts uintptr, flags int32) int32
	avHWDeviceFindTypeByName func(name string) int32
	avHWDeviceGetTypeName    func(deviceType int32) uintptr
	avHWFrameTransferData    func(dst, src uintptr, flags int32) int32

	// Buffer reference functions
	avBufferCreate func(data uintptr, size int32, freeCb uintptr, opaque uintptr, flags int32) uintptr
	avBufferRef    func(buf uintptr) uintptr
	avBufferUnref  func(buf *unsafe.Pointer)

	// Frame field accessors (using getter/setter pattern since we can't access struct fields)
	// We need to calculate offsets based on FFmpeg version
	bindingsRegistered bool
)

func init() {
	registerBindings()
}

func registerBindings() {
	if bindingsRegistered {
		return
	}

	// Ensure FFmpeg is loaded
	if err := bindings.Load(); err != nil {
		return // Will fail later when functions are called
	}

	lib := bindings.LibAVUtil()
	if lib == 0 {
		return
	}

	purego.RegisterLibFunc(&avFrameAlloc, lib, "av_frame_alloc")
	purego.RegisterLibFunc(&avFrameFree, lib, "av_frame_free")
	purego.RegisterLibFunc(&avFrameRef, lib, "av_frame_ref")
	purego.RegisterLibFunc(&avFrameUnref, lib, "av_frame_unref")
	purego.RegisterLibFunc(&avFrameGetBuffer, lib, "av_frame_get_buffer")
	purego.RegisterLibFunc(&avFrameMakeWritable, lib, "av_frame_make_writable")

	purego.RegisterLibFunc(&avMalloc, lib, "av_malloc")
	purego.RegisterLibFunc(&avFree, lib, "av_free")
	purego.RegisterLibFunc(&avFreep, lib, "av_freep")

	purego.RegisterLibFunc(&avDictSet, lib, "av_dict_set")
	purego.RegisterLibFunc(&avDictGet, lib, "av_dict_get")
	purego.RegisterLibFunc(&avDictFree, lib, "av_dict_free")

	purego.RegisterLibFunc(&avStrerror, lib, "av_strerror")

	// Channel layout functions (FFmpeg 5.1+)
	purego.RegisterLibFunc(&avChannelLayoutDefault, lib, "av_channel_layout_default")
	purego.RegisterLibFunc(&avChannelLayoutCopy, lib, "av_channel_layout_copy")

	// AVOptions API
	purego.RegisterLibFunc(&avOptSet, lib, "av_opt_set")
	purego.RegisterLibFunc(&avOptSetInt, lib, "av_opt_set_int")
	purego.RegisterLibFunc(&avOptSetDouble, lib, "av_opt_set_double")

	// Hardware context functions
	purego.RegisterLibFunc(&avHWDeviceCtxCreate, lib, "av_hwdevice_ctx_create")
	purego.RegisterLibFunc(&avHWDeviceFindTypeByName, lib, "av_hwdevice_find_type_by_name")
	purego.RegisterLibFunc(&avHWDeviceGetTypeName, lib, "av_hwdevice_get_type_name")
	purego.RegisterLibFunc(&avHWFrameTransferData, lib, "av_hwframe_transfer_data")

	// Buffer reference functions
	purego.RegisterLibFunc(&avBufferCreate, lib, "av_buffer_create")
	purego.RegisterLibFunc(&avBufferRef, lib, "av_buffer_ref")
	purego.RegisterLibFunc(&avBufferUnref, lib, "av_buffer_unref")

	bindingsRegistered = true
}

// FrameAlloc allocates an AVFrame and returns a pointer to it.
// The returned frame must be freed with FrameFree when no longer needed.
func FrameAlloc() Frame {
	if avFrameAlloc == nil {
		return nil
	}
	return unsafe.Pointer(avFrameAlloc())
}

// FrameFree frees an AVFrame and sets the pointer to nil.
// Safe to call with nil pointer.
func FrameFree(frame *Frame) {
	if frame == nil || *frame == nil || avFrameFree == nil {
		return
	}
	avFrameFree(frame)
	*frame = nil
}

// FrameRef creates a reference to src and stores it in dst.
// dst must be an allocated frame (via FrameAlloc).
func FrameRef(dst, src Frame) error {
	if avFrameRef == nil {
		return bindings.ErrNotLoaded
	}
	ret := avFrameRef(uintptr(dst), uintptr(src))
	if ret < 0 {
		return NewError(ret, "av_frame_ref")
	}
	return nil
}

// FrameUnref unreferences all buffers referenced by frame.
func FrameUnref(frame Frame) {
	if frame == nil || avFrameUnref == nil {
		return
	}
	avFrameUnref(uintptr(frame))
}

// FrameGetBufferErr allocates buffers for the frame based on its format/dimensions.
// The frame must have format, width, height set for video, or
// format, nb_samples, channel_layout set for audio.
// Returns an error if allocation fails.
func FrameGetBufferErr(frame Frame, align int32) error {
	if avFrameGetBuffer == nil {
		return bindings.ErrNotLoaded
	}
	ret := avFrameGetBuffer(uintptr(frame), align)
	if ret < 0 {
		return NewError(ret, "av_frame_get_buffer")
	}
	return nil
}

// FrameMakeWritable ensures the frame data is writable.
// If the frame is not writable, it will copy the data.
func FrameMakeWritable(frame Frame) error {
	if avFrameMakeWritable == nil {
		return bindings.ErrNotLoaded
	}
	ret := avFrameMakeWritable(uintptr(frame))
	if ret < 0 {
		return NewError(ret, "av_frame_make_writable")
	}
	return nil
}

// NoPTSValue is the value used to indicate no PTS.
const NoPTSValue int64 = -9223372036854775808 // 0x8000000000000000

// AV_NOPTS_VALUE is an alias for NoPTSValue (matches FFmpeg naming).
const AV_NOPTS_VALUE = NoPTSValue

// AVFrame struct field offsets (for FFmpeg 6.x / avutil 58.x)
// These are used to read/write frame properties without accessing struct fields directly
// Verified with offsetof() on FFmpeg 58.29.100
const (
	// Data pointer array offset
	offsetData = 0 // uint8_t *data[8] at offset 0

	// Linesize array offset
	offsetLinesize = 64 // int linesize[8] at offset 64

	// Video frame fields
	offsetWidth     = 104 // int width at offset 104
	offsetHeight    = 108 // int height at offset 108
	offsetNbSamples = 112 // int nb_samples at offset 112
	offsetFormat    = 116 // int format at offset 116

	// Key frame flag
	offsetKeyFrame = 120 // int key_frame at offset 120

	// Timing fields
	offsetPts = 136 // int64 pts at offset 136

	// Audio fields
	offsetSampleRate = 216 // int sample_rate at offset 216 (FFmpeg 6.x)
)

// GetFrameWidth returns the width of the frame.
func GetFrameWidth(frame Frame) int32 {
	if frame == nil {
		return 0
	}
	return *(*int32)(unsafe.Pointer(uintptr(frame) + offsetWidth))
}

// SetFrameWidth sets the width of the frame.
func SetFrameWidth(frame Frame, width int32) {
	if frame == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(frame) + offsetWidth)) = width
}

// GetFrameHeight returns the height of the frame.
func GetFrameHeight(frame Frame) int32 {
	if frame == nil {
		return 0
	}
	return *(*int32)(unsafe.Pointer(uintptr(frame) + offsetHeight))
}

// SetFrameHeight sets the height of the frame.
func SetFrameHeight(frame Frame, height int32) {
	if frame == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(frame) + offsetHeight)) = height
}

// GetFrameFormat returns the pixel format (video) or sample format (audio).
func GetFrameFormat(frame Frame) int32 {
	if frame == nil {
		return -1
	}
	return *(*int32)(unsafe.Pointer(uintptr(frame) + offsetFormat))
}

// SetFrameFormat sets the pixel format (video) or sample format (audio).
func SetFrameFormat(frame Frame, format int32) {
	if frame == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(frame) + offsetFormat)) = format
}

// GetFramePTS returns the presentation timestamp.
func GetFramePTS(frame Frame) int64 {
	if frame == nil {
		return NoPTSValue
	}
	return *(*int64)(unsafe.Pointer(uintptr(frame) + offsetPts))
}

// SetFramePTS sets the presentation timestamp.
func SetFramePTS(frame Frame, pts int64) {
	if frame == nil {
		return
	}
	*(*int64)(unsafe.Pointer(uintptr(frame) + offsetPts)) = pts
}

// GetFrameNbSamples returns the number of audio samples in this frame.
func GetFrameNbSamples(frame Frame) int32 {
	if frame == nil {
		return 0
	}
	return *(*int32)(unsafe.Pointer(uintptr(frame) + offsetNbSamples))
}

// SetFrameNbSamples sets the number of audio samples.
func SetFrameNbSamples(frame Frame, nbSamples int32) {
	if frame == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(frame) + offsetNbSamples)) = nbSamples
}

// GetFrameSampleRate returns the audio sample rate.
func GetFrameSampleRate(frame Frame) int32 {
	if frame == nil {
		return 0
	}
	return *(*int32)(unsafe.Pointer(uintptr(frame) + offsetSampleRate))
}

// SetFrameSampleRate sets the audio sample rate.
func SetFrameSampleRate(frame Frame, sampleRate int32) {
	if frame == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(frame) + offsetSampleRate)) = sampleRate
}

// FrameSetSampleRate is an alias for SetFrameSampleRate
func FrameSetSampleRate(frame Frame, sampleRate int32) {
	SetFrameSampleRate(frame, sampleRate)
}

// FrameSetChannels sets the number of audio channels in the frame.
// Note: In FFmpeg 5.1+, this should be done via AVChannelLayout, but we support legacy mode.
const offsetChannels = 148 // nb_channels in FFmpeg 5.x+ (via ch_layout.nb_channels)

func FrameSetChannels(frame Frame, channels int32) {
	if frame == nil {
		return
	}
	*(*int32)(unsafe.Pointer(uintptr(frame) + offsetChannels)) = channels
}

// GetFrameChannels returns the number of audio channels.
func GetFrameChannels(frame Frame) int32 {
	if frame == nil {
		return 0
	}
	return *(*int32)(unsafe.Pointer(uintptr(frame) + offsetChannels))
}

// FrameSetFormat is an alias for SetFrameFormat
func FrameSetFormat(frame Frame, format int32) {
	SetFrameFormat(frame, format)
}

// FrameSetNbSamples is an alias for SetFrameNbSamples
func FrameSetNbSamples(frame Frame, nbSamples int32) {
	SetFrameNbSamples(frame, nbSamples)
}

// FrameGetBuffer is the wrapper that returns int for compatibility
func FrameGetBuffer(frame Frame, align int32) int32 {
	if avFrameGetBuffer == nil {
		return -1
	}
	return avFrameGetBuffer(uintptr(frame), align)
}

// GetFrameKeyFrame returns 1 if this is a key frame, 0 otherwise.
func GetFrameKeyFrame(frame Frame) int32 {
	if frame == nil {
		return 0
	}
	return *(*int32)(unsafe.Pointer(uintptr(frame) + offsetKeyFrame))
}

// GetFrameLinesizePlane returns the linesize for a given plane.
func GetFrameLinesizePlane(frame Frame, plane int) int32 {
	if frame == nil || plane < 0 || plane >= 8 {
		return 0
	}
	linesizeArray := (*[8]int32)(unsafe.Pointer(uintptr(frame) + offsetLinesize))
	return linesizeArray[plane]
}

// GetFrameDataPlane returns the data pointer for a given plane.
func GetFrameDataPlane(frame Frame, plane int) unsafe.Pointer {
	if frame == nil || plane < 0 || plane >= 8 {
		return nil
	}
	dataArray := (*[8]unsafe.Pointer)(unsafe.Pointer(uintptr(frame) + offsetData))
	return dataArray[plane]
}

// GetFrameData returns pointers to all data planes.
func GetFrameData(frame Frame) [8]unsafe.Pointer {
	if frame == nil {
		return [8]unsafe.Pointer{}
	}
	return *(*[8]unsafe.Pointer)(unsafe.Pointer(uintptr(frame) + offsetData))
}

// GetFrameLinesize returns the linesizes for all planes.
func GetFrameLinesize(frame Frame) [8]int32 {
	if frame == nil {
		return [8]int32{}
	}
	linesizeArray := (*[8]int32)(unsafe.Pointer(uintptr(frame) + offsetLinesize))
	return *linesizeArray
}

// Malloc allocates memory using FFmpeg's allocator.
func Malloc(size uintptr) unsafe.Pointer {
	if avMalloc == nil {
		return nil
	}
	return unsafe.Pointer(avMalloc(size))
}

// Free frees memory allocated by Malloc.
func Free(ptr unsafe.Pointer) {
	if ptr == nil || avFree == nil {
		return
	}
	avFree(uintptr(ptr))
}

// DictSet sets a key-value pair in a dictionary.
func DictSet(dict *Dictionary, key, value string, flags int32) error {
	if avDictSet == nil {
		return bindings.ErrNotLoaded
	}
	ret := avDictSet(dict, key, value, flags)
	if ret < 0 {
		return NewError(ret, "av_dict_set")
	}
	return nil
}

// DictFree frees a dictionary.
func DictFree(dict *Dictionary) {
	if dict == nil || avDictFree == nil {
		return
	}
	avDictFree(dict)
}

// ErrorString returns a human-readable error message for an FFmpeg error code.
func ErrorString(errnum int32) string {
	if avStrerror == nil {
		return "unknown error (FFmpeg not loaded)"
	}

	bufArr := new([256]byte)
	avStrerror(errnum, &bufArr[0], 256)
	buf := bufArr[:]

	// Find null terminator
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

// ChannelLayoutDefault sets the default channel layout for the given number of channels.
// chLayout must be a pointer to an AVChannelLayout struct (e.g., embedded in AVCodecContext).
func ChannelLayoutDefault(chLayout unsafe.Pointer, nbChannels int32) {
	if avChannelLayoutDefault == nil || chLayout == nil {
		return
	}
	avChannelLayoutDefault(uintptr(chLayout), nbChannels)
}

// ChannelLayoutCopy copies a channel layout from src to dst.
func ChannelLayoutCopy(dst, src unsafe.Pointer) error {
	if avChannelLayoutCopy == nil {
		return nil
	}
	ret := avChannelLayoutCopy(uintptr(dst), uintptr(src))
	if ret < 0 {
		return NewError(ret, "av_channel_layout_copy")
	}
	return nil
}

// AV_OPT_SEARCH constants for av_opt_set functions
const (
	AV_OPT_SEARCH_CHILDREN = 1 << 0 // Search in child objects
	AV_OPT_SEARCH_FAKE_OBJ = 1 << 1 // Search in options on fake objects
)

// OptSet sets a string option value on an AVOptions-enabled struct.
// obj should be an AVCodecContext, AVFormatContext, or other AVOptions-enabled struct.
// Use AV_OPT_SEARCH_CHILDREN to search in child objects (e.g., private codec data).
func OptSet(obj unsafe.Pointer, name, val string, searchFlags int32) error {
	if avOptSet == nil {
		return bindings.ErrNotLoaded
	}
	if obj == nil {
		return NewError(-22, "av_opt_set: nil object")
	}
	ret := avOptSet(uintptr(obj), name, val, searchFlags)
	if ret < 0 {
		return NewError(ret, "av_opt_set")
	}
	return nil
}

// OptSetInt sets an integer option value on an AVOptions-enabled struct.
func OptSetInt(obj unsafe.Pointer, name string, val int64, searchFlags int32) error {
	if avOptSetInt == nil {
		return bindings.ErrNotLoaded
	}
	if obj == nil {
		return NewError(-22, "av_opt_set_int: nil object")
	}
	ret := avOptSetInt(uintptr(obj), name, val, searchFlags)
	if ret < 0 {
		return NewError(ret, "av_opt_set_int")
	}
	return nil
}

// OptSetDouble sets a double option value on an AVOptions-enabled struct.
func OptSetDouble(obj unsafe.Pointer, name string, val float64, searchFlags int32) error {
	if avOptSetDouble == nil {
		return bindings.ErrNotLoaded
	}
	if obj == nil {
		return NewError(-22, "av_opt_set_double: nil object")
	}
	ret := avOptSetDouble(uintptr(obj), name, val, searchFlags)
	if ret < 0 {
		return NewError(ret, "av_opt_set_double")
	}
	return nil
}

// HWDeviceType represents a hardware accelerator type.
type HWDeviceType int32

// Hardware device type constants
const (
	HWDeviceTypeNone         HWDeviceType = 0
	HWDeviceTypeVDPAU        HWDeviceType = 1
	HWDeviceTypeCUDA         HWDeviceType = 2
	HWDeviceTypeVAAPI        HWDeviceType = 3
	HWDeviceTypeDXVA2        HWDeviceType = 4
	HWDeviceTypeQSV          HWDeviceType = 5
	HWDeviceTypeVideoToolbox HWDeviceType = 6
	HWDeviceTypeD3D11VA      HWDeviceType = 7
	HWDeviceTypeDRM          HWDeviceType = 8
	HWDeviceTypeOpenCL       HWDeviceType = 9
	HWDeviceTypeMediaCodec   HWDeviceType = 10
	HWDeviceTypeVulkan       HWDeviceType = 11
)

// HWDeviceCtxCreate creates a hardware device context.
// deviceType is the type of hardware accelerator (e.g., HWDeviceTypeVAAPI).
// device is an optional device identifier (e.g., "/dev/dri/renderD128" for VAAPI).
// Returns the device context (must be freed with BufferUnref) or error.
func HWDeviceCtxCreate(deviceType HWDeviceType, device string) (HWDeviceContext, error) {
	if avHWDeviceCtxCreate == nil {
		return nil, bindings.ErrNotLoaded
	}
	var ctx unsafe.Pointer
	ret := avHWDeviceCtxCreate(&ctx, int32(deviceType), device, 0, 0)
	if ret < 0 {
		return nil, NewError(ret, "av_hwdevice_ctx_create")
	}
	return ctx, nil
}

// HWDeviceFindTypeByName returns the hardware device type for the given name.
// Returns HWDeviceTypeNone if the type is not found.
func HWDeviceFindTypeByName(name string) HWDeviceType {
	if avHWDeviceFindTypeByName == nil {
		return HWDeviceTypeNone
	}
	return HWDeviceType(avHWDeviceFindTypeByName(name))
}

// HWDeviceGetTypeName returns the name of the hardware device type.
func HWDeviceGetTypeName(deviceType HWDeviceType) string {
	if avHWDeviceGetTypeName == nil {
		return ""
	}
	ptr := unsafe.Pointer(avHWDeviceGetTypeName(int32(deviceType)))
	if ptr == nil {
		return ""
	}
	return goString(ptr)
}

// HWFrameTransferData copies data from a hardware frame to a software frame.
// dst should be a software frame, src should be a hardware frame.
func HWFrameTransferData(dst, src Frame, flags int32) error {
	if avHWFrameTransferData == nil {
		return bindings.ErrNotLoaded
	}
	ret := avHWFrameTransferData(uintptr(dst), uintptr(src), flags)
	if ret < 0 {
		return NewError(ret, "av_hwframe_transfer_data")
	}
	return nil
}

// BufferCreate wraps av_buffer_create.
//
// freeCb is a purego callback pointer for: void free(void *opaque, uint8_t *data).
// opaque is passed through to the callback when the buffer is released.
func BufferCreate(data unsafe.Pointer, size int, freeCb uintptr, opaque unsafe.Pointer, flags int32) AVBufferRef {
	if avBufferCreate == nil || data == nil || size <= 0 {
		return nil
	}
	return unsafe.Pointer(avBufferCreate(uintptr(data), int32(size), freeCb, uintptr(opaque), flags))
}

// NewBufferRef creates a new reference to a buffer.
func NewBufferRef(buf AVBufferRef) AVBufferRef {
	if avBufferRef == nil || buf == nil {
		return nil
	}
	return unsafe.Pointer(avBufferRef(uintptr(buf)))
}

// FreeBufferRef unreferences a buffer and sets the pointer to nil.
func FreeBufferRef(buf *AVBufferRef) {
	if avBufferUnref == nil || buf == nil || *buf == nil {
		return
	}
	avBufferUnref(buf)
	*buf = nil
}

// goString converts a C string to a Go string.
func goString(ptr unsafe.Pointer) string {
	if ptr == nil {
		return ""
	}
	var length int
	for {
		b := *(*byte)(unsafe.Pointer(uintptr(ptr) + uintptr(length)))
		if b == 0 {
			break
		}
		length++
		if length > 256 {
			break
		}
	}
	if length == 0 {
		return ""
	}
	return string((*[256]byte)(ptr)[:length:length])
}
