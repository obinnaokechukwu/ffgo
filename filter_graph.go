//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"github.com/obinnaokechukwu/ffgo/avfilter"
	"github.com/obinnaokechukwu/ffgo/avutil"
)

// FilterGraph represents a filter processing pipeline for video or audio.
// It abstracts the complexity of FFmpeg filter graphs into a simple push/pull interface.
type FilterGraph struct {
	graph      avfilter.Graph
	bufferSrc  avfilter.Context
	bufferSink avfilter.Context
	outFrame   unsafe.Pointer // reusable output frame
	isVideo    bool
	closed     bool

	// Input format (for reference)
	srcWidth  int
	srcHeight int
	srcPixFmt PixelFormat
}

// FilterGraphConfig configures a filter graph.
type FilterGraphConfig struct {
	// Video parameters (required for video filters)
	Width     int
	Height    int
	PixelFmt  PixelFormat
	TimeBase  Rational // defaults to 1/90000
	FrameRate Rational // optional
	SAR       Rational // sample aspect ratio, defaults to 1/1

	// Audio parameters (required for audio filters)
	SampleRate    int
	Channels      int
	ChannelLayout ChannelLayout
	SampleFmt     SampleFormat

	// Filter string (e.g., "scale=320:240,transpose=1")
	Filters string
}

// ErrFilterGraphClosed is returned when operating on a closed filter graph.
var ErrFilterGraphClosed = errors.New("ffgo: filter graph is closed")

// NewVideoFilterGraph creates a video filter graph with the specified parameters.
// The filters string uses FFmpeg's filter graph syntax (e.g., "scale=320:240,transpose=1").
//
// Example:
//
//	graph, err := ffgo.NewVideoFilterGraph("scale=640:480", 1920, 1080, ffgo.PixelFormatYUV420P)
//	if err != nil {
//	    return err
//	}
//	defer graph.Close()
func NewVideoFilterGraph(filters string, width, height int, pixFmt PixelFormat) (*FilterGraph, error) {
	return NewFilterGraph(FilterGraphConfig{
		Width:    width,
		Height:   height,
		PixelFmt: pixFmt,
		Filters:  filters,
	})
}

// NewFilterGraph creates a filter graph from the given configuration.
func NewFilterGraph(cfg FilterGraphConfig) (*FilterGraph, error) {
	if err := avfilter.Init(); err != nil {
		return nil, fmt.Errorf("ffgo: failed to initialize avfilter: %w", err)
	}

	// Determine if this is video or audio based on config
	isVideo := cfg.Width > 0 && cfg.Height > 0
	isAudio := cfg.SampleRate > 0 && cfg.Channels > 0

	if !isVideo && !isAudio {
		return nil, errors.New("ffgo: must specify either video (Width, Height) or audio (SampleRate, Channels) parameters")
	}

	if isVideo && isAudio {
		return nil, errors.New("ffgo: cannot mix video and audio parameters; create separate filter graphs")
	}

	g := &FilterGraph{
		isVideo:   isVideo,
		srcWidth:  cfg.Width,
		srcHeight: cfg.Height,
		srcPixFmt: cfg.PixelFmt,
	}

	// Allocate filter graph
	g.graph = avfilter.GraphAlloc()
	if g.graph == nil {
		return nil, errors.New("ffgo: failed to allocate filter graph")
	}

	var err error
	if isVideo {
		err = g.setupVideoFilters(cfg)
	} else {
		err = g.setupAudioFilters(cfg)
	}

	if err != nil {
		avfilter.GraphFree(&g.graph)
		return nil, err
	}

	// Allocate output frame
	g.outFrame = avutil.FrameAlloc()
	if g.outFrame == nil {
		avfilter.GraphFree(&g.graph)
		return nil, errors.New("ffgo: failed to allocate output frame")
	}

	runtime.SetFinalizer(g, (*FilterGraph).cleanup)
	return g, nil
}

