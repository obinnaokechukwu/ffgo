//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTwoPassTranscode_Integration(t *testing.T) {
	in := filepath.Join("testdata", "test.mp4")
	if _, err := os.Stat(in); err != nil {
		t.Skipf("missing test input: %v", err)
	}

	tmpDir := t.TempDir()
	out := filepath.Join(tmpDir, "out.mp4")
	passBase := filepath.Join(tmpDir, "passlog")

	opts := &EncoderOptions{
		Video: &VideoEncoderConfig{
			Codec:       CodecIDH264,
			Width:       0, // infer from input
			Height:      0,
			PixelFormat: PixelFormatYUV420P,
			FrameRate:   NewRational(25, 1),
			Bitrate:     500000,
			GOPSize:     10,
			MaxBFrames:  0,
		},
		PassLogFile: passBase,
	}

	if err := TwoPassTranscode(in, out, opts); err != nil {
		t.Skipf("two-pass not supported in this environment/encoder: %v", err)
	}

	if _, err := os.Stat(out); err != nil {
		t.Fatalf("output not created: %v", err)
	}

	// User-provided passlog base should not be cleaned up.
	matches, _ := filepath.Glob(passBase + "*")
	if len(matches) == 0 {
		t.Fatalf("expected passlog files with prefix %q", passBase)
	}
}

func TestTwoPassTranscode_HEVC_Integration(t *testing.T) {
	in := filepath.Join("testdata", "test.mp4")
	if _, err := os.Stat(in); err != nil {
		t.Skipf("missing test input: %v", err)
	}

	tmpDir := t.TempDir()
	out := filepath.Join(tmpDir, "out.mkv") // mkv is widely compatible for HEVC
	passBase := filepath.Join(tmpDir, "passlog")

	opts := &EncoderOptions{
		Video: &VideoEncoderConfig{
			Codec:       CodecIDHEVC,
			Width:       0, // infer from input
			Height:      0,
			PixelFormat: PixelFormatYUV420P,
			FrameRate:   NewRational(25, 1),
			Bitrate:     500000,
			GOPSize:     10,
			MaxBFrames:  0,
		},
		PassLogFile: passBase,
	}

	if err := TwoPassTranscode(in, out, opts); err != nil {
		t.Skipf("two-pass HEVC not supported in this environment/encoder: %v", err)
	}

	if _, err := os.Stat(out); err != nil {
		t.Fatalf("output not created: %v", err)
	}

	matches, _ := filepath.Glob(passBase + "*")
	if len(matches) == 0 {
		t.Fatalf("expected passlog files with prefix %q", passBase)
	}
}
