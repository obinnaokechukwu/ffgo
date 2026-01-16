//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SubtitleRenderer burns subtitles into video frames using FFmpeg's subtitles filter.
// It supports SRT, ASS/SSA, and other subtitle formats supported by FFmpeg.
type SubtitleRenderer struct {
	graph        *FilterGraph
	subtitlePath string
}

// SubtitleRendererOptions configures subtitle rendering.
type SubtitleRendererOptions struct {
	// FontName specifies the font to use for text subtitles (optional).
	FontName string

	// FontSize specifies the font size in points (optional, default: use original size).
	FontSize int

	// PrimaryColor specifies the primary text color in ABGR hex format (optional).
	// Example: "&HFFFFFF&" for white.
	PrimaryColor string

	// OutlineColor specifies the outline color in ABGR hex format (optional).
	OutlineColor string

	// OutlineWidth specifies the outline width (optional).
	OutlineWidth float64

	// MarginV specifies the vertical margin from bottom in pixels (optional).
	MarginV int

	// OriginalSize, if true, renders subtitles at their original size.
	// If false, subtitles are scaled to the video dimensions.
	OriginalSize bool

	// CharEncoding specifies the character encoding for text subtitles (e.g., "UTF-8").
	CharEncoding string

	// FontsDir specifies a directory containing fonts to use.
	FontsDir string
}

// NewSubtitleRenderer creates a renderer that burns subtitles into video frames.
// The subtitlePath should point to a subtitle file (SRT, ASS, SSA, VTT, etc.).
// Width and height should match the video dimensions.
func NewSubtitleRenderer(subtitlePath string, width, height int) (*SubtitleRenderer, error) {
	return NewSubtitleRendererWithOptions(subtitlePath, width, height, nil)
}

// NewSubtitleRendererWithOptions creates a subtitle renderer with custom options.
func NewSubtitleRendererWithOptions(subtitlePath string, width, height int, opts *SubtitleRendererOptions) (*SubtitleRenderer, error) {
	if subtitlePath == "" {
		return nil, errors.New("ffgo: subtitle path cannot be empty")
	}

	// Check if file exists
	if _, err := os.Stat(subtitlePath); err != nil {
		return nil, fmt.Errorf("ffgo: subtitle file not found: %w", err)
	}

	if width <= 0 || height <= 0 {
		return nil, errors.New("ffgo: width and height must be positive")
	}

	// Build subtitles filter string
	// FFmpeg subtitles filter syntax: subtitles=filename:options
	// The filename needs to be escaped for the filter string
	escapedPath := escapeFilterPath(subtitlePath)
	filterStr := fmt.Sprintf("subtitles=%s", escapedPath)

	// Add options if provided
	var filterOpts []string
	if opts != nil {
		if opts.OriginalSize {
			filterOpts = append(filterOpts, "original_size=true")
		}
		if opts.CharEncoding != "" {
			filterOpts = append(filterOpts, fmt.Sprintf("charenc=%s", opts.CharEncoding))
		}
		if opts.FontsDir != "" {
			escapedFontsDir := escapeFilterPath(opts.FontsDir)
			filterOpts = append(filterOpts, fmt.Sprintf("fontsdir=%s", escapedFontsDir))
		}

		// Force_style option for SRT/text subtitles
		// In libass/ASS format, styles are comma-separated
		// For FFmpeg filter, we need to escape commas inside the force_style value
		var styleOpts []string
		if opts.FontName != "" {
			styleOpts = append(styleOpts, fmt.Sprintf("FontName=%s", opts.FontName))
		}
		if opts.FontSize > 0 {
			styleOpts = append(styleOpts, fmt.Sprintf("FontSize=%d", opts.FontSize))
		}
		if opts.PrimaryColor != "" {
			styleOpts = append(styleOpts, fmt.Sprintf("PrimaryColour=%s", opts.PrimaryColor))
		}
		if opts.OutlineColor != "" {
			styleOpts = append(styleOpts, fmt.Sprintf("OutlineColour=%s", opts.OutlineColor))
		}
		if opts.OutlineWidth > 0 {
			styleOpts = append(styleOpts, fmt.Sprintf("Outline=%g", opts.OutlineWidth))
		}
		if opts.MarginV > 0 {
			styleOpts = append(styleOpts, fmt.Sprintf("MarginV=%d", opts.MarginV))
		}

		if len(styleOpts) > 0 {
			// Join with comma, then quote the entire force_style value with single quotes
			// to allow commas inside (ASS style uses comma-separated options)
			forceStyle := strings.Join(styleOpts, ",")
			filterOpts = append(filterOpts, fmt.Sprintf("force_style='%s'", forceStyle))
		}
	}

	if len(filterOpts) > 0 {
		filterStr += ":" + strings.Join(filterOpts, ":")
	}

	// Create video filter graph with subtitles filter
	graph, err := NewVideoFilterGraph(filterStr, width, height, PixelFormatYUV420P)
	if err != nil {
		return nil, fmt.Errorf("ffgo: failed to create subtitle filter: %w", err)
	}

	return &SubtitleRenderer{
		graph:        graph,
		subtitlePath: subtitlePath,
	}, nil
}

