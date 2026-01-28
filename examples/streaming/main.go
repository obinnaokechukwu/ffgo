//go:build !ios && !android && (amd64 || arm64)

// Example: streaming - Demonstrates streaming output helpers (RTMP/UDP/RTP).
//
// Usage: streaming <input_file> <output_url>
//
// Example:
//
//	streaming input.mp4 rtmp://localhost/live/stream
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/obinnaokechukwu/ffgo"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <input_file> <output_url>\n", os.Args[0])
		os.Exit(1)
	}

	in := os.Args[1]
	outURL := os.Args[2]

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
		fmt.Fprintln(os.Stderr, "Input has no video stream.")
		os.Exit(1)
	}
	v := dec.VideoStream()
	if err := dec.OpenVideoDecoder(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open video decoder: %v\n", err)
		os.Exit(1)
	}

	enc, err := ffgo.NewStreamingEncoder(outURL,
		ffgo.WithVideoEncoder(&ffgo.VideoEncoderConfig{
			Codec:       ffgo.CodecIDH264,
			Width:       v.Width,
			Height:      v.Height,
			FrameRate:   v.FrameRate,
			PixelFormat: v.PixelFmt,
			RateControl: ffgo.RateControlCRF,
			CRF:         23,
			Preset:      ffgo.PresetVeryfast,
		}),
		ffgo.WithStreamingOptions(&ffgo.StreamingOptions{
			Timeout:  10 * time.Second,
			MaxDelay: 500 * time.Millisecond,
		}),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create streaming encoder: %v\n", err)
		os.Exit(1)
	}
	defer enc.Close()

	// Stream a short sample (first N frames) for demonstration.
	const maxFrames = 150
	for i := 0; i < maxFrames; i++ {
		f, err := dec.DecodeVideo()
		if err != nil {
			fmt.Fprintf(os.Stderr, "DecodeVideo failed: %v\n", err)
			os.Exit(1)
		}
		if f.IsNil() {
			break
		}
		if err := enc.WriteVideoFrame(f); err != nil {
			fmt.Fprintf(os.Stderr, "WriteVideoFrame failed: %v\n", err)
			os.Exit(1)
		}
	}

	if err := enc.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "Close failed: %v\n", err)
		os.Exit(1)
	}
}
