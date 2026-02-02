// Package swresample provides audio resampling and format conversion using FFmpeg's libswresample.
package swresample

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/obinnaokechukwu/ffgo/internal/bindings"
)

// SwrContext is an opaque audio resampling context
type SwrContext = unsafe.Pointer

var (
	libSWResample uintptr
	initOnce      sync.Once
	initErr       error
)

// Function bindings
var (
	swr_alloc           func() uintptr
	swr_init            func(s uintptr) int32
	swr_free            func(s *SwrContext)
	swr_convert         func(s, out, in uintptr, outCount, inCount int32) int32
	swr_convert_frame   func(s, output, input uintptr) int32
	swr_get_delay       func(s uintptr, base int64) int64
	swr_get_out_samples func(s uintptr, inSamples int32) int32
	swr_is_initialized  func(s uintptr) int32
	swr_close           func(s uintptr)

	// For FFmpeg 5.1+ with AVChannelLayout
	swr_alloc_set_opts2 func(ps *SwrContext,
		outChLayout uintptr, outFmt, outRate int32,
		inChLayout uintptr, inFmt, inRate int32,
		logOffset int32, logCtx uintptr) int32

	// Legacy API for older FFmpeg
	swr_alloc_set_opts func(s uintptr,
		outChLayout int64, outFmt, outRate int32,
		inChLayout int64, inFmt, inRate int32,
		logOffset int32, logCtx uintptr) uintptr
)

// Init initializes the swresample library bindings
func Init() error {
	initOnce.Do(func() {
		initErr = initLibrary()
	})
	return initErr
}

func initLibrary() error {
	var err error
	libSWResample, err = bindings.LoadLibrary("swresample", []int{5, 4, 3})
	if err != nil {
		return fmt.Errorf("swresample: failed to load library: %w", err)
	}

	// Bind required functions
	purego.RegisterLibFunc(&swr_alloc, libSWResample, "swr_alloc")
	purego.RegisterLibFunc(&swr_init, libSWResample, "swr_init")
	purego.RegisterLibFunc(&swr_free, libSWResample, "swr_free")
	purego.RegisterLibFunc(&swr_convert, libSWResample, "swr_convert")
	purego.RegisterLibFunc(&swr_convert_frame, libSWResample, "swr_convert_frame")
	purego.RegisterLibFunc(&swr_get_delay, libSWResample, "swr_get_delay")
	purego.RegisterLibFunc(&swr_get_out_samples, libSWResample, "swr_get_out_samples")
	purego.RegisterLibFunc(&swr_is_initialized, libSWResample, "swr_is_initialized")
	purego.RegisterLibFunc(&swr_close, libSWResample, "swr_close")

	// Try to bind FFmpeg 5.1+ API first
	registerOptionalLibFunc(&swr_alloc_set_opts2, libSWResample, "swr_alloc_set_opts2")

	// Fallback to legacy API
	registerOptionalLibFunc(&swr_alloc_set_opts, libSWResample, "swr_alloc_set_opts")

	return nil
}

func registerOptionalLibFunc(fptr any, handle uintptr, name string) {
	defer func() { _ = recover() }()
	purego.RegisterLibFunc(fptr, handle, name)
}

// Alloc allocates a new SwrContext
func Alloc() SwrContext {
	if err := Init(); err != nil {
		return nil
	}
	return unsafe.Pointer(swr_alloc())
}

// AllocSetOpts allocates and configures a SwrContext with the given parameters
// Uses legacy channel layout (uint64 bitmask)
func AllocSetOpts(s SwrContext, outChLayout int64, outSampleFmt, outSampleRate int32,
	inChLayout int64, inSampleFmt, inSampleRate int32) SwrContext {
	if err := Init(); err != nil {
		return nil
	}
	if swr_alloc_set_opts == nil {
		return nil
	}
	return unsafe.Pointer(swr_alloc_set_opts(uintptr(s), outChLayout, outSampleFmt, outSampleRate,
		inChLayout, inSampleFmt, inSampleRate, 0, 0))
}

// AllocSetOpts2 allocates and configures a SwrContext with AVChannelLayout (FFmpeg 5.1+)
// outChLayout and inChLayout should be pointers to AVChannelLayout structs
func AllocSetOpts2(ps *SwrContext, outChLayout, inChLayout unsafe.Pointer,
	outSampleFmt, inSampleFmt int32, outSampleRate, inSampleRate int32) error {
	if err := Init(); err != nil {
		return err
	}
	if swr_alloc_set_opts2 == nil {
		return fmt.Errorf("swr_alloc_set_opts2 not available (FFmpeg < 5.1)")
	}
	ret := swr_alloc_set_opts2(ps,
		uintptr(outChLayout), outSampleFmt, outSampleRate,
		uintptr(inChLayout), inSampleFmt, inSampleRate,
		0, 0)
	if ret < 0 {
		return fmt.Errorf("swr_alloc_set_opts2 failed: %d", ret)
	}
	return nil
}

