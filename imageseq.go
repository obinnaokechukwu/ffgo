//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"fmt"

	"github.com/obinnaokechukwu/ffgo/avutil"
)

// ImageSequenceConfig configures image sequence reading or writing.
type ImageSequenceConfig struct {
	// Pattern is the file pattern with printf-style format specifier.
	// Examples: "frame_%04d.png", "image-%03d.jpg", "out%d.bmp"
	Pattern string

	// StartNumber is the first frame number (default: 0).
	StartNumber int

	// FrameRate is the frame rate for the sequence.
	// For decoding, this determines the duration of each frame.
	// For encoding, this is the output frame rate.
	FrameRate avutil.Rational
}

// NewImageSequenceDecoder creates a decoder that reads an image sequence as video.
// The pattern should use printf-style format specifier (e.g., "frame_%04d.png").
// The sequence is decoded as a video stream with the specified frame rate.
func NewImageSequenceDecoder(config ImageSequenceConfig) (*Decoder, error) {
	if config.Pattern == "" {
		return nil, fmt.Errorf("ffgo: image sequence pattern cannot be empty")
	}

	opts := map[string]string{
		"pattern_type": "sequence", // Required for glob-style patterns
	}

	if config.StartNumber > 0 {
		opts["start_number"] = fmt.Sprintf("%d", config.StartNumber)
	}

	if config.FrameRate.Num > 0 && config.FrameRate.Den > 0 {
		opts["framerate"] = fmt.Sprintf("%d/%d", config.FrameRate.Num, config.FrameRate.Den)
	}

	return NewDecoderWithOptions(config.Pattern, &DecoderOptions{
		Format:    "image2",
		AVOptions: opts,
	})
}

// NewImageSequenceEncoder creates an encoder that writes video as an image sequence.
// The pattern should use printf-style format specifier (e.g., "frame_%04d.png").
// The output format is determined by the file extension in the pattern.
func NewImageSequenceEncoder(config ImageSequenceConfig, width, height int, pixFmt PixelFormat) (*Encoder, error) {
	if config.Pattern == "" {
		return nil, fmt.Errorf("ffgo: image sequence pattern cannot be empty")
	}

	// Determine codec from pattern extension
	codec := "png" // default
	if len(config.Pattern) > 4 {
		ext := config.Pattern[len(config.Pattern)-4:]
		switch ext {
		case ".jpg", ".JPG":
			codec = "mjpeg"
		case "jpeg", "JPEG":
			codec = "mjpeg"
		case ".bmp", ".BMP":
			codec = "bmp"
		case ".png", ".PNG":
			codec = "png"
		}
	}

	frameRate := 25 // Default 25 fps
	if config.FrameRate.Num > 0 && config.FrameRate.Den > 0 {
		frameRate = int(config.FrameRate.Num / config.FrameRate.Den)
		if frameRate <= 0 {
			frameRate = 25
		}
	}

	// Map codec name to codec ID
	var codecID CodecID
	switch codec {
	case "mjpeg":
		codecID = CodecIDMJPEG
	case "bmp":
		codecID = CodecIDBMP
	case "png":
		codecID = CodecIDPNG
	default:
		codecID = CodecIDPNG
	}

	return NewEncoder(config.Pattern, EncoderConfig{
		Width:       width,
		Height:      height,
		PixelFormat: pixFmt,
		FrameRate:   frameRate,
		CodecID:     codecID,
	})
}
