//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/obinnaokechukwu/ffgo/avutil"
)

func TestHLSSegmenter_Integration(t *testing.T) {
	if testing.Short() {
		t.Log("Skipping HLS segmenter integration test in short mode")
		return
	}
	if !requireFFmpeg(t) {
		return
	}

	tmpDir := t.TempDir()
	playlist := filepath.Join(tmpDir, "out.m3u8")
	segPattern := filepath.Join(tmpDir, "seg_%03d.ts")

	cfg := &HLSSegmenterConfig{
		SegmentTime:            500 * time.Millisecond,
		ListSize:               0,
		SegmentFilenamePattern: segPattern,
	}
	hls, err := NewHLSSegmenter(playlist, cfg)
	if err != nil {
		t.Logf("hls muxer not available: %v", err)
		return
	}
	defer hls.Close()

	vs, err := hls.AddVideoStream(&VideoStreamConfig{
		Codec:       CodecIDH264,
		Width:       160,
		Height:      120,
		PixelFormat: PixelFormatYUV420P,
		FrameRate:   25,
		BitRate:     500000,
	})
	if err != nil {
		t.Logf("unable to add video stream: %v", err)
		return
	}

	opts, err := cfg.HeaderOptions()
	if err != nil {
		t.Fatalf("HeaderOptions: %v", err)
	}
	if err := hls.WriteHeaderWithOptions(opts); err != nil {
		t.Logf("WriteHeaderWithOptions failed (hls may be missing in this FFmpeg build): %v", err)
		return
	}

	for i := 0; i < 60; i++ {
		frame := FrameAlloc()
		if frame.IsNil() {
			t.Fatal("Failed to allocate frame")
		}

		AVUtil.SetFrameWidth(frame, 160)
		AVUtil.SetFrameHeight(frame, 120)
		AVUtil.SetFrameFormat(frame, int32(PixelFormatYUV420P))
		if err := AVUtil.FrameGetBuffer(frame, 32); err != nil {
			_ = FrameFree(&frame)
			t.Fatalf("Failed to allocate frame buffer: %v", err)
		}
		fillTestFrameYUV420(frame, uint8(i*3))
		avutil.SetFramePTS(frame.ptr, int64(i))

		if err := hls.WriteFrame(vs, frame); err != nil {
			_ = FrameFree(&frame)
			t.Fatalf("WriteFrame failed: %v", err)
		}
		_ = FrameFree(&frame)
	}

	if err := hls.WriteTrailer(); err != nil {
		t.Fatalf("WriteTrailer failed: %v", err)
	}
	_ = hls.Close()

	if _, err := os.Stat(playlist); err != nil {
		t.Fatalf("playlist not found: %v", err)
	}
	segs, _ := filepath.Glob(filepath.Join(tmpDir, "*.ts"))
	if len(segs) == 0 {
		t.Fatalf("expected at least one .ts segment in %s", tmpDir)
	}
}

func TestDASHSegmenter_Integration(t *testing.T) {
	if testing.Short() {
		t.Log("Skipping DASH segmenter integration test in short mode")
		return
	}
	if !requireFFmpeg(t) {
		return
	}

	tmpDir := t.TempDir()
	mpd := filepath.Join(tmpDir, "out.mpd")

	cfg := &DASHSegmenterConfig{
		SegmentTime: 500 * time.Millisecond,
		InitName:    "init-stream$RepresentationID$.mp4",
		MediaName:   "chunk-stream$RepresentationID$-$Number%05d$.m4s",
	}
	dash, err := NewDASHSegmenter(mpd, cfg)
	if err != nil {
		t.Logf("dash muxer not available: %v", err)
		return
	}
	defer dash.Close()

	vs, err := dash.AddVideoStream(&VideoStreamConfig{
		Codec:       CodecIDH264,
		Width:       160,
		Height:      120,
		PixelFormat: PixelFormatYUV420P,
		FrameRate:   25,
		BitRate:     500000,
	})
	if err != nil {
		t.Logf("unable to add video stream: %v", err)
		return
	}

	if err := dash.WriteHeaderWithOptions(cfg.HeaderOptions()); err != nil {
		t.Logf("WriteHeaderWithOptions failed (dash may be missing in this FFmpeg build): %v", err)
		return
	}

	for i := 0; i < 60; i++ {
		frame := FrameAlloc()
		if frame.IsNil() {
			t.Fatal("Failed to allocate frame")
		}

		AVUtil.SetFrameWidth(frame, 160)
		AVUtil.SetFrameHeight(frame, 120)
		AVUtil.SetFrameFormat(frame, int32(PixelFormatYUV420P))
		if err := AVUtil.FrameGetBuffer(frame, 32); err != nil {
			_ = FrameFree(&frame)
			t.Fatalf("Failed to allocate frame buffer: %v", err)
		}
		fillTestFrameYUV420(frame, uint8(i*3))
		avutil.SetFramePTS(frame.ptr, int64(i))

		if err := dash.WriteFrame(vs, frame); err != nil {
			_ = FrameFree(&frame)
			t.Fatalf("WriteFrame failed: %v", err)
		}
		_ = FrameFree(&frame)
	}

	if err := dash.WriteTrailer(); err != nil {
		t.Fatalf("WriteTrailer failed: %v", err)
	}
	_ = dash.Close()

	if _, err := os.Stat(mpd); err != nil {
		t.Fatalf("mpd not found: %v", err)
	}
}
