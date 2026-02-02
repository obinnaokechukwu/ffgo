//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"path/filepath"
	"testing"

	"github.com/obinnaokechukwu/ffgo/avutil"
)

func TestGenerateTimestamps(t *testing.T) {
	tb := NewRational(1, 30)
	got := GenerateTimestamps(3, tb, 30)
	want := []int64{0, 1, 2}
	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx %d: got %d want %d", i, got[i], want[i])
		}
	}
}

func TestValidateTimestamps(t *testing.T) {
	if !requireFFmpeg(t) {
		return
	}

	f1 := FrameAlloc()
	f2 := FrameAlloc()
	f3 := FrameAlloc()
	defer func() { _ = FrameFree(&f1) }()
	defer func() { _ = FrameFree(&f2) }()
	defer func() { _ = FrameFree(&f3) }()

	avutil.SetFramePTS(f1.ptr, 0)
	avutil.SetFramePTS(f2.ptr, 1)
	avutil.SetFramePTS(f3.ptr, 2)

	if err := ValidateTimestamps([]*Frame{&f1, &f2, &f3}); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}

	avutil.SetFramePTS(f2.ptr, -1)
	if err := ValidateTimestamps([]*Frame{&f1, &f2, &f3}); err == nil {
		t.Fatalf("expected error for non-monotonic pts")
	}
}

func TestFrameRateDetect(t *testing.T) {
	if testing.Short() {
		t.Log("Skipping FrameRateDetect in short mode")
		return
	}
	if !requireFFmpeg(t) {
		return
	}

	in := filepath.Join("testdata", "test.mp4")
	dec, err := NewDecoder(in)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	defer dec.Close()

	fps, err := FrameRateDetect(dec)
	if err != nil {
		t.Fatalf("FrameRateDetect failed: %v", err)
	}
	if fps <= 0 {
		t.Fatalf("expected positive fps, got %f", fps)
	}
	if fps > 240 {
		t.Fatalf("fps too high: %f", fps)
	}
}
