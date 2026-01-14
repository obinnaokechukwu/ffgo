//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"io"
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
	scaler, err := NewScalerWithConfig(ScalerConfig{
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
	scaler, err := NewScalerWithConfig(ScalerConfig{
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

func TestDecoderFromReader(t *testing.T) {
	testFile := createTestVideo(t)
	if testFile == "" {
		return
	}

	// Open file as io.Reader
	file, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	// Create decoder from reader
	decoder, err := NewDecoderFromReader(file, "")
	if err != nil {
		t.Fatalf("NewDecoderFromReader failed: %v", err)
	}
	defer decoder.Close()

	// Check stream info
	if !decoder.HasVideo() {
		t.Error("Expected video stream")
	}

	videoInfo := decoder.VideoStream()
	if videoInfo == nil {
		t.Fatal("VideoStream returned nil")
	}

	t.Logf("Video from reader: %dx%d, codec=%s",
		videoInfo.Width, videoInfo.Height, videoInfo.CodecID.String())

	if videoInfo.Width != 320 || videoInfo.Height != 240 {
		t.Errorf("Expected 320x240, got %dx%d", videoInfo.Width, videoInfo.Height)
	}
}

func TestDecoderFromReaderWithDecode(t *testing.T) {
	testFile := createTestVideo(t)
	if testFile == "" {
		return
	}

	// Open file as io.Reader
	file, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	// Create decoder from reader
	decoder, err := NewDecoderFromReader(file, "")
	if err != nil {
		t.Fatalf("NewDecoderFromReader failed: %v", err)
	}
	defer decoder.Close()

	// Open video decoder
	if err := decoder.OpenVideoDecoder(); err != nil {
		t.Fatalf("OpenVideoDecoder failed: %v", err)
	}

	// Decode a few frames
	frameCount := 0
	for i := 0; i < 5; i++ {
		frame, err := decoder.DecodeVideo()
		if err != nil {
			if IsEOF(err) {
				break
			}
			t.Fatalf("DecodeVideo failed: %v", err)
		}
		if frame != nil {
			frameCount++
		}
	}

	if frameCount == 0 {
		t.Error("No frames decoded from reader")
	}
	t.Logf("Decoded %d frames from io.Reader", frameCount)
}

func TestDecoderFromIOCallbacks(t *testing.T) {
	testFile := createTestVideo(t)
	if testFile == "" {
		return
	}

	// Open file manually
	file, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	// Track stats
	totalBytesRead := int64(0)
	readCalls := 0

	// Create custom callbacks
	callbacks := &IOCallbacks{
		Read: func(buf []byte) (int, error) {
			n, err := file.Read(buf)
			if n > 0 {
				totalBytesRead += int64(n)
				readCalls++
			}
			if err == io.EOF {
				return n, io.EOF
			}
			return n, err
		},
		Seek: func(offset int64, whence int) (int64, error) {
			return file.Seek(offset, whence)
		},
	}

	// Create decoder with custom callbacks
	decoder, err := NewDecoderFromIO(callbacks, "")
	if err != nil {
		t.Fatalf("NewDecoderFromIO failed: %v", err)
	}
	defer decoder.Close()

	// Check we can read stream info
	if !decoder.HasVideo() {
		t.Error("Expected video stream")
	}

	// Open and decode a frame
	if err := decoder.OpenVideoDecoder(); err != nil {
		t.Fatalf("OpenVideoDecoder failed: %v", err)
	}

	frame, err := decoder.DecodeVideo()
	if err != nil && !IsEOF(err) {
		t.Fatalf("DecodeVideo failed: %v", err)
	}
	if frame == nil {
		t.Error("Expected a decoded frame")
	}

	t.Logf("Custom I/O stats: bytes_read=%d, read_calls=%d", totalBytesRead, readCalls)

	if totalBytesRead == 0 {
		t.Error("No bytes read through custom I/O")
	}
	if readCalls == 0 {
		t.Error("Read callback was never called")
	}
}

func TestEncoderWithAudio(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "audio_video.mp4")

	// Create encoder with audio
	encoder, err := NewEncoderWithOptions(outFile, &EncoderOptions{
		Video: &VideoEncoderConfig{
			Codec:       CodecIDH264,
			Width:       320,
			Height:      240,
			FrameRate:   NewRational(30, 1),
			Bitrate:     500000,
			PixelFormat: PixelFormatYUV420P,
			GOPSize:     10,
		},
		Audio: &AudioEncoderConfig{
			Codec:      CodecIDAAC,
			SampleRate: 48000,
			Channels:   2,
			Bitrate:    128000,
		},
	})
	if err != nil {
		t.Fatalf("NewEncoderWithOptions failed: %v", err)
	}
	defer encoder.Close()

	// Verify encoder has both video and audio
	if !encoder.HasVideo() {
		t.Error("Encoder should have video")
	}
	if !encoder.HasAudio() {
		t.Error("Encoder should have audio")
	}

	// Check audio properties
	if encoder.SampleRate() != 48000 {
		t.Errorf("SampleRate = %d, want 48000", encoder.SampleRate())
	}
	if encoder.Channels() != 2 {
		t.Errorf("Channels = %d, want 2", encoder.Channels())
	}
	if encoder.AudioFrameSize() == 0 {
		t.Log("AudioFrameSize is 0 (codec may determine dynamically)")
	}

	t.Logf("Encoder created with audio: sample_rate=%d, channels=%d, frame_size=%d",
		encoder.SampleRate(), encoder.Channels(), encoder.AudioFrameSize())
}

func TestEncoderWriteVideoAndAudioFrames(t *testing.T) {
	tmpDir := t.TempDir()
	outFile := filepath.Join(tmpDir, "av_output.mp4")

	// Create encoder with audio
	encoder, err := NewEncoderWithOptions(outFile, &EncoderOptions{
		Video: &VideoEncoderConfig{
			Codec:       CodecIDH264,
			Width:       160,
			Height:      120,
			FrameRate:   NewRational(10, 1),
			Bitrate:     200000,
			PixelFormat: PixelFormatYUV420P,
			GOPSize:     5,
		},
		Audio: &AudioEncoderConfig{
			Codec:      CodecIDAAC,
			SampleRate: 44100,
			Channels:   2,
			Bitrate:    96000,
		},
	})
	if err != nil {
		t.Fatalf("NewEncoderWithOptions failed: %v", err)
	}

	// Allocate video frame
	videoFrame := FrameAlloc()
	if videoFrame == nil {
		encoder.Close()
		t.Fatal("Failed to allocate video frame")
	}
	defer FrameFree(&videoFrame)

	avutil.SetFrameWidth(videoFrame, 160)
	avutil.SetFrameHeight(videoFrame, 120)
	avutil.SetFrameFormat(videoFrame, int32(PixelFormatYUV420P))

	if err := avutil.FrameGetBufferErr(videoFrame, 0); err != nil {
		encoder.Close()
		t.Fatalf("Failed to allocate video frame buffer: %v", err)
	}

	// Write a few video frames
	numVideoFrames := 10
	for i := 0; i < numVideoFrames; i++ {
		if err := avutil.FrameMakeWritable(videoFrame); err != nil {
			encoder.Close()
			t.Fatalf("FrameMakeWritable failed: %v", err)
		}
		fillTestFrame(videoFrame, i, 160, 120)
		if err := encoder.WriteVideoFrame(videoFrame); err != nil {
			encoder.Close()
			t.Fatalf("WriteVideoFrame failed at frame %d: %v", i, err)
		}
	}

	// Close encoder
	if err := encoder.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify output file
	info, err := os.Stat(outFile)
	if err != nil {
		t.Fatalf("Output file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Output file is empty")
	}

	t.Logf("Encoded %d video frames to %s (%d bytes)", numVideoFrames, outFile, info.Size())

	// Verify output can be read
	decoder, err := NewDecoder(outFile)
	if err != nil {
		t.Fatalf("Cannot read output: %v", err)
	}
	defer decoder.Close()

	if !decoder.HasVideo() {
		t.Error("Output should have video")
	}
	// Note: Audio stream should be present even if we didn't write audio frames,
	// but it will be empty/silent
	t.Logf("Output verified: video=%v, audio=%v", decoder.HasVideo(), decoder.HasAudio())
}

func TestSampleFormatConstants(t *testing.T) {
	// Verify sample format constants are exported correctly
	tests := []struct {
		name   string
		format SampleFormat
		want   int32
	}{
		{"SampleFormatNone", SampleFormatNone, -1},
		{"SampleFormatU8", SampleFormatU8, 0},
		{"SampleFormatS16", SampleFormatS16, 1},
		{"SampleFormatS32", SampleFormatS32, 2},
		{"SampleFormatFlt", SampleFormatFlt, 3},
		{"SampleFormatDbl", SampleFormatDbl, 4},
		{"SampleFormatU8P", SampleFormatU8P, 5},
		{"SampleFormatS16P", SampleFormatS16P, 6},
		{"SampleFormatS32P", SampleFormatS32P, 7},
		{"SampleFormatFLTP", SampleFormatFLTP, 8},
		{"SampleFormatFltP", SampleFormatFltP, 8},
		{"SampleFormatDblP", SampleFormatDblP, 9},
		{"SampleFormatS64", SampleFormatS64, 10},
		{"SampleFormatS64P", SampleFormatS64P, 11},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int32(tt.format) != tt.want {
				t.Errorf("%s = %d, want %d", tt.name, tt.format, tt.want)
			}
		})
	}
}

func TestNewResampler(t *testing.T) {
	// Create a resampler for 44100Hz stereo S16 -> 48000Hz stereo FLTP
	src := AudioFormat{
		SampleRate:   44100,
		Channels:     2,
		SampleFormat: SampleFormatS16,
	}
	dst := AudioFormat{
		SampleRate:   48000,
		Channels:     2,
		SampleFormat: SampleFormatFLTP,
	}

	resampler, err := NewResampler(src, dst)
	if err != nil {
		t.Fatalf("NewResampler failed: %v", err)
	}
	defer resampler.Close()

	// Verify formats
	if resampler.SrcFormat().SampleRate != 44100 {
		t.Errorf("SrcFormat().SampleRate = %d, want 44100", resampler.SrcFormat().SampleRate)
	}
	if resampler.DstFormat().SampleRate != 48000 {
		t.Errorf("DstFormat().SampleRate = %d, want 48000", resampler.DstFormat().SampleRate)
	}

	t.Logf("Resampler created: %dHz %dch -> %dHz %dch",
		resampler.SrcFormat().SampleRate, resampler.SrcFormat().Channels,
		resampler.DstFormat().SampleRate, resampler.DstFormat().Channels)
}

func TestResamplerValidation(t *testing.T) {
	// Test invalid inputs
	tests := []struct {
		name string
		src  AudioFormat
		dst  AudioFormat
	}{
		{
			name: "invalid src sample rate",
			src:  AudioFormat{SampleRate: 0, Channels: 2},
			dst:  AudioFormat{SampleRate: 48000, Channels: 2},
		},
		{
			name: "invalid dst sample rate",
			src:  AudioFormat{SampleRate: 44100, Channels: 2},
			dst:  AudioFormat{SampleRate: 0, Channels: 2},
		},
		{
			name: "invalid src channels",
			src:  AudioFormat{SampleRate: 44100, Channels: 0},
			dst:  AudioFormat{SampleRate: 48000, Channels: 2},
		},
		{
			name: "invalid dst channels",
			src:  AudioFormat{SampleRate: 44100, Channels: 2},
			dst:  AudioFormat{SampleRate: 48000, Channels: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resampler, err := NewResampler(tt.src, tt.dst)
			if err == nil {
				resampler.Close()
				t.Error("Expected error, got nil")
			}
		})
	}
}

func TestChannelLayoutConstants(t *testing.T) {
	// Verify channel layout constants
	tests := []struct {
		name   string
		layout ChannelLayout
		want   int64
	}{
		{"Mono", ChannelLayoutMono, 0x4},
		{"Stereo", ChannelLayoutStereo, 0x3},
		{"5.1", ChannelLayout5Point1, 0x60F},
		{"7.1", ChannelLayout7Point1, 0x63F},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if int64(tt.layout) != tt.want {
				t.Errorf("%s = 0x%x, want 0x%x", tt.name, tt.layout, tt.want)
			}
		})
	}
}

