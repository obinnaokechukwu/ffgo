//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/obinnaokechukwu/ffgo/avutil"
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

func TestEncoder(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "output.mp4")

	// Create encoder
	encoder, err := NewEncoder(outFile, EncoderConfig{
		Width:       320,
		Height:      240,
		PixelFormat: PixelFormatYUV420P,
		CodecID:     CodecIDH264,
		BitRate:     1000000,
		FrameRate:   30,
		GOPSize:     12,
		MaxBFrames:  0,
	})
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}
	defer encoder.Close()

	// Verify encoder properties
	if encoder.Width() != 320 {
		t.Errorf("Width: expected 320, got %d", encoder.Width())
	}
	if encoder.Height() != 240 {
		t.Errorf("Height: expected 240, got %d", encoder.Height())
	}
	if encoder.PixelFormat() != PixelFormatYUV420P {
		t.Errorf("PixelFormat: expected %d, got %d", PixelFormatYUV420P, encoder.PixelFormat())
	}

	t.Logf("Encoder created: %dx%d, format=%d", encoder.Width(), encoder.Height(), encoder.PixelFormat())
}

func TestEncoderWriteFrames(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "output.mp4")

	// Create encoder
	encoder, err := NewEncoder(outFile, EncoderConfig{
		Width:       160,
		Height:      120,
		PixelFormat: PixelFormatYUV420P,
		CodecID:     CodecIDH264,
		BitRate:     500000,
		FrameRate:   15,
		GOPSize:     10,
	})
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}

	// Create a test frame
	frame := FrameAlloc()
	if frame == nil {
		t.Fatal("FrameAlloc returned nil")
	}
	defer FrameFree(&frame)

	// Set up frame
	AVUtil.SetFrameWidth(frame, 160)
	AVUtil.SetFrameHeight(frame, 120)
	AVUtil.SetFrameFormat(frame, int32(PixelFormatYUV420P))

	// Allocate frame buffer
	if err := AVUtil.FrameGetBuffer(frame, 0); err != nil {
		t.Fatalf("FrameGetBuffer failed: %v", err)
	}

	// Write a few frames (solid color frames)
	numFrames := 15 // Half a second at 30 fps
	for i := 0; i < numFrames; i++ {
		// Make frame writable
		if err := AVUtil.FrameMakeWritable(frame); err != nil {
			t.Fatalf("FrameMakeWritable failed: %v", err)
		}

		// Fill Y plane with a gradient
		fillTestFrame(frame, i, 160, 120)

		if err := encoder.WriteFrame(frame); err != nil {
			t.Fatalf("WriteFrame failed at frame %d: %v", i, err)
		}
	}

	// Close encoder (flushes and writes trailer)
	if err := encoder.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify output file exists
	info, err := os.Stat(outFile)
	if err != nil {
		t.Fatalf("Output file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Output file is empty")
	}

	t.Logf("Encoded %d frames to %s (%d bytes)", numFrames, outFile, info.Size())

	// Verify output file can be read
	decoder, err := NewDecoder(outFile)
	if err != nil {
		t.Fatalf("Cannot read output file: %v", err)
	}
	defer decoder.Close()

	if !decoder.HasVideo() {
		t.Error("Output file has no video stream")
	}

	videoInfo := decoder.VideoStream()
	if videoInfo.Width != 160 || videoInfo.Height != 120 {
		t.Errorf("Output dimensions wrong: %dx%d", videoInfo.Width, videoInfo.Height)
	}

	t.Logf("Output file verified: %dx%d, codec=%s",
		videoInfo.Width, videoInfo.Height, videoInfo.CodecID.String())
}

// fillTestFrame fills a YUV420P frame with a test pattern
func fillTestFrame(frame Frame, frameNum, width, height int) {
	// Get data pointers using avutil package directly
	data := avutil.GetFrameData(frame)
	linesize := avutil.GetFrameLinesize(frame)

	// Y plane
	if data[0] != nil {
		yPlane := (*[1 << 24]byte)(unsafe.Pointer(data[0]))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				// Create a moving gradient
				val := byte((x + y + frameNum*5) % 256)
				yPlane[y*int(linesize[0])+x] = val
			}
		}
	}

	// U and V planes (half size for YUV420P)
	uvHeight := height / 2
	uvWidth := width / 2

	if data[1] != nil {
		uPlane := (*[1 << 24]byte)(unsafe.Pointer(data[1]))
		for y := 0; y < uvHeight; y++ {
			for x := 0; x < uvWidth; x++ {
				uPlane[y*int(linesize[1])+x] = 128
			}
		}
	}

	if data[2] != nil {
		vPlane := (*[1 << 24]byte)(unsafe.Pointer(data[2]))
		for y := 0; y < uvHeight; y++ {
			for x := 0; x < uvWidth; x++ {
				vPlane[y*int(linesize[2])+x] = 128
			}
		}
	}
}

func TestEncoderWithDecoder(t *testing.T) {
	// Create a test video
	inputFile := createTestVideo(t)
	if inputFile == "" {
		return
	}

	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "transcoded.mp4")

	// Open input
	decoder, err := NewDecoder(inputFile)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	defer decoder.Close()

	if !decoder.HasVideo() {
		t.Skip("No video stream in input")
	}

	videoInfo := decoder.VideoStream()

	// Create encoder with same dimensions
	encoder, err := NewEncoder(outputFile, EncoderConfig{
		Width:       videoInfo.Width,
		Height:      videoInfo.Height,
		PixelFormat: PixelFormatYUV420P,
		CodecID:     CodecIDH264,
		BitRate:     500000,
		FrameRate:   30,
		GOPSize:     12,
	})
	if err != nil {
		t.Fatalf("NewEncoder failed: %v", err)
	}

	// Open decoder
	if err := decoder.OpenVideoDecoder(); err != nil {
		t.Fatalf("OpenVideoDecoder failed: %v", err)
	}

	// Transcode a few frames
	framesTranscoded := 0
	for i := 0; i < 10; i++ {
		frame, err := decoder.DecodeVideo()
		if err != nil {
			if IsEOF(err) {
				break
			}
			t.Fatalf("DecodeVideo failed: %v", err)
		}
		if frame == nil {
			continue
		}

		if err := encoder.WriteFrame(frame); err != nil {
			t.Fatalf("WriteFrame failed: %v", err)
		}
		framesTranscoded++
	}

	// Close encoder
	if err := encoder.Close(); err != nil {
		t.Fatalf("Encoder close failed: %v", err)
	}

	t.Logf("Transcoded %d frames from %s to %s", framesTranscoded, inputFile, outputFile)

	// Verify output exists
	if _, err := os.Stat(outputFile); err != nil {
		t.Fatalf("Output file not created: %v", err)
	}
}