// InitContext initializes a SwrContext after options have been set
func InitContext(s SwrContext) error {
	if err := Init(); err != nil {
		return err
	}
	ret := swr_init(uintptr(s))
	if ret < 0 {
		return fmt.Errorf("swr_init failed: %d", ret)
	}
	return nil
}

// Free releases a SwrContext
func Free(s *SwrContext) {
	if s == nil || *s == nil {
		return
	}
	if err := Init(); err != nil {
		return
	}
	swr_free(s)
}

// Convert resamples audio data
// out and in should be pointers to arrays of channel data pointers
// Returns number of samples output per channel, or negative on error
func Convert(s SwrContext, out unsafe.Pointer, outCount int32,
	in unsafe.Pointer, inCount int32) (int, error) {
	if err := Init(); err != nil {
		return 0, err
	}
	ret := swr_convert(uintptr(s), uintptr(out), uintptr(in), outCount, inCount)
	runtime.KeepAlive(out)
	runtime.KeepAlive(in)
	if ret < 0 {
		return 0, fmt.Errorf("swr_convert failed: %d", ret)
	}
	return int(ret), nil
}

// ConvertFrame resamples an entire AVFrame
// output and input should be pointers to AVFrame structs
func ConvertFrame(s SwrContext, output, input unsafe.Pointer) error {
	if err := Init(); err != nil {
		return err
	}
	ret := swr_convert_frame(uintptr(s), uintptr(output), uintptr(input))
	runtime.KeepAlive(output)
	runtime.KeepAlive(input)
	if ret < 0 {
		return fmt.Errorf("swr_convert_frame failed: %d", ret)
	}
	return nil
}

// GetDelay returns the delay (in samples at the given sample rate) introduced by the resampler
func GetDelay(s SwrContext, base int64) int64 {
	if err := Init(); err != nil {
		return 0
	}
	return swr_get_delay(uintptr(s), base)
}

// GetOutSamples estimates the number of output samples for a given number of input samples
func GetOutSamples(s SwrContext, inSamples int) int {
	if err := Init(); err != nil {
		return 0
	}
	return int(swr_get_out_samples(uintptr(s), int32(inSamples)))
}

// IsInitialized returns true if the context is initialized
func IsInitialized(s SwrContext) bool {
	if err := Init(); err != nil {
		return false
	}
	return swr_is_initialized(uintptr(s)) != 0
}

// Close closes the context, but does not free it
func Close(s SwrContext) {
	if err := Init(); err != nil {
		return
	}
	swr_close(uintptr(s))
}

// Sample format constants (match AVSampleFormat)
const (
	SampleFormatNone int32 = -1
	SampleFormatU8   int32 = 0
	SampleFormatS16  int32 = 1
	SampleFormatS32  int32 = 2
	SampleFormatFLT  int32 = 3
	SampleFormatDBL  int32 = 4
	SampleFormatU8P  int32 = 5
	SampleFormatS16P int32 = 6
	SampleFormatS32P int32 = 7
	SampleFormatFLTP int32 = 8
	SampleFormatDBLP int32 = 9
	SampleFormatS64  int32 = 10
	SampleFormatS64P int32 = 11
)

// Channel layout constants (legacy uint64 bitmask format)
const (
	ChannelLayoutMono        int64 = 0x4   // AV_CH_FRONT_CENTER
	ChannelLayoutStereo      int64 = 0x3   // AV_CH_FRONT_LEFT | AV_CH_FRONT_RIGHT
	ChannelLayout2Point1     int64 = 0xB   // AV_CH_LAYOUT_2POINT1
	ChannelLayoutSurround    int64 = 0x7   // AV_CH_LAYOUT_SURROUND
	ChannelLayout4Point0     int64 = 0x107 // AV_CH_LAYOUT_4POINT0
	ChannelLayout5Point0     int64 = 0x607 // AV_CH_LAYOUT_5POINT0
	ChannelLayout5Point1     int64 = 0x60F // AV_CH_LAYOUT_5POINT1
	ChannelLayout6Point1     int64 = 0x70F // AV_CH_LAYOUT_6POINT1
	ChannelLayout7Point1     int64 = 0x63F // AV_CH_LAYOUT_7POINT1
	ChannelLayout7Point1Wide int64 = 0xFF  // AV_CH_LAYOUT_7POINT1_WIDE
)
