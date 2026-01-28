//go:build !ios && !android && (amd64 || arm64)

// Example: timing - Demonstrates frame timing helpers.
//
// Usage: timing <input_file>
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
	in := os.Args[1]

	if err := ffgo.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize FFmpeg: %v\n", err)
		os.Exit(1)
	}

	dec, err := ffgo.NewDecoder(in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open input: %v\n", err)
		os.Exit(1)
	}
	defer dec.Close()

	if !dec.HasVideo() {
		fmt.Fprintln(os.Stderr, "No video stream.")
		os.Exit(1)
	}

	fps, err := ffgo.FrameRateDetect(dec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FrameRateDetect failed: %v\n", err)
		os.Exit(1)
	}
	tb := dec.VideoStream().TimeBase
	fmt.Printf("Detected fps: %0.3f\n", fps)
	fmt.Printf("Time base: %d/%d\n", tb.Num, tb.Den)

	ts := ffgo.GenerateTimestamps(10, tb, fps)
	fmt.Printf("First 10 PTS: %v\n", ts)
}
