//go:build !ios && !android && (amd64 || arm64)

// Example: concat - Demonstrates seamless concatenation using FFmpeg's concat demuxer.
//
// Usage: concat <input1> <input2> [input3...]
package main

import (
	"fmt"
	"os"

	"github.com/obinnaokechukwu/ffgo"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <input1> <input2> [input3...]\n", os.Args[0])
		os.Exit(1)
	}

	if err := ffgo.Init(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize FFmpeg: %v\n", err)
		os.Exit(1)
	}

	files := os.Args[1:]
	dec, err := ffgo.NewConcatDecoder(files, ffgo.WithConcatSafeMode(false))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open concat decoder: %v\n", err)
		os.Exit(1)
	}
	defer dec.Close()

	fmt.Printf("Concat duration: %v\n", dec.Duration())
	fmt.Printf("Streams: %d\n", dec.NumStreams())
	if dec.HasVideo() {
		v := dec.VideoStream()
		fmt.Printf("Video: %s %dx%d tb=%d/%d\n", v.CodecName, v.Width, v.Height, v.TimeBase.Num, v.TimeBase.Den)
	}
	if dec.HasAudio() {
		a := dec.AudioStream()
		fmt.Printf("Audio: %s %d Hz ch=%d tb=%d/%d\n", a.CodecName, a.SampleRate, a.Channels, a.TimeBase.Num, a.TimeBase.Den)
	}

	// Decode a small sample of frames to show continuous timestamps.
	if !dec.HasVideo() {
		fmt.Println("No video stream.")
		return
	}
	if err := dec.OpenVideoDecoder(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open video decoder: %v\n", err)
		os.Exit(1)
	}

	tb := ffgo.NewRational(1, 1)
	if dec.VideoStream() != nil {
		tb = dec.VideoStream().TimeBase
	}

	const maxFrames = 60
	for i := 0; i < maxFrames; i++ {
		f, err := dec.DecodeVideo()
		if err != nil {
			fmt.Fprintf(os.Stderr, "DecodeVideo failed: %v\n", err)
			os.Exit(1)
		}
		if f.IsNil() {
			break
		}
		info := ffgo.GetFrameInfo(f)
		secs := float64(info.PTS) * float64(tb.Num) / float64(tb.Den)
		fmt.Printf("Frame %3d: pts=%d (%0.3fs)\n", i+1, info.PTS, secs)
	}
}
