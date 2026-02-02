//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"testing"
	"unsafe"

	"github.com/obinnaokechukwu/ffgo/swscale"
)

func TestScalerSetColorspace(t *testing.T) {
	if !requireFFmpeg(t) {
		return
	}
	if !swscale.HasColorspaceDetails() || swscale.GetCoefficients(1) == nil {
		t.Log("swscale colorspace APIs not available in this FFmpeg build")
		return
	}

	s, err := NewScaler(16, 16, PixelFormatYUV420P, 16, 16, PixelFormatYUV420P, ScaleBilinear)
	if err != nil {
		t.Fatalf("NewScaler failed: %v", err)
	}
	defer s.Close()

	if err := s.SetColorspace(ColorSpaceBT709, ColorSpaceBT2020NCL); err != nil {
		t.Fatalf("SetColorspace failed: %v", err)
	}

	var invTable unsafe.Pointer
	var table unsafe.Pointer
	var srcRange int32
	var dstRange int32
	var brightness, contrast, saturation int32
	ret := swscale.GetColorspaceDetails(s.ctx, &invTable, &srcRange, &table, &dstRange, &brightness, &contrast, &saturation)
	if ret < 0 {
		t.Fatalf("GetColorspaceDetails failed: %d", ret)
	}

	if want := swscale.GetCoefficients(toSwsColorspace(ColorSpaceBT709)); invTable != want {
		// Some swscale builds return a different pointer than sws_getCoefficients, but the contents should match.
		got := readSwsCoeffs(invTable)
		exp := readSwsCoeffs(want)
		if got != exp {
			t.Fatalf("unexpected invTable coeffs: got=%v want=%v (ptr got=%p want=%p)", got, exp, invTable, want)
		}
	}
	if want := swscale.GetCoefficients(toSwsColorspace(ColorSpaceBT2020NCL)); table != want {
		got := readSwsCoeffs(table)
		exp := readSwsCoeffs(want)
		if got != exp {
			t.Fatalf("unexpected table coeffs: got=%v want=%v (ptr got=%p want=%p)", got, exp, table, want)
		}
	}
}

func TestToSwsColorspace_BT601Mapping(t *testing.T) {
	if got := toSwsColorspace(ColorSpaceBT601); got != 5 {
		t.Fatalf("expected BT.601 to map to 5, got %d", got)
	}
}

func readSwsCoeffs(ptr unsafe.Pointer) [4]int32 {
	if ptr == nil {
		return [4]int32{}
	}
	s := unsafe.Slice((*int32)(ptr), 4)
	return [4]int32{s[0], s[1], s[2], s[3]}
}
