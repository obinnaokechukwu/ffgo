//go:build !ios && !android && (amd64 || arm64)

// Example: colorspace - Demonstrates explicit colorspace matrix control for swscale.
//
// Usage: colorspace <input_file>
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
	v := dec.VideoStream()
	if err := dec.OpenVideoDecoder(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open video decoder: %v\n", err)
		os.Exit(1)
	}

	f, err := dec.DecodeVideo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "DecodeVideo failed: %v\n", err)
		os.Exit(1)
	}
	if f.IsNil() {
		fmt.Fprintln(os.Stderr, "No frame decoded.")
		os.Exit(1)
	}

	// Best-effort: set source color metadata (use BT.709).
	f.SetColorSpec(ffgo.ColorSpec{
		Range:     ffgo.ColorRangeMPEG,
		Space:     ffgo.ColorSpaceBT709,
		Primaries: ffgo.ColorPrimariesBT709,
		Transfer:  ffgo.ColorTransferBT709,
	})

	// Create a scaler (no resize, same pixel format) and force conversion matrix to BT.2020.
	sc, err := ffgo.NewScaler(v.Width, v.Height, v.PixelFmt, v.Width, v.Height, v.PixelFmt, ffgo.ScaleBilinear)
	if err != nil {
		fmt.Fprintf(os.Stderr, "NewScaler failed: %v\n", err)
		os.Exit(1)
	}
	defer sc.Close()

	if err := sc.SetColorspace(ffgo.ColorSpaceBT709, ffgo.ColorSpaceBT2020NCL); err != nil {
		fmt.Fprintf(os.Stderr, "SetColorspace failed: %v\n", err)
		os.Exit(1)
	}

	out, err := sc.Scale(f)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Scale failed: %v\n", err)
		os.Exit(1)
	}

	// Attach output metadata describing BT.2020/PQ (BT.2100 PQ) as an example.
	out.SetColorSpec(ffgo.ColorSpec{
		Range:     ffgo.ColorRangeMPEG,
		Space:     ffgo.ColorSpaceBT2020NCL,
		Primaries: ffgo.ColorPrimariesBT2020,
		Transfer:  ffgo.ColorTransferSMPTE2084,
	})

	fmt.Printf("Input color:  %+v\n", f.ColorSpec())
	fmt.Printf("Output color: %+v\n", out.ColorSpec())
}