func TestChannelLayoutString(t *testing.T) {
	tests := []struct {
		layout ChannelLayout
		want   string
	}{
		{ChannelLayoutMono, "mono"},
		{ChannelLayoutStereo, "stereo"},
		{ChannelLayout5Point1, "5.1"},
		{ChannelLayout7Point1, "7.1"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.layout.String(); got != tt.want {
				t.Errorf("layout.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChannelLayoutNumChannels(t *testing.T) {
	tests := []struct {
		layout   ChannelLayout
		channels int
	}{
		{ChannelLayoutMono, 1},
		{ChannelLayoutStereo, 2},
		{ChannelLayout5Point1, 6},
		{ChannelLayout7Point1, 8},
	}

	for _, tt := range tests {
		t.Run(tt.layout.String(), func(t *testing.T) {
			if got := tt.layout.NumChannels(); got != tt.channels {
				t.Errorf("layout.NumChannels() = %d, want %d", got, tt.channels)
			}
		})
	}
}

// FilterGraph tests

func TestNewVideoFilterGraph(t *testing.T) {
	graph, err := NewVideoFilterGraph("null", 320, 240, PixelFormatYUV420P)
	if err != nil {
		t.Fatalf("NewVideoFilterGraph failed: %v", err)
	}
	defer graph.Close()

	if !graph.IsVideo() {
		t.Error("expected IsVideo() to be true")
	}
	if graph.IsAudio() {
		t.Error("expected IsAudio() to be false")
	}

	t.Log("NewVideoFilterGraph with null filter succeeded")
}

func TestVideoFilterGraphWithScale(t *testing.T) {
	// Create a filter graph that scales 640x480 to 320x240
	graph, err := NewVideoFilterGraph("scale=320:240", 640, 480, PixelFormatYUV420P)
	if err != nil {
		t.Fatalf("NewVideoFilterGraph failed: %v", err)
	}
	defer graph.Close()

	t.Log("NewVideoFilterGraph with scale filter succeeded")
}

func TestVideoFilterGraphWithChain(t *testing.T) {
	// Test multiple filters in a chain
	graph, err := NewVideoFilterGraph("scale=320:240,format=yuv420p", 640, 480, PixelFormatYUV420P)
	if err != nil {
		t.Fatalf("NewVideoFilterGraph failed: %v", err)
	}
	defer graph.Close()

	t.Log("NewVideoFilterGraph with filter chain succeeded")
}

func TestFilterGraphValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     FilterGraphConfig
		wantErr bool
	}{
		{
			name:    "empty config",
			cfg:     FilterGraphConfig{},
			wantErr: true, // no video or audio params
		},
		{
			name: "video only",
			cfg: FilterGraphConfig{
				Width:    320,
				Height:   240,
				PixelFmt: PixelFormatYUV420P,
				Filters:  "null",
			},
			wantErr: false,
		},
		{
			name: "audio only",
			cfg: FilterGraphConfig{
				SampleRate: 44100,
				Channels:   2,
				SampleFmt:  SampleFormatFLTP,
				Filters:    "anull",
			},
			wantErr: false,
		},
		{
			name: "mixed video and audio",
			cfg: FilterGraphConfig{
				Width:      320,
				Height:     240,
				PixelFmt:   PixelFormatYUV420P,
				SampleRate: 44100,
				Channels:   2,
				Filters:    "null",
			},
			wantErr: true, // cannot mix
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph, err := NewFilterGraph(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					if graph != nil {
						graph.Close()
					}
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				} else {
					graph.Close()
				}
			}
		})
	}
}

