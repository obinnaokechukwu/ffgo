//go:build !ios && !android && (amd64 || arm64)

// Package swscale provides bindings to FFmpeg's libswscale library.
// It includes video scaling and pixel format conversion functionality.
package swscale

import (
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/obinnaokechukwu/ffgo/avutil"
	"github.com/obinnaokechukwu/ffgo/internal/bindings"
)

// Context is an opaque SwsContext pointer.
type Context = unsafe.Pointer

// Filter is an opaque SwsFilter pointer.
type Filter = unsafe.Pointer

// Scaling algorithm flags
const (
	FlagFastBilinear = 1    // Fast bilinear scaling
	FlagBilinear     = 2    // Bilinear scaling
	FlagBicubic      = 4    // Bicubic scaling
	FlagX            = 8    // Experimental
	FlagPoint        = 0x10 // Nearest neighbor (point sampling)
	FlagArea         = 0x20 // Area averaging
	FlagBicublin     = 0x40 // Luma bicubic, chroma bilinear
	FlagGauss        = 0x80 // Gaussian
	FlagSinc         = 0x100
	FlagLanczos      = 0x200 // Lanczos scaling
	FlagSpline       = 0x400 // Natural bicubic spline
)

// Function bindings
var (
	swsGetContext     func(srcW, srcH int32, srcFormat int32, dstW, dstH int32, dstFormat int32, flags int32, srcFilter, dstFilter, param unsafe.Pointer) unsafe.Pointer
	swsScale          func(ctx unsafe.Pointer, srcSlice, srcStride unsafe.Pointer, srcSliceY, srcSliceH int32, dst, dstStride unsafe.Pointer) int32
	swsFreeContext    func(ctx unsafe.Pointer)
	swsScaleFrame     func(ctx, dst, src unsafe.Pointer) int32
	swsFrameStart     func(ctx, dst, src unsafe.Pointer) int32
	swsFrameEnd       func(ctx unsafe.Pointer)
	swsIsSupportedIn  func(format int32) int32
	swsIsSupportedOut func(format int32) int32

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

	lib := bindings.LibSWScale()
	if lib == 0 {
		return
	}

	purego.RegisterLibFunc(&swsGetContext, lib, "sws_getContext")
	purego.RegisterLibFunc(&swsScale, lib, "sws_scale")
	purego.RegisterLibFunc(&swsFreeContext, lib, "sws_freeContext")

	// sws_scale_frame was added in FFmpeg 5.0
	purego.RegisterLibFunc(&swsScaleFrame, lib, "sws_scale_frame")
	purego.RegisterLibFunc(&swsFrameStart, lib, "sws_frame_start")
	purego.RegisterLibFunc(&swsFrameEnd, lib, "sws_frame_end")

	purego.RegisterLibFunc(&swsIsSupportedIn, lib, "sws_isSupportedInput")
	purego.RegisterLibFunc(&swsIsSupportedOut, lib, "sws_isSupportedOutput")

	bindingsRegistered = true
}

// GetContext creates a scaling context for the given parameters.
// srcW, srcH: source dimensions
// srcFormat: source pixel format (avutil.PixelFormat)
// dstW, dstH: destination dimensions
// dstFormat: destination pixel format (avutil.PixelFormat)
// flags: scaling algorithm flags (FlagBilinear, FlagBicubic, etc.)
// srcFilter, dstFilter: optional filters (can be nil)
// param: optional parameters (can be nil)
// Returns nil if the context cannot be created.
func GetContext(srcW, srcH int, srcFormat avutil.PixelFormat, dstW, dstH int, dstFormat avutil.PixelFormat, flags int32, srcFilter, dstFilter Filter, param unsafe.Pointer) Context {
	if swsGetContext == nil {
		return nil
	}
	return swsGetContext(
		int32(srcW), int32(srcH), int32(srcFormat),
		int32(dstW), int32(dstH), int32(dstFormat),
		flags,
		srcFilter, dstFilter, param,
	)
}

// FreeContext frees a scaling context.
// Safe to call with nil.
func FreeContext(ctx Context) {
	if ctx == nil || swsFreeContext == nil {
		return
	}
	swsFreeContext(ctx)
}

// Scale performs the scaling operation on raw data pointers.
// This is the low-level API; prefer ScaleFrame for frame-to-frame scaling.
// srcSlice: array of pointers to source planes
// srcStride: array of strides for source planes
// srcSliceY: starting Y position in source
// srcSliceH: height of the slice to convert
// dst: array of pointers to destination planes
// dstStride: array of strides for destination planes
// Returns the height of the output slice.
func Scale(ctx Context, srcSlice *[8]unsafe.Pointer, srcStride *[8]int32, srcSliceY, srcSliceH int32, dst *[8]unsafe.Pointer, dstStride *[8]int32) int32 {
	if ctx == nil || swsScale == nil {
		return -1
	}
	return swsScale(ctx,
		unsafe.Pointer(srcSlice), unsafe.Pointer(srcStride),
		srcSliceY, srcSliceH,
		unsafe.Pointer(dst), unsafe.Pointer(dstStride),
	)
}

// ScaleFrame scales from src frame to dst frame.
// Both frames must be allocated with proper format/dimensions.
// This is a convenience wrapper that was added in FFmpeg 5.0.
// Returns a negative error code on failure.
func ScaleFrame(ctx Context, dst, src avutil.Frame) int32 {
	if ctx == nil {
		return -1
	}

	// sws_scale_frame may not be available in older FFmpeg
	if swsScaleFrame != nil {
		return swsScaleFrame(ctx, dst, src)
	}

	// Fallback to sws_scale
	if swsScale == nil {
		return -1
	}

	// Get frame data
	srcData := avutil.GetFrameData(src)
	srcLinesize := avutil.GetFrameLinesize(src)
	dstData := avutil.GetFrameData(dst)
	dstLinesize := avutil.GetFrameLinesize(dst)
	srcH := avutil.GetFrameHeight(src)

	return swsScale(ctx,
		unsafe.Pointer(&srcData), unsafe.Pointer(&srcLinesize),
		0, srcH,
		unsafe.Pointer(&dstData), unsafe.Pointer(&dstLinesize),
	)
}

// IsSupportedInput returns true if the pixel format is supported as input.
func IsSupportedInput(format avutil.PixelFormat) bool {
	if swsIsSupportedIn == nil {
		return false
	}
	return swsIsSupportedIn(int32(format)) > 0
}

// IsSupportedOutput returns true if the pixel format is supported as output.
func IsSupportedOutput(format avutil.PixelFormat) bool {
	if swsIsSupportedOut == nil {
		return false
	}
	return swsIsSupportedOut(int32(format)) > 0
}
