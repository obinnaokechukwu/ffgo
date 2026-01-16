//go:build !ios && !android && (amd64 || arm64)

// Example: decode - Demonstrates video decoding with ffgo
//
// Usage: decode <input_file>
//
// This example opens a video file, decodes it, and prints frame information.
package main

import (
	"fmt"
	"os"

	"github.com/obinnaokechukwu/ffgo"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <input_file>\n", os.Args[0])
		os.Exit(1)
	}

	inputFile := os.Args[1]

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

	// Open the input file
	decoder, err := ffgo.NewDecoder(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open file: %v\n", err)
		os.Exit(1)
	}
	defer decoder.Close()

	// Print file info
	fmt.Printf("\nFile: %s\n", inputFile)
	fmt.Printf("Duration: %v\n", decoder.Duration())
	fmt.Printf("Bit rate: %d bps\n", decoder.BitRate())
	fmt.Printf("Streams: %d\n", decoder.NumStreams())

	// Print video stream info
	if decoder.HasVideo() {
		video := decoder.VideoStream()
		fmt.Printf("\nVideo Stream:\n")
		fmt.Printf("  Index: %d\n", video.Index)
		fmt.Printf("  Codec: %d (%s)\n", video.CodecID, video.CodecID.String())
		fmt.Printf("  Resolution: %dx%d\n", video.Width, video.Height)
		fmt.Printf("  Pixel Format: %d\n", video.PixelFmt)
	} else {
		fmt.Println("\nNo video stream found")
	}

	// Print audio stream info
	if decoder.HasAudio() {
		audio := decoder.AudioStream()
		fmt.Printf("\nAudio Stream:\n")
		fmt.Printf("  Index: %d\n", audio.Index)
		fmt.Printf("  Codec: %d (%s)\n", audio.CodecID, audio.CodecID.String())
		fmt.Printf("  Sample Rate: %d Hz\n", audio.SampleRate)
	} else {
		fmt.Println("\nNo audio stream found")
	}

	// Decode video frames
	if decoder.HasVideo() {
		fmt.Println("\nDecoding video frames...")

		if err := decoder.OpenVideoDecoder(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open video decoder: %v\n", err)
			os.Exit(1)
		}

		frameCount := 0
		maxFrames := 30 // Limit to first 30 frames for demo

		for frameCount < maxFrames {
			frame, err := decoder.DecodeVideo()
			if err != nil {
				if ffgo.IsEOF(err) {
					break
				}
				fmt.Fprintf(os.Stderr, "Decode error: %v\n", err)
				break
			}
			if frame.IsNil() {
				continue
			}

			frameCount++
			info := ffgo.GetFrameInfo(frame)
			fmt.Printf("Frame %3d: %dx%d, pts=%d\n",
				frameCount, info.Width, info.Height, info.PTS)
		}

		fmt.Printf("\nDecoded %d frames\n", frameCount)
	}
}
