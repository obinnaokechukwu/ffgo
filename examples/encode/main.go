//go:build !ios && !android && (amd64 || arm64)

// Example: encode - Demonstrates video encoding with ffgo
//
// Usage: encode <output_file>
//
// This example creates a test video file with animated frames.
package main

import (
	"fmt"
	"os"

	"github.com/obinnaokechukwu/ffgo"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <output_file>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s output.mp4\n", os.Args[0])
		os.Exit(1)
	}

	outputFile := os.Args[1]

	// Initialize FFmpeg
	if err := ffgo.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize FFmpeg: %v\n", err)
		os.Exit(1)
	}

	// Print version info
	avutil, avcodec, avformat := ffgo.Version()
	fmt.Printf("FFmpeg versions: avutil=%d.%d.%d, avcodec=%d.%d.%d, avformat=%d.%d.%d\n",
		avutil>>16, (avutil>>8)&0xFF, avutil&0xFF,
		avcodec>>16, (avcodec>>8)&0xFF, avcodec&0xFF,
		avformat>>16, (avformat>>8)&0xFF, avformat&0xFF)

	// Create encoder
	width, height := 320, 240
	frameRate := 30
	duration := 3 // seconds
	totalFrames := duration * frameRate

	fmt.Printf("\nCreating encoder: %dx%d, %d fps, %d frames\n", width, height, frameRate, totalFrames)

	encoder, err := ffgo.NewEncoder(outputFile, ffgo.EncoderConfig{
		Width:       width,
		Height:      height,
		PixelFormat: ffgo.PixelFormatYUV420P,
		CodecID:     ffgo.CodecIDH264,
		BitRate:     1000000, // 1 Mbps
		FrameRate:   frameRate,
		GOPSize:     12,
		MaxBFrames:  0,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create encoder: %v\n", err)
		os.Exit(1)
	}
	defer encoder.Close()

	// Allocate frame
	frame := ffgo.FrameAlloc()
	if frame.IsNil() {
		fmt.Fprintf(os.Stderr, "Failed to allocate frame\n")
		os.Exit(1)
	}
	defer func() { _ = ffgo.FrameFree(&frame) }()

	// Set up frame
	ffgo.AVUtil.SetFrameWidth(frame, int32(width))
	ffgo.AVUtil.SetFrameHeight(frame, int32(height))
	ffgo.AVUtil.SetFrameFormat(frame, int32(ffgo.PixelFormatYUV420P))

	// Allocate frame buffer
	if err := ffgo.AVUtil.FrameGetBuffer(frame, 0); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to allocate frame buffer: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Encoding frames...")

	// Encode frames
	for i := 0; i < totalFrames; i++ {
		// Make frame writable
		if err := ffgo.AVUtil.FrameMakeWritable(frame); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to make frame writable: %v\n", err)
			os.Exit(1)
		}

		// Fill with test pattern
		fillTestPattern(frame, i, width, height)

		// Encode frame
		if err := encoder.WriteFrame(frame); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode frame %d: %v\n", i, err)
			os.Exit(1)
		}

		// Progress indicator
		if (i+1)%30 == 0 || i == totalFrames-1 {
			fmt.Printf("\rFrame %d/%d (%d%%)", i+1, totalFrames, (i+1)*100/totalFrames)
		}
	}

	fmt.Println()

	// Close encoder (flushes and writes trailer)
	if err := encoder.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to finalize output: %v\n", err)
		os.Exit(1)
	}

	// Verify output file
	info, err := os.Stat(outputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Output file not found: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nOutput: %s (%d bytes)\n", outputFile, info.Size())
	fmt.Printf("Encoded %d frames, %.1f seconds at %d fps\n", totalFrames, float64(totalFrames)/float64(frameRate), frameRate)
}

// fillTestPattern fills a YUV420P frame with an animated test pattern
func fillTestPattern(frame ffgo.Frame, frameNum, width, height int) {
	fw := ffgo.WrapFrame(frame, ffgo.MediaTypeVideo)
	if fw == nil {
		return
	}
	yPlane := fw.Data(0)
	uPlane := fw.Data(1)
	vPlane := fw.Data(2)
	yStride := fw.Linesize(0)
	uStride := fw.Linesize(1)
	vStride := fw.Linesize(2)

	// Y plane - animated gradient
	t := float64(frameNum) * 0.1

	for y := 0; y < height; y++ {
		row := y * yStride
		for x := 0; x < width; x++ {
			// Moving diagonal bars
			fx := float64(x) / float64(width)
			fy := float64(y) / float64(height)

			// Create animated pattern
			pattern := (fx + fy + t) * 3.0
			val := byte(int(pattern*255) % 256)

			// Add some brightness variation
			if int(pattern)%2 == 0 {
				val = 255 - val
			}

			yPlane[row+x] = val
		}
	}

	// U and V planes (half size for YUV420P)
	uvHeight := height / 2
	uvWidth := width / 2

	// U plane - subtle color variation
	for y := 0; y < uvHeight; y++ {
		row := y * uStride
		for x := 0; x < uvWidth; x++ {
			// Subtle hue shift
			val := byte(128 + int(float64(frameNum)*0.5)%50 - 25)
			uPlane[row+x] = val
		}
	}

	// V plane - subtle color variation
	for y := 0; y < uvHeight; y++ {
		row := y * vStride
		for x := 0; x < uvWidth; x++ {
			// Subtle hue shift (opposite direction)
			val := byte(128 - int(float64(frameNum)*0.5)%50 + 25)
			vPlane[row+x] = val
		}
	}
}
