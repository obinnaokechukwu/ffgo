//go:build !ios && !android && (amd64 || arm64)

package ffgo

import "testing"

func TestColorSpec_RoundTrip(t *testing.T) {
	if !requireFFmpeg(t) {
		return
	}
	if !colorOffsetsAvailable() {
		t.Log("color offsets not available (shim missing ffshim_avframe_color_offsets)")
		return
	}

	f := FrameAlloc()
	if f.IsNil() {
		t.Fatal("FrameAlloc returned nil")
	}
	defer func() { _ = FrameFree(&f) }()

	want := ColorSpec{
		Range:     ColorRangeJPEG,
		Space:     1,
		Primaries: 1,
		Transfer:  1,
	}
	f.SetColorSpec(want)
	got := f.ColorSpec()
	if got != want {
		t.Fatalf("ColorSpec mismatch: got %+v, want %+v", got, want)
	}
}
