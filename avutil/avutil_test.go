//go:build !ios && !android && (amd64 || arm64)

package avutil

import (
	"os"
	"testing"

	"github.com/obinnaokechukwu/ffgo/internal/bindings"
)

var ffmpegAvailable bool

func TestMain(m *testing.M) {
	if err := bindings.Load(); err == nil {
		ffmpegAvailable = true
	}
	os.Exit(m.Run())
}

func skipIfNoFFmpeg(t *testing.T) {
	t.Helper()
	if !ffmpegAvailable {
		t.Skip("FFmpeg not available")
	}
}

func TestFrameAlloc(t *testing.T) {
	skipIfNoFFmpeg(t)
	frame := FrameAlloc()
	if frame == nil {
		t.Fatal("FrameAlloc returned nil")
	}
	defer FrameFree(&frame)

	if frame == nil {
		t.Error("Frame should still be valid before free")
	}
}

func TestFrameFree(t *testing.T) {
	skipIfNoFFmpeg(t)
	frame := FrameAlloc()
	if frame == nil {
		t.Fatal("FrameAlloc returned nil")
	}

	FrameFree(&frame)

	if frame != nil {
		t.Error("Frame should be nil after free")
	}

	// Double free should be safe
	FrameFree(&frame)
}

func TestFrameAllocAndSetup(t *testing.T) {
	skipIfNoFFmpeg(t)
	frame := FrameAlloc()
	if frame == nil {
		t.Fatal("FrameAlloc returned nil")
	}
	defer FrameFree(&frame)

	// Set up a video frame
	SetFrameWidth(frame, 1920)
	SetFrameHeight(frame, 1080)
	SetFrameFormat(frame, int32(PixelFormatYUV420P))

	// Verify values
	if GetFrameWidth(frame) != 1920 {
		t.Errorf("Width: expected 1920, got %d", GetFrameWidth(frame))
	}
	if GetFrameHeight(frame) != 1080 {
		t.Errorf("Height: expected 1080, got %d", GetFrameHeight(frame))
	}
	if GetFrameFormat(frame) != int32(PixelFormatYUV420P) {
		t.Errorf("Format: expected %d, got %d", PixelFormatYUV420P, GetFrameFormat(frame))
	}
}

func TestRational(t *testing.T) {
	// Test basic creation
	r := NewRational(30000, 1001)
	if r.Num != 30000 || r.Den != 1001 {
		t.Errorf("Expected 30000/1001, got %d/%d", r.Num, r.Den)
	}

	// Test Float64
	fps := r.Float64()
	expected := 29.97002997
	if fps < expected-0.0001 || fps > expected+0.0001 {
		t.Errorf("Expected ~%f, got %f", expected, fps)
	}

	// Test zero denominator
	zr := NewRational(1, 0)
	if zr.Float64() != 0 {
		t.Error("Zero denominator should return 0")
	}
}

func TestRationalMul(t *testing.T) {
	a := NewRational(1, 2)
	b := NewRational(3, 4)
	result := a.Mul(b)

	// 1/2 * 3/4 = 3/8
	if result.Num != 3 || result.Den != 8 {
		t.Errorf("Expected 3/8, got %d/%d", result.Num, result.Den)
	}
}

func TestRationalDiv(t *testing.T) {
	a := NewRational(1, 2)
	b := NewRational(3, 4)
	result := a.Div(b)

	// 1/2 / 3/4 = 4/6 = 2/3
	if result.Float64() < 0.66 || result.Float64() > 0.67 {
		t.Errorf("Expected ~0.666, got %f", result.Float64())
	}
}

func TestRationalAdd(t *testing.T) {
	a := NewRational(1, 4)
	b := NewRational(1, 4)
	result := a.Add(b)

	// 1/4 + 1/4 = 1/2
	if result.Float64() != 0.5 {
		t.Errorf("Expected 0.5, got %f", result.Float64())
	}
}

func TestRationalSub(t *testing.T) {
	a := NewRational(3, 4)
	b := NewRational(1, 4)
	result := a.Sub(b)

	// 3/4 - 1/4 = 1/2
	if result.Float64() != 0.5 {
		t.Errorf("Expected 0.5, got %f", result.Float64())
	}
}

func TestRationalCmp(t *testing.T) {
	a := NewRational(1, 2)
	b := NewRational(1, 3)

	// 1/2 > 1/3
	cmp := a.Cmp(b)
	if cmp <= 0 {
		t.Errorf("Expected 1/2 > 1/3, got cmp=%d", cmp)
	}

	// Equal
	c := NewRational(2, 4)
	cmp = a.Cmp(c)
	if cmp != 0 {
		t.Errorf("Expected 1/2 == 2/4, got cmp=%d", cmp)
	}
}

func TestErrorString(t *testing.T) {
	skipIfNoFFmpeg(t)
	// AVERROR_EOF
	msg := ErrorString(AVERROR_EOF)
	if msg == "" {
		t.Error("ErrorString should return non-empty string for AVERROR_EOF")
	}
	t.Logf("AVERROR_EOF message: %s", msg)

	// Invalid error should still return something
	msg = ErrorString(-999999)
	if msg == "" {
		t.Error("ErrorString should return non-empty string for unknown error")
	}
}
