//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"sync"
	"unsafe"

	"github.com/obinnaokechukwu/ffgo/internal/shim"
)

type ColorRange int32
type ColorSpace int32
type ColorPrimaries int32
type ColorTransfer int32

// Common FFmpeg values (AVColorRange). Kept in sync with libavutil/pixfmt.h.
const (
	ColorRangeUnspecified ColorRange = 0
	ColorRangeMPEG        ColorRange = 1 // limited (16-235)
	ColorRangeJPEG        ColorRange = 2 // full (0-255)
)

// ColorSpec describes color metadata attached to a frame.
type ColorSpec struct {
	Range     ColorRange
	Space     ColorSpace
	Primaries ColorPrimaries
	Transfer  ColorTransfer
}

var (
	colorOffOnce sync.Once
	colorOffOK   bool
	offRange     int32
	offSpace     int32
	offPrim      int32
	offTrc       int32
)

func ensureColorOffsets() {
	colorOffOnce.Do(func() {
		_ = shim.Load()
		r, s, p, t, err := shim.AVFrameColorOffsets()
		if err != nil {
			colorOffOK = false
			return
		}
		offRange, offSpace, offPrim, offTrc = r, s, p, t
		colorOffOK = true
	})
}

// ColorSpec returns the frame's color metadata. If the shim does not provide
// AVFrame color offsets, it returns a zero-value ColorSpec.
func (f Frame) ColorSpec() ColorSpec {
	if f.ptr == nil {
		return ColorSpec{}
	}
	ensureColorOffsets()
	if !colorOffOK {
		return ColorSpec{}
	}
	return ColorSpec{
		Range:     ColorRange(*(*int32)(unsafe.Add(f.ptr, offRange))),
		Space:     ColorSpace(*(*int32)(unsafe.Add(f.ptr, offSpace))),
		Primaries: ColorPrimaries(*(*int32)(unsafe.Add(f.ptr, offPrim))),
		Transfer:  ColorTransfer(*(*int32)(unsafe.Add(f.ptr, offTrc))),
	}
}

// SetColorSpec sets the frame's color metadata. If the shim does not provide
// AVFrame color offsets, this is a no-op.
func (f Frame) SetColorSpec(spec ColorSpec) {
	if f.ptr == nil {
		return
	}
	ensureColorOffsets()
	if !colorOffOK {
		return
	}
	*(*int32)(unsafe.Add(f.ptr, offRange)) = int32(spec.Range)
	*(*int32)(unsafe.Add(f.ptr, offSpace)) = int32(spec.Space)
	*(*int32)(unsafe.Add(f.ptr, offPrim)) = int32(spec.Primaries)
	*(*int32)(unsafe.Add(f.ptr, offTrc)) = int32(spec.Transfer)
}

func colorOffsetsAvailable() bool {
	ensureColorOffsets()
	return colorOffOK
}