func (g *FilterGraph) setupVideoFilters(cfg FilterGraphConfig) error {
	// Default time base
	timeBase := cfg.TimeBase
	if timeBase.Num == 0 {
		timeBase = Rational{Num: 1, Den: 90000}
	}

	// Default SAR
	sar := cfg.SAR
	if sar.Num == 0 {
		sar = Rational{Num: 1, Den: 1}
	}

	// Create buffersrc
	bufferSrc := avfilter.GetByName("buffer")
	if bufferSrc == nil {
		return errors.New("ffgo: buffer filter not found")
	}

	srcArgs := fmt.Sprintf("video_size=%dx%d:pix_fmt=%d:time_base=%d/%d:pixel_aspect=%d/%d",
		cfg.Width, cfg.Height, int(cfg.PixelFmt),
		timeBase.Num, timeBase.Den,
		sar.Num, sar.Den)

	var err error
	g.bufferSrc, err = avfilter.GraphCreateFilter(g.graph, bufferSrc, "in", srcArgs)
	if err != nil {
		return fmt.Errorf("ffgo: failed to create buffersrc: %w", err)
	}

	// Create buffersink
	bufferSink := avfilter.GetByName("buffersink")
	if bufferSink == nil {
		return errors.New("ffgo: buffersink filter not found")
	}

	g.bufferSink, err = avfilter.GraphCreateFilter(g.graph, bufferSink, "out", "")
	if err != nil {
		return fmt.Errorf("ffgo: failed to create buffersink: %w", err)
	}

	// Link filters - use manual linking approach which is more reliable
	if cfg.Filters == "" || cfg.Filters == "null" {
		// No filters or null filter - link src directly to sink
		if err := avfilter.Link(g.bufferSrc, 0, g.bufferSink, 0); err != nil {
			return fmt.Errorf("ffgo: failed to link src to sink: %w", err)
		}
	} else {
		// For filter chains, we create intermediate filters manually
		// This is more reliable than using GraphParse2 which has linking issues
		if err := g.linkFilterChain(cfg.Filters); err != nil {
			return err
		}
	}

	// Configure the graph
	if err := avfilter.GraphConfig(g.graph); err != nil {
		return fmt.Errorf("ffgo: failed to configure filter graph: %w", err)
	}

	return nil
}

// linkFilterChain parses a filter string and creates/links filters manually.
// Filter string format: "filter1=args1,filter2=args2,..."
func (g *FilterGraph) linkFilterChain(filters string) error {
	// Parse filters - split by comma (simple parsing, doesn't handle nested commas)
	filterList := parseFilterChain(filters)

	if len(filterList) == 0 {
		// Empty filter list, link src to sink directly
		return avfilter.Link(g.bufferSrc, 0, g.bufferSink, 0)
	}

	// Create filter contexts
	var filterCtxs []avfilter.Context
	for i, f := range filterList {
		filter := avfilter.GetByName(f.name)
		if filter == nil {
			return fmt.Errorf("ffgo: filter %q not found", f.name)
		}

		ctx, err := avfilter.GraphCreateFilter(g.graph, filter, fmt.Sprintf("f%d", i), f.args)
		if err != nil {
			return fmt.Errorf("ffgo: failed to create filter %q: %w", f.name, err)
		}
		filterCtxs = append(filterCtxs, ctx)
	}

	// Link: buffersrc -> filter0 -> filter1 -> ... -> buffersink
	prevCtx := g.bufferSrc
	for i, ctx := range filterCtxs {
		if err := avfilter.Link(prevCtx, 0, ctx, 0); err != nil {
			return fmt.Errorf("ffgo: failed to link filter chain at filter %d: %w", i, err)
		}
		prevCtx = ctx
	}

	// Link last filter to buffersink
	if err := avfilter.Link(prevCtx, 0, g.bufferSink, 0); err != nil {
		return fmt.Errorf("ffgo: failed to link to buffersink: %w", err)
	}

	return nil
}

// filterSpec represents a parsed filter specification
type filterSpec struct {
	name string
	args string
}

// parseFilterChain parses a filter chain string like "scale=320:240,format=yuv420p"
func parseFilterChain(filters string) []filterSpec {
	var result []filterSpec

	// Simple parsing - split by comma then by =
	// This doesn't handle complex cases like nested brackets
	parts := splitFilterChain(filters)

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		var spec filterSpec
		if idx := strings.Index(part, "="); idx > 0 {
			spec.name = part[:idx]
			spec.args = part[idx+1:]
		} else {
			spec.name = part
			spec.args = ""
		}
		result = append(result, spec)
	}

	return result
}

