//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"path/filepath"
	"testing"

	"github.com/obinnaokechukwu/ffgo/avutil"
)

func TestNewConcatDecoder_Errors(t *testing.T) {
	if _, err := NewConcatDecoder(nil); err == nil {
		t.Fatalf("expected error for empty file list")
	}
	if _, err := NewConcatDecoder([]string{""}); err == nil {
		t.Fatalf("expected error for empty path")
	}
	if _, err := NewConcatDecoder([]string{"does-not-exist.mp4"}); err == nil {
		t.Fatalf("expected error for missing input file")
	}
	if _, err := NewConcatDecoderFromFFConcat(nil); err == nil {
		t.Fatalf("expected error for empty ffconcat script")
	}
	if _, err := NewConcatDecoderFromFile(""); err == nil {
		t.Fatalf("expected error for empty ffconcat list path")
	}
}

func TestNewConcatDecoder_ConcatsVideo(t *testing.T) {
	in := filepath.Join("testdata", "test.mp4")

	// Baseline single-file duration + frame count.
	d1, err := NewDecoder(in)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	defer d1.Close()

	dur1 := d1.DurationMicroseconds()
	_ = dur1 // duration may be unknown depending on demuxer/container

	count1, err := countVideoFrames(d1)
	if err != nil {
		t.Fatalf("countVideoFrames (single) failed: %v", err)
	}
	if count1 <= 0 {
		t.Fatalf("expected at least 1 video frame, got %d", count1)
	}

	// Concat same file twice.
	d2, err := NewConcatDecoder([]string{in, in})
	if err != nil {
		t.Fatalf("NewConcatDecoder failed: %v", err)
	}
	defer d2.Close()

	dur2 := d2.DurationMicroseconds()
	_ = dur2 // duration is often unknown for concat demuxer; rely on frame count instead.

	// Decode all video frames, ensure monotonic PTS where present and that we got >1 file worth.
	var lastPTS int64 = avutil.NoPTSValue
	var gotPTS bool
	count2 := 0
	for {
		f, err := d2.DecodeVideo()
		if err != nil {
			t.Fatalf("DecodeVideo failed: %v", err)
		}
		if f.IsNil() {
			break
		}
		count2++

		pts := avutil.GetFramePTS(f.ptr)
		if pts != avutil.NoPTSValue {
			if gotPTS && lastPTS != avutil.NoPTSValue && pts < lastPTS {
				t.Fatalf("non-monotonic PTS: prev=%d curr=%d", lastPTS, pts)
			}
			lastPTS = pts
			gotPTS = true
		}
	}

	if count2 < count1*2-1 {
		t.Fatalf("expected ~2x frames (single=%d, concat=%d)", count1, count2)
	}
	if !gotPTS {
		t.Fatalf("expected at least some frames to have PTS")
	}
}

func countVideoFrames(d *Decoder) (int, error) {
	n := 0
	for {
		f, err := d.DecodeVideo()
		if err != nil {
			return 0, err
		}
		if f.IsNil() {
			return n, nil
		}
		n++
	}
}
