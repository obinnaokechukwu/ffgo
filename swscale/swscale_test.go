//go:build !ios && !android && (amd64 || arm64)

package swscale

import (
	"testing"

	"github.com/obinnaokechukwu/ffgo/avutil"
	"github.com/obinnaokechukwu/ffgo/internal/bindings"
)

func init() {
	if err := bindings.Load(); err != nil {
		panic("Failed to load FFmpeg: " + err.Error())
	}
}

func TestGetContext(t *testing.T) {
	// Create a scaling context: 1920x1080 YUV420P -> 1280x720 RGB24
	ctx := GetContext(
		1920, 1080, avutil.PixelFormatYUV420P,
		1280, 720, avutil.PixelFormatRGB24,
		FlagBilinear, nil, nil, nil,
	)
	if ctx == nil {
		t.Fatal("GetContext returned nil")
	}
	defer FreeContext(ctx)
}

func TestGetContextSameSize(t *testing.T) {
	// Create a context for pixel format conversion only (no scaling)
	ctx := GetContext(
		640, 480, avutil.PixelFormatYUV420P,
		640, 480, avutil.PixelFormatRGB24,
		FlagBilinear, nil, nil, nil,
	)
	if ctx == nil {
		t.Fatal("GetContext returned nil for same-size conversion")
	}
	defer FreeContext(ctx)
}

func TestFreeContext(t *testing.T) {
	ctx := GetContext(
		320, 240, avutil.PixelFormatYUV420P,
		320, 240, avutil.PixelFormatRGB24,
		FlagBilinear, nil, nil, nil,
	)
	if ctx == nil {
		t.Skip("GetContext returned nil")
	}

	// Free should not panic
	FreeContext(ctx)

	// Free nil should not panic
	FreeContext(nil)
}

func TestScaleFlags(t *testing.T) {
	testCases := []struct {
		name  string
		flags int32
	}{
		{"FastBilinear", FlagFastBilinear},
		{"Bilinear", FlagBilinear},
		{"Bicubic", FlagBicubic},
		{"Lanczos", FlagLanczos},
		{"Point", FlagPoint},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := GetContext(
				640, 480, avutil.PixelFormatYUV420P,
				320, 240, avutil.PixelFormatRGB24,
				tc.flags, nil, nil, nil,
			)
			if ctx == nil {
				t.Errorf("GetContext with %s flag returned nil", tc.name)
				return
			}
			FreeContext(ctx)
		})
	}
}

func TestScaleWithFrames(t *testing.T) {
	// Allocate source frame
	srcFrame := avutil.FrameAlloc()
	if srcFrame == nil {
		t.Fatal("Failed to allocate source frame")
	}
	defer avutil.FrameFree(&srcFrame)

	// Set source frame properties
	avutil.SetFrameWidth(srcFrame, 320)
	avutil.SetFrameHeight(srcFrame, 240)
	avutil.SetFrameFormat(srcFrame, int32(avutil.PixelFormatYUV420P))

	// Allocate buffer for source frame
	err := avutil.FrameGetBufferErr(srcFrame, 0)
	if err != nil {
		t.Fatalf("Failed to allocate source frame buffer: %v", err)
	}

	// Make writable
	err = avutil.FrameMakeWritable(srcFrame)
	if err != nil {
		t.Fatalf("Failed to make source frame writable: %v", err)
	}

	// Fill with test pattern (simple gradient in Y plane)
	fillTestPattern(srcFrame)

	// Allocate destination frame
	dstFrame := avutil.FrameAlloc()
	if dstFrame == nil {
		t.Fatal("Failed to allocate destination frame")
	}
	defer avutil.FrameFree(&dstFrame)

	// Set destination frame properties
	avutil.SetFrameWidth(dstFrame, 160)
	avutil.SetFrameHeight(dstFrame, 120)
	avutil.SetFrameFormat(dstFrame, int32(avutil.PixelFormatRGB24))

	// Allocate buffer for destination frame
	err = avutil.FrameGetBufferErr(dstFrame, 0)
	if err != nil {
		t.Fatalf("Failed to allocate destination frame buffer: %v", err)
	}

	// Create scaling context
	ctx := GetContext(
		320, 240, avutil.PixelFormatYUV420P,
		160, 120, avutil.PixelFormatRGB24,
		FlagBilinear, nil, nil, nil,
	)
	if ctx == nil {
		t.Fatal("GetContext returned nil")
	}
	defer FreeContext(ctx)

	// Scale the frame
	ret := ScaleFrame(ctx, dstFrame, srcFrame)
	if ret < 0 {
		t.Errorf("ScaleFrame returned %d", ret)
	}

	t.Logf("Scaled frame: 320x240 YUV420P -> 160x120 RGB24, returned %d", ret)
}

func fillTestPattern(frame avutil.Frame) {
	width := int(avutil.GetFrameWidth(frame))
	height := int(avutil.GetFrameHeight(frame))
	data := avutil.GetFrameData(frame)
	linesize := avutil.GetFrameLinesize(frame)

	if data[0] == nil || linesize[0] == 0 {
		return
	}

	// Fill Y plane with gradient
	yPlane := (*[1 << 30]byte)(data[0])[:height*int(linesize[0])]
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			yPlane[y*int(linesize[0])+x] = byte((x + y) % 256)
		}
	}

	// Fill U and V planes with middle gray (128)
	chromaHeight := height / 2

	if data[1] != nil && linesize[1] > 0 {
		uPlane := (*[1 << 30]byte)(data[1])[:chromaHeight*int(linesize[1])]
		for i := range uPlane {
			uPlane[i] = 128
		}
	}

	if data[2] != nil && linesize[2] > 0 {
		vPlane := (*[1 << 30]byte)(data[2])[:chromaHeight*int(linesize[2])]
		for i := range vPlane {
			vPlane[i] = 128
		}
	}
}

func TestVersion(t *testing.T) {
	ver := bindings.SWScaleVersion()
	if ver == 0 {
		t.Error("SWScaleVersion returned 0")
	}
	t.Logf("swscale version: %d.%d.%d", ver>>16, (ver>>8)&0xFF, ver&0xFF)
}
