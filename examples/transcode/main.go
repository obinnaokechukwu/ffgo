//go:build !ios && !android && (amd64 || arm64)

// Example: transcode - Demonstrates video transcoding with ffgo
//
// Usage: transcode <input_file> <output_file>
//
// This example reads a video file and transcodes it to H.264.
package main

import (
	"fmt"
	"os"

	"github.com/obinnaokechukwu/ffgo"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <input_file> <output_file>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s input.mp4 output.mp4\n", os.Args[0])
		os.Exit(1)
	}

	inputFile := os.Args[1]
	outputFile := os.Args[2]

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

	// Open input file
	fmt.Printf("\nOpening input: %s\n", inputFile)
	decoder, err := ffgo.NewDecoder(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open input: %v\n", err)
		os.Exit(1)
	}
	defer decoder.Close()

	// Check for video stream
	if !decoder.HasVideo() {
		fmt.Fprintf(os.Stderr, "No video stream found in input\n")
		os.Exit(1)
	}

	videoInfo := decoder.VideoStream()
	fmt.Printf("Input video: %dx%d, codec=%s\n",
		videoInfo.Width, videoInfo.Height, videoInfo.CodecID.String())
	fmt.Printf("Duration: %v\n", decoder.DurationTime())

	if decoder.HasAudio() {
		audioInfo := decoder.AudioStream()
		fmt.Printf("Input audio: %d Hz, %d channels, codec=%s\n",
			audioInfo.SampleRate, audioInfo.Channels, audioInfo.CodecID.String())
	}

	// Create encoder with same dimensions as input
	fmt.Printf("\nCreating output: %s\n", outputFile)
	encoder, err := ffgo.NewEncoder(outputFile, ffgo.EncoderConfig{
		Width:       videoInfo.Width,
		Height:      videoInfo.Height,
		PixelFormat: ffgo.PixelFormatYUV420P,
		CodecID:     ffgo.CodecIDH264,
		BitRate:     2000000, // 2 Mbps
		FrameRate:   30,
		GOPSize:     12,
		MaxBFrames:  0,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create encoder: %v\n", err)
		os.Exit(1)
	}
	defer encoder.Close()

	// Create scaler if pixel format conversion is needed
	var scaler *ffgo.Scaler
	if videoInfo.PixelFmt != ffgo.PixelFormatYUV420P {
		fmt.Printf("Creating scaler: %v -> YUV420P\n", videoInfo.PixelFmt)
		scaler, err = ffgo.NewScaler(ffgo.ScalerConfig{
			SrcWidth:  videoInfo.Width,
			SrcHeight: videoInfo.Height,
			SrcFormat: videoInfo.PixelFmt,
			DstWidth:  videoInfo.Width,
			DstHeight: videoInfo.Height,
			DstFormat: ffgo.PixelFormatYUV420P,
			Flags:     ffgo.ScaleBilinear,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create scaler: %v\n", err)
			os.Exit(1)
		}
		defer scaler.Close()
	}

	fmt.Println("\nTranscoding...")

	// Transcode frames
	frameCount := 0
	for {
		// Decode next video frame
		frame, err := decoder.DecodeVideo()
		if err != nil {
			if ffgo.IsEOF(err) {
				break
			}
			fmt.Fprintf(os.Stderr, "Decode error: %v\n", err)
			os.Exit(1)
		}
		if frame == nil {
			break // EOF
		}

		// Scale if necessary
		var encFrame ffgo.Frame = frame
		if scaler != nil {
			encFrame, err = scaler.Scale(frame)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Scale error: %v\n", err)
				os.Exit(1)
			}
		}

		// Encode frame
		if err := encoder.WriteFrame(encFrame); err != nil {
			fmt.Fprintf(os.Stderr, "Encode error: %v\n", err)
			os.Exit(1)
		}

		frameCount++

		// Progress indicator
		if frameCount%30 == 0 {
			fmt.Printf("\rFrames processed: %d", frameCount)
		}
	}

	fmt.Printf("\rFrames processed: %d\n", frameCount)

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

	fmt.Printf("\nTranscode complete!\n")
	fmt.Printf("Output: %s (%d bytes)\n", outputFile, info.Size())
	fmt.Printf("Transcoded %d frames\n", frameCount)
}
