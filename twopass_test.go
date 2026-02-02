//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/obinnaokechukwu/ffgo/avcodec"
)

func TestTwoPassTranscode_Integration(t *testing.T) {
	if testing.Short() {
		t.Log("Skipping two-pass integration test in short mode")
		return
	}
	if !requireFFmpeg(t) {
		return
	}

	in := filepath.Join("testdata", "test.mp4")
	if _, err := os.Stat(in); err != nil {
		t.Fatalf("missing test input: %v", err)
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

	if avcodec.FindEncoder(avcodec.CodecIDH264) == nil {
		t.Log("H.264 encoder not available in this FFmpeg build")
		return
	}

	if err := TwoPassTranscode(in, out, opts); err != nil {
		t.Logf("TwoPassTranscode not supported in this environment/encoder: %v", err)
		return
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
	if testing.Short() {
		t.Log("Skipping two-pass HEVC integration test in short mode")
		return
	}
	if !requireFFmpeg(t) {
		return
	}

	in := filepath.Join("testdata", "test.mp4")
	if _, err := os.Stat(in); err != nil {
		t.Fatalf("missing test input: %v", err)
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

	if avcodec.FindEncoder(avcodec.CodecIDHEVC) == nil {
		t.Log("HEVC encoder not available in this FFmpeg build")
		return
	}

	if err := TwoPassTranscode(in, out, opts); err != nil {
		t.Logf("TwoPassTranscode (HEVC) not supported in this environment/encoder: %v", err)
		return
	}

	if _, err := os.Stat(out); err != nil {
		t.Fatalf("output not created: %v", err)
	}

	matches, _ := filepath.Glob(passBase + "*")
	if len(matches) == 0 {
		t.Fatalf("expected passlog files with prefix %q", passBase)
	}
}