// Render burns the subtitle at the current frame's PTS onto the frame.
// The frame must have valid PTS for correct subtitle timing.
//
// Returns a new frame with subtitles burned in. The returned frame is owned
// by the caller and must be freed.
func (r *SubtitleRenderer) Render(frame Frame) (Frame, error) {
	if r.graph == nil {
		return Frame{}, errors.New("ffgo: subtitle renderer is closed")
	}

	frames, err := r.graph.Filter(&frame)
	if err != nil {
		return Frame{}, err
	}

	if len(frames) == 0 {
		return Frame{}, errors.New("ffgo: no output frame from subtitle filter")
	}

	// Return the first frame (subtitles filter outputs one frame per input).
	// The returned frame is owned by the caller and must be freed.
	out := *frames[0]
	for i := 1; i < len(frames); i++ {
		_ = frames[i].Free()
	}
	return out, nil
}

// Close releases all resources.
func (r *SubtitleRenderer) Close() error {
	if r.graph != nil {
		err := r.graph.Close()
		r.graph = nil
		return err
	}
	return nil
}

// SubtitlePath returns the path to the subtitle file being used.
func (r *SubtitleRenderer) SubtitlePath() string {
	return r.subtitlePath
}

// escapeFilterPath escapes a file path for use in FFmpeg filter strings.
// Special characters like : and \ need to be escaped.
func escapeFilterPath(path string) string {
	// Convert to absolute path if relative
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	// Escape special characters used in FFmpeg filter syntax
	// : is used as option separator, so escape it
	// ' is used to quote values, so escape it
	// \ is escape character, so escape it first
	result := absPath
	result = strings.ReplaceAll(result, "\\", "\\\\")
	result = strings.ReplaceAll(result, ":", "\\:")
	result = strings.ReplaceAll(result, "'", "\\'")
	result = strings.ReplaceAll(result, "[", "\\[")
	result = strings.ReplaceAll(result, "]", "\\]")

	return result
}

// SubtitleFormat represents subtitle file formats.
type SubtitleFormat int

const (
	SubtitleFormatSRT    SubtitleFormat = iota // SubRip text format
	SubtitleFormatASS                          // Advanced SubStation Alpha
	SubtitleFormatSSA                          // SubStation Alpha
	SubtitleFormatWebVTT                       // Web Video Text Tracks
	SubtitleFormatMOVText                      // QuickTime text
)

// String returns the file extension for the subtitle format.
func (f SubtitleFormat) String() string {
	switch f {
	case SubtitleFormatSRT:
		return "srt"
	case SubtitleFormatASS:
		return "ass"
	case SubtitleFormatSSA:
		return "ssa"
	case SubtitleFormatWebVTT:
		return "vtt"
	case SubtitleFormatMOVText:
		return "mov_text"
	default:
		return "unknown"
	}
}
