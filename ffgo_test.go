//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func init() {
	if err := Init(); err != nil {
		panic("Failed to initialize FFmpeg: " + err.Error())
	}
}

// createTestVideo creates a test video file using ffmpeg CLI
func createTestVideo(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.mp4")

	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=320x240:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-c:a", "aac", "-b:a", "128k",
		"-pix_fmt", "yuv420p",
		testFile)

	if err := cmd.Run(); err != nil {
		t.Skipf("ffmpeg not available or failed: %v", err)
		return ""
	}

	if _, err := os.Stat(testFile); err != nil {
		t.Skipf("Test file not created: %v", err)
		return ""
	}

	return testFile
}

func TestInit(t *testing.T) {
	err := Init()
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if !IsLoaded() {
		t.Error("IsLoaded returned false after Init")
	}
}

func TestVersion(t *testing.T) {
	avutil, avcodec, avformat := Version()
	if avutil == 0 {
		t.Error("avutil version is 0")
	}
	if avcodec == 0 {
		t.Error("avcodec version is 0")
	}
	if avformat == 0 {
		t.Error("avformat version is 0")
	}
	t.Logf("Versions: avutil=%d.%d.%d, avcodec=%d.%d.%d, avformat=%d.%d.%d",
		avutil>>16, (avutil>>8)&0xFF, avutil&0xFF,
		avcodec>>16, (avcodec>>8)&0xFF, avcodec&0xFF,
		avformat>>16, (avformat>>8)&0xFF, avformat&0xFF)
}

func TestDecoder(t *testing.T) {
	testFile := createTestVideo(t)
	if testFile == "" {
		return
	}

	decoder, err := NewDecoder(testFile)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	defer decoder.Close()

	// Check stream info
	if !decoder.HasVideo() {
		t.Error("Expected video stream")
	}
	if !decoder.HasAudio() {
		t.Error("Expected audio stream")
	}

	videoInfo := decoder.VideoStream()
	if videoInfo == nil {
		t.Fatal("VideoStream returned nil")
	}
	t.Logf("Video: %dx%d, codec=%d", videoInfo.Width, videoInfo.Height, videoInfo.CodecID)

	if videoInfo.Width != 320 || videoInfo.Height != 240 {
		t.Errorf("Expected 320x240, got %dx%d", videoInfo.Width, videoInfo.Height)
	}

	audioInfo := decoder.AudioStream()
	if audioInfo == nil {
		t.Fatal("AudioStream returned nil")
	}
	t.Logf("Audio: sample_rate=%d, codec=%d", audioInfo.SampleRate, audioInfo.CodecID)
}

func TestDecoderDecodeVideo(t *testing.T) {
	testFile := createTestVideo(t)
	if testFile == "" {
		return
	}

	decoder, err := NewDecoder(testFile)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	defer decoder.Close()

	// Open video decoder
	err = decoder.OpenVideoDecoder()
	if err != nil {
		t.Fatalf("OpenVideoDecoder failed: %v", err)
	}

	// Decode a few frames
	frameCount := 0
	for i := 0; i < 10; i++ {
		frame, err := decoder.DecodeVideo()
		if err != nil {
			if IsEOF(err) {
				break
			}
			t.Fatalf("DecodeVideo failed: %v", err)
		}
		if frame != nil {
			frameCount++
			info := GetFrameInfo(frame)
			t.Logf("Frame %d: %dx%d, format=%d, pts=%d",
				frameCount, info.Width, info.Height, info.Format, info.PTS)
		}
	}

	if frameCount == 0 {
		t.Error("No frames decoded")
	} else {
		t.Logf("Decoded %d frames", frameCount)
	}
}

func TestScaler(t *testing.T) {
	// Create scaler
	scaler, err := NewScaler(ScalerConfig{
		SrcWidth:  320,
		SrcHeight: 240,
		SrcFormat: PixelFormatYUV420P,
		DstWidth:  160,
		DstHeight: 120,
		DstFormat: PixelFormatRGB24,
		Flags:     ScaleBilinear,
	})
	if err != nil {
		t.Fatalf("NewScaler failed: %v", err)
	}
	defer scaler.Close()

	// Verify dimensions
	if scaler.SrcWidth() != 320 || scaler.SrcHeight() != 240 {
		t.Errorf("Source dimensions wrong: %dx%d", scaler.SrcWidth(), scaler.SrcHeight())
	}
	if scaler.DstWidth() != 160 || scaler.DstHeight() != 120 {
		t.Errorf("Destination dimensions wrong: %dx%d", scaler.DstWidth(), scaler.DstHeight())
	}
}

func TestScalerWithDecoder(t *testing.T) {
	testFile := createTestVideo(t)
	if testFile == "" {
		return
	}

	decoder, err := NewDecoder(testFile)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	defer decoder.Close()

	videoInfo := decoder.VideoStream()
	if videoInfo == nil {
		t.Skip("No video stream")
	}

	// Create scaler to convert to RGB
	scaler, err := NewScaler(ScalerConfig{
		SrcWidth:  videoInfo.Width,
		SrcHeight: videoInfo.Height,
		SrcFormat: videoInfo.PixelFmt,
		DstWidth:  videoInfo.Width / 2,
		DstHeight: videoInfo.Height / 2,
		DstFormat: PixelFormatRGB24,
		Flags:     ScaleBilinear,
	})
	if err != nil {
		t.Fatalf("NewScaler failed: %v", err)
	}
	defer scaler.Close()

	// Open video decoder
	if err := decoder.OpenVideoDecoder(); err != nil {
		t.Fatalf("OpenVideoDecoder failed: %v", err)
	}

	// Decode and scale one frame
	frame, err := decoder.DecodeVideo()
	if err != nil {
		t.Fatalf("DecodeVideo failed: %v", err)
	}
	if frame == nil {
		t.Skip("No frame decoded")
	}

	// Scale the frame
	scaledFrame, err := scaler.Scale(frame)
	if err != nil {
		t.Fatalf("Scale failed: %v", err)
	}

	scaledInfo := GetFrameInfo(scaledFrame)
	t.Logf("Scaled frame: %dx%d, format=%d", scaledInfo.Width, scaledInfo.Height, scaledInfo.Format)

	if scaledInfo.Width != videoInfo.Width/2 || scaledInfo.Height != videoInfo.Height/2 {
		t.Errorf("Expected %dx%d, got %dx%d",
			videoInfo.Width/2, videoInfo.Height/2, scaledInfo.Width, scaledInfo.Height)
	}
}

func TestFrameAlloc(t *testing.T) {
	frame := FrameAlloc()
	if frame == nil {
		t.Fatal("FrameAlloc returned nil")
	}
	defer FrameFree(&frame)

	if frame == nil {
		t.Error("Frame should not be nil before free")
	}
}

func TestRational(t *testing.T) {
	r := NewRational(30000, 1001)
	if r.Num != 30000 || r.Den != 1001 {
		t.Errorf("Expected 30000/1001, got %d/%d", r.Num, r.Den)
	}

	f := r.Float64()
	expected := 29.97002997
	if f < expected-0.0001 || f > expected+0.0001 {
		t.Errorf("Expected ~%f, got %f", expected, f)
	}
}