// splitFilterChain splits by comma but respects brackets and quotes
func splitFilterChain(s string) []string {
	var result []string
	var current strings.Builder
	depth := 0
	inQuote := false

	for _, c := range s {
		switch c {
		case '\'':
			inQuote = !inQuote
			current.WriteRune(c)
		case '[', '(':
			depth++
			current.WriteRune(c)
		case ']', ')':
			depth--
			current.WriteRune(c)
		case ',':
			if depth == 0 && !inQuote {
				result = append(result, current.String())
				current.Reset()
			} else {
				current.WriteRune(c)
			}
		default:
			current.WriteRune(c)
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

func (g *FilterGraph) setupAudioFilters(cfg FilterGraphConfig) error {
	// Create abuffer (audio buffer source)
	abufferSrc := avfilter.GetByName("abuffer")
	if abufferSrc == nil {
		return errors.New("ffgo: abuffer filter not found")
	}

	// Get channel layout string
	layoutStr := "stereo"
	switch cfg.Channels {
	case 1:
		layoutStr = "mono"
	case 2:
		layoutStr = "stereo"
	case 6:
		layoutStr = "5.1"
	case 8:
		layoutStr = "7.1"
	}

	// Note: sample_fmt format string uses name like "fltp", "s16", etc.
	sampleFmtName := getSampleFormatName(cfg.SampleFmt)

	srcArgs := fmt.Sprintf("sample_rate=%d:sample_fmt=%s:channel_layout=%s",
		cfg.SampleRate, sampleFmtName, layoutStr)

	var err error
	g.bufferSrc, err = avfilter.GraphCreateFilter(g.graph, abufferSrc, "in", srcArgs)
	if err != nil {
		return fmt.Errorf("ffgo: failed to create abuffer: %w", err)
	}

	// Create abuffersink (audio buffer sink)
	abufferSink := avfilter.GetByName("abuffersink")
	if abufferSink == nil {
		return errors.New("ffgo: abuffersink filter not found")
	}

	g.bufferSink, err = avfilter.GraphCreateFilter(g.graph, abufferSink, "out", "")
	if err != nil {
		return fmt.Errorf("ffgo: failed to create abuffersink: %w", err)
	}

	// Parse and link filters
	if cfg.Filters != "" && cfg.Filters != "anull" {
		inputs, outputs, err := avfilter.GraphParse2(g.graph, cfg.Filters)
		if err != nil {
			return fmt.Errorf("ffgo: failed to parse filter graph: %w", err)
		}

		// Link src -> first filter
		if outputs != nil {
			outputCtx := avfilter.InOutGetFilterCtx(outputs)
			outputPad := avfilter.InOutGetPadIdx(outputs)
			if err := avfilter.Link(g.bufferSrc, 0, outputCtx, uint32(outputPad)); err != nil {
				avfilter.InOutFree(&inputs)
				avfilter.InOutFree(&outputs)
				return fmt.Errorf("ffgo: failed to link abuffer: %w", err)
			}
		}

		// Link last filter -> sink
		if inputs != nil {
			inputCtx := avfilter.InOutGetFilterCtx(inputs)
			inputPad := avfilter.InOutGetPadIdx(inputs)
			if err := avfilter.Link(inputCtx, uint32(inputPad), g.bufferSink, 0); err != nil {
				avfilter.InOutFree(&inputs)
				avfilter.InOutFree(&outputs)
				return fmt.Errorf("ffgo: failed to link abuffersink: %w", err)
			}
		}

		avfilter.InOutFree(&inputs)
		avfilter.InOutFree(&outputs)
	} else {
		// Direct link
		if err := avfilter.Link(g.bufferSrc, 0, g.bufferSink, 0); err != nil {
			return fmt.Errorf("ffgo: failed to link src to sink: %w", err)
		}
	}

	// Configure
	if err := avfilter.GraphConfig(g.graph); err != nil {
		return fmt.Errorf("ffgo: failed to configure filter graph: %w", err)
	}

	return nil
}

func getSampleFormatName(fmt SampleFormat) string {
	switch fmt {
	case SampleFormatU8:
		return "u8"
	case SampleFormatS16:
		return "s16"
	case SampleFormatS32:
		return "s32"
	case SampleFormatFlt:
		return "flt"
	case SampleFormatDbl:
		return "dbl"
	case SampleFormatU8P:
		return "u8p"
	case SampleFormatS16P:
		return "s16p"
	case SampleFormatS32P:
		return "s32p"
	case SampleFormatFLTP:
		return "fltp"
	case SampleFormatDblP:
		return "dblp"
	case SampleFormatS64:
		return "s64"
	case SampleFormatS64P:
		return "s64p"
	default:
		return "s16"
	}
}

// Filter processes a frame through the filter graph and returns filtered frames.
// The input frame is not modified. Multiple output frames may be returned for
// filters that change frame timing (e.g., framerate conversion).
//
// Example:
//
//	filtered, err := graph.Filter(inputFrame)
//	if err != nil {
//	    return err
//	}
//	for _, frame := range filtered {
//	    // Process filtered frame
//	}
func (g *FilterGraph) Filter(frame *Frame) ([]*Frame, error) {
	if g.closed {
		return nil, ErrFilterGraphClosed
	}

	// Push frame to buffersrc
	var framePtr unsafe.Pointer
	if frame != nil {
		framePtr = frame.ptr
	}

	if err := avfilter.BufferSrcAddFrameFlags(g.bufferSrc, framePtr, avfilter.AV_BUFFERSRC_FLAG_KEEP_REF); err != nil {
		return nil, fmt.Errorf("ffgo: failed to push frame to filter: %w", err)
	}
	if frame != nil {
		runtime.KeepAlive(frame)
	}

	// Pull frames from buffersink
	var frames []*Frame
	for {
		avutil.FrameUnref(g.outFrame)
		ret := avfilter.BufferSinkGetFrame(g.bufferSink, g.outFrame)
		if ret == avutil.AVERROR_EAGAIN || ret == avutil.AVERROR_EOF {
			break
		}
		if ret < 0 {
			return frames, fmt.Errorf("ffgo: failed to get frame from filter: %d", ret)
		}

		// Clone the frame (avutil.FrameClone allocates a new frame and copies refs)
		newFrame := avutil.FrameAlloc()
		if newFrame == nil {
			return frames, errors.New("ffgo: failed to allocate output frame")
		}
		avutil.FrameRef(newFrame, g.outFrame)
		// Allocate a Frame slot to hold the pointer (since we return []*Frame)
		framePtr := new(Frame)
		*framePtr = Frame{ptr: newFrame, owned: true}
		frames = append(frames, framePtr)
	}

	return frames, nil
}

// Flush drains any remaining frames from the filter graph.
// Call this after sending all input frames to get any buffered output.
func (g *FilterGraph) Flush() ([]*Frame, error) {
	if g.closed {
		return nil, ErrFilterGraphClosed
	}

	// Send NULL frame to signal EOF
	if err := avfilter.BufferSrcAddFrameFlags(g.bufferSrc, nil, 0); err != nil {
		return nil, fmt.Errorf("ffgo: failed to flush filter: %w", err)
	}

	// Collect remaining frames
	var frames []*Frame
	for {
		avutil.FrameUnref(g.outFrame)
		ret := avfilter.BufferSinkGetFrame(g.bufferSink, g.outFrame)
		if ret == avutil.AVERROR_EAGAIN || ret == avutil.AVERROR_EOF {
			break
		}
		if ret < 0 {
			return frames, fmt.Errorf("ffgo: failed to get frame from filter: %d", ret)
		}

		newFrame := avutil.FrameAlloc()
		if newFrame == nil {
			return frames, errors.New("ffgo: failed to allocate output frame")
		}
		avutil.FrameRef(newFrame, g.outFrame)
		// Allocate a Frame slot to hold the pointer (since we return []*Frame)
		framePtr := new(Frame)
		*framePtr = Frame{ptr: newFrame, owned: true}
		frames = append(frames, framePtr)
	}

	return frames, nil
}

// Close releases all resources associated with the filter graph.
func (g *FilterGraph) Close() error {
	if g.closed {
		return nil
	}
	g.closed = true
	runtime.SetFinalizer(g, nil)
	g.cleanup()
	return nil
}

func (g *FilterGraph) cleanup() {
	if g.outFrame != nil {
		avutil.FrameFree(&g.outFrame)
		g.outFrame = nil
	}
	if g.graph != nil {
		avfilter.GraphFree(&g.graph)
		g.graph = nil
	}
}

// IsVideo returns true if this is a video filter graph.
func (g *FilterGraph) IsVideo() bool {
	return g.isVideo
}

// IsAudio returns true if this is an audio filter graph.
func (g *FilterGraph) IsAudio() bool {
	return !g.isVideo
}