func TestFilterGraphClose(t *testing.T) {
	graph, err := NewVideoFilterGraph("null", 320, 240, PixelFormatYUV420P)
	if err != nil {
		t.Fatalf("NewVideoFilterGraph failed: %v", err)
	}

	// Close should succeed
	if err := graph.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// Second close should be no-op
	if err := graph.Close(); err != nil {
		t.Errorf("second Close() failed: %v", err)
	}

	// Operations on closed graph should fail
	_, err = graph.Filter(nil)
	if err != ErrFilterGraphClosed {
		t.Errorf("Filter() on closed graph: got %v, want ErrFilterGraphClosed", err)
	}

	_, err = graph.Flush()
	if err != ErrFilterGraphClosed {
		t.Errorf("Flush() on closed graph: got %v, want ErrFilterGraphClosed", err)
	}
}

func TestAudioFilterGraphBasic(t *testing.T) {
	cfg := FilterGraphConfig{
		SampleRate: 48000,
		Channels:   2,
		SampleFmt:  SampleFormatS16,
		Filters:    "anull", // pass-through filter
	}

	graph, err := NewFilterGraph(cfg)
	if err != nil {
		t.Fatalf("NewFilterGraph failed: %v", err)
	}
	defer graph.Close()

	if graph.IsVideo() {
		t.Error("expected IsVideo() to be false")
	}
	if !graph.IsAudio() {
		t.Error("expected IsAudio() to be true")
	}

	t.Log("Audio filter graph with anull filter succeeded")
}

func TestAudioFilterGraphVolume(t *testing.T) {
	// Test volume filter
	cfg := FilterGraphConfig{
		SampleRate: 44100,
		Channels:   2,
		SampleFmt:  SampleFormatFLTP,
		Filters:    "volume=0.5", // reduce volume by half
	}

	graph, err := NewFilterGraph(cfg)
	if err != nil {
		t.Fatalf("NewFilterGraph with volume filter failed: %v", err)
	}
	defer graph.Close()

	t.Log("Audio filter graph with volume filter succeeded")
}
