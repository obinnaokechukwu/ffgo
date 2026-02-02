//go:build !ios && !android && (amd64 || arm64)

package avformat

import (
	"os"
	"path/filepath"
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

func requireFFmpeg(t *testing.T) bool {
	t.Helper()
	if !ffmpegAvailable {
		t.Log("FFmpeg not available")
		return false
	}
	return true
}

func testInputVideo(t *testing.T) string {
	t.Helper()
	testFile := filepath.Join("..", "testdata", "test.mp4")
	if _, err := os.Stat(testFile); err != nil {
		t.Fatalf("test input missing: %v", err)
	}
	return testFile
}

func TestAllocContext(t *testing.T) {
	if !requireFFmpeg(t) {
		return
	}
	ctx := AllocContext()
	if ctx == nil {
		t.Fatal("AllocContext returned nil")
	}
	FreeContext(ctx)
}

func TestOpenInput(t *testing.T) {
	if !requireFFmpeg(t) {
		return
	}
	testFile := testInputVideo(t)

	var ctx FormatContext
	err := OpenInput(&ctx, testFile, nil, nil)
	if err != nil {
		t.Fatalf("OpenInput failed: %v", err)
	}
	defer CloseInput(&ctx)

	if ctx == nil {
		t.Error("Context should not be nil after OpenInput")
	}
}

func TestFindStreamInfo(t *testing.T) {
	if !requireFFmpeg(t) {
		return
	}
	testFile := testInputVideo(t)

	var ctx FormatContext
	if err := OpenInput(&ctx, testFile, nil, nil); err != nil {
		t.Fatalf("OpenInput failed: %v", err)
	}
	defer CloseInput(&ctx)

	err := FindStreamInfo(ctx, nil)
	if err != nil {
		t.Fatalf("FindStreamInfo failed: %v", err)
	}

	// Should have at least one stream
	numStreams := GetNumStreams(ctx)
	if numStreams < 1 {
		t.Errorf("Expected at least 1 stream, got %d", numStreams)
	}
	t.Logf("Found %d streams", numStreams)
}

func TestFindBestStream(t *testing.T) {
	if !requireFFmpeg(t) {
		return
	}
	testFile := testInputVideo(t)

	var ctx FormatContext
	if err := OpenInput(&ctx, testFile, nil, nil); err != nil {
		t.Fatalf("OpenInput failed: %v", err)
	}
	defer CloseInput(&ctx)

	if err := FindStreamInfo(ctx, nil); err != nil {
		t.Fatalf("FindStreamInfo failed: %v", err)
	}

	// Find video stream
	videoIdx := FindBestStream(ctx, MediaTypeVideo, -1, -1, nil, 0)
	if videoIdx < 0 {
		t.Error("No video stream found")
	} else {
		t.Logf("Video stream index: %d", videoIdx)
	}

	// Find audio stream
	audioIdx := FindBestStream(ctx, MediaTypeAudio, -1, -1, nil, 0)
	if audioIdx < 0 {
		t.Error("No audio stream found")
	} else {
		t.Logf("Audio stream index: %d", audioIdx)
	}
}

func TestReadFrame(t *testing.T) {
	if !requireFFmpeg(t) {
		return
	}
	testFile := testInputVideo(t)

	var ctx FormatContext
	if err := OpenInput(&ctx, testFile, nil, nil); err != nil {
		t.Fatalf("OpenInput failed: %v", err)
	}
	defer CloseInput(&ctx)

	if err := FindStreamInfo(ctx, nil); err != nil {
		t.Fatalf("FindStreamInfo failed: %v", err)
	}

	// Read a few frames
	pkt := AllocPacket()
	if pkt == nil {
		t.Fatal("AllocPacket returned nil")
	}
	defer FreePacket(&pkt)

	frameCount := 0
	for i := 0; i < 10; i++ {
		err := ReadFrame(ctx, pkt)
		if err != nil {
			break
		}
		frameCount++
		PacketUnref(pkt)
	}

	if frameCount == 0 {
		t.Error("No frames read")
	} else {
		t.Logf("Read %d packets", frameCount)
	}
}

func TestVersion(t *testing.T) {
	if !requireFFmpeg(t) {
		return
	}
	ver := bindings.AVFormatVersion()
	if ver == 0 {
		t.Error("AVFormatVersion returned 0")
	}
	t.Logf("avformat version: %d.%d.%d", ver>>16, (ver>>8)&0xFF, ver&0xFF)
}
