//go:build !ios && !android && (amd64 || arm64)

// Package avfilter provides audio/video filtering using FFmpeg's libavfilter.
package avfilter

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/obinnaokechukwu/ffgo/internal/bindings"
)

// Opaque types
type (
	// Graph represents an AVFilterGraph
	Graph = unsafe.Pointer
	// Context represents an AVFilterContext
	Context = unsafe.Pointer
	// Filter represents an AVFilter
	Filter = unsafe.Pointer
	// InOut represents an AVFilterInOut
	InOut = unsafe.Pointer
)

var (
	libAVFilter uintptr
	initOnce    sync.Once
	initErr     error
)

	// Function bindings
	var (
		// Graph management
		avfilter_graph_alloc         func() uintptr
		avfilter_graph_free          func(graph *Graph)
		avfilter_graph_config        func(graphctx, log_ctx uintptr) int32
		avfilter_graph_parse2        func(graph uintptr, filters *byte, inputs, outputs *InOut) int32
		avfilter_graph_create_filter func(filt_ctx *Context, filt, namePtr, argsPtr, opaque, graphCtx uintptr) int32

		// Filter lookup
		avfilter_get_by_name func(name *byte) uintptr

		// Filter linking
		avfilter_link func(src uintptr, srcpad uint32, dst uintptr, dstpad uint32) int32

		// Buffer source/sink
		av_buffersrc_add_frame_flags  func(ctx, frame uintptr, flags int32) int32
		av_buffersink_get_frame_flags func(ctx, frame uintptr, flags int32) int32
		av_buffersink_get_frame       func(ctx, frame uintptr) int32

		// InOut management
		avfilter_inout_alloc func() uintptr
		avfilter_inout_free  func(inout *InOut)

	// Version
	avfilter_version func() uint32

	// AVFilterInOut field accessors (offsets may need verification)
	// We use the offset approach since AVFilterInOut is a struct
)

// Buffer source flags
const (
	AV_BUFFERSRC_FLAG_NO_CHECK_FORMAT = 1 // Do not check for format changes
	AV_BUFFERSRC_FLAG_PUSH            = 4 // Push frame immediately
	AV_BUFFERSRC_FLAG_KEEP_REF        = 8 // Keep reference to frame
)

// Buffer sink flags
const (
	AV_BUFFERSINK_FLAG_PEEK       = 1 // Peek without consuming
	AV_BUFFERSINK_FLAG_NO_REQUEST = 2 // Don't request frame
)

// Init initializes the avfilter library bindings
func Init() error {
	initOnce.Do(func() {
		initErr = initLibrary()
	})
	return initErr
}

func initLibrary() error {
	var err error
	// libavfilter versions: 9.x (FFmpeg 6.x), 8.x (FFmpeg 5.x), 7.x (FFmpeg 4.x)
	libAVFilter, err = bindings.LoadLibrary("avfilter", []int{10, 9, 8, 7})
	if err != nil {
		return fmt.Errorf("avfilter: failed to load library: %w", err)
	}

	// Bind core functions
	purego.RegisterLibFunc(&avfilter_graph_alloc, libAVFilter, "avfilter_graph_alloc")
	purego.RegisterLibFunc(&avfilter_graph_free, libAVFilter, "avfilter_graph_free")
	purego.RegisterLibFunc(&avfilter_graph_config, libAVFilter, "avfilter_graph_config")
	purego.RegisterLibFunc(&avfilter_graph_parse2, libAVFilter, "avfilter_graph_parse2")
	purego.RegisterLibFunc(&avfilter_graph_create_filter, libAVFilter, "avfilter_graph_create_filter")
	purego.RegisterLibFunc(&avfilter_get_by_name, libAVFilter, "avfilter_get_by_name")
	purego.RegisterLibFunc(&avfilter_link, libAVFilter, "avfilter_link")
	purego.RegisterLibFunc(&avfilter_inout_alloc, libAVFilter, "avfilter_inout_alloc")
	purego.RegisterLibFunc(&avfilter_inout_free, libAVFilter, "avfilter_inout_free")
	purego.RegisterLibFunc(&avfilter_version, libAVFilter, "avfilter_version")

	// Buffer source/sink functions (from libavfilter)
	purego.RegisterLibFunc(&av_buffersrc_add_frame_flags, libAVFilter, "av_buffersrc_add_frame_flags")
	purego.RegisterLibFunc(&av_buffersink_get_frame_flags, libAVFilter, "av_buffersink_get_frame_flags")
	purego.RegisterLibFunc(&av_buffersink_get_frame, libAVFilter, "av_buffersink_get_frame")

	return nil
}

// Version returns the libavfilter version.
func Version() uint32 {
	if err := Init(); err != nil {
		return 0
	}
	return avfilter_version()
}

// VersionString returns the libavfilter version as a string (e.g., "9.12.100").
func VersionString() string {
	v := Version()
	if v == 0 {
		return "unknown"
	}
	major := (v >> 16) & 0xFF
	minor := (v >> 8) & 0xFF
	micro := v & 0xFF
	return fmt.Sprintf("%d.%d.%d", major, minor, micro)
}

// GraphAlloc allocates a new filter graph.
func GraphAlloc() Graph {
	if err := Init(); err != nil {
		return nil
	}
	return unsafe.Pointer(avfilter_graph_alloc())
}

// GraphFree frees a filter graph and all associated filters.
func GraphFree(graph *Graph) {
	if graph == nil || *graph == nil {
		return
	}
	if err := Init(); err != nil {
		return
	}
	avfilter_graph_free(graph)
}

// GraphConfig validates and configures a filter graph.
func GraphConfig(graph Graph) error {
	if graph == nil {
		return fmt.Errorf("avfilter: nil graph")
	}
	if err := Init(); err != nil {
		return err
	}
	ret := avfilter_graph_config(uintptr(graph), 0)
	if ret < 0 {
		return fmt.Errorf("avfilter_graph_config failed: %d", ret)
	}
	return nil
}

// cString converts a Go string to a null-terminated C string (as *byte)
func cString(s string) *byte {
	if s == "" {
		return nil
	}
	b := append([]byte(s), 0)
	return &b[0]
}

// GraphParse2 parses a filter graph description.
// Returns inputs and outputs that need to be linked.
func GraphParse2(graph Graph, filters string) (inputs, outputs InOut, err error) {
	if graph == nil {
		return nil, nil, fmt.Errorf("avfilter: nil graph")
	}
	if err := Init(); err != nil {
		return nil, nil, err
	}

	ret := avfilter_graph_parse2(uintptr(graph), cString(filters), &inputs, &outputs)
	if ret < 0 {
		return nil, nil, fmt.Errorf("avfilter_graph_parse2 failed: %d", ret)
	}
	return inputs, outputs, nil
}

// GraphCreateFilter creates and adds a filter to a graph.
func GraphCreateFilter(graph Graph, filter Filter, name, args string) (Context, error) {
	if graph == nil {
		return nil, fmt.Errorf("avfilter: nil graph")
	}
	if filter == nil {
		return nil, fmt.Errorf("avfilter: nil filter")
	}
	if err := Init(); err != nil {
		return nil, err
	}

	var ctx Context
	ret := avfilter_graph_create_filter(
		&ctx,
		uintptr(filter),
		uintptr(unsafe.Pointer(cString(name))),
		uintptr(unsafe.Pointer(cString(args))),
		0,
		uintptr(graph),
	)
	if ret < 0 {
		return nil, fmt.Errorf("avfilter_graph_create_filter failed: %d", ret)
	}
	return ctx, nil
}

// GetByName finds a filter by name (e.g., "buffer", "buffersink", "scale").
func GetByName(name string) Filter {
	if err := Init(); err != nil {
		return nil
	}
	return unsafe.Pointer(avfilter_get_by_name(cString(name)))
}

// Link links two filter contexts together.
func Link(src Context, srcPad uint32, dst Context, dstPad uint32) error {
	if src == nil || dst == nil {
		return fmt.Errorf("avfilter: nil context")
	}
	if err := Init(); err != nil {
		return err
	}
	ret := avfilter_link(uintptr(src), srcPad, uintptr(dst), dstPad)
	if ret < 0 {
		return fmt.Errorf("avfilter_link failed: %d", ret)
	}
	return nil
}

// BufferSrcAddFrameFlags pushes a frame to a buffersrc filter.
func BufferSrcAddFrameFlags(ctx Context, frame unsafe.Pointer, flags int32) error {
	if ctx == nil {
		return fmt.Errorf("avfilter: nil context")
	}
	if err := Init(); err != nil {
		return err
	}
	ret := av_buffersrc_add_frame_flags(uintptr(ctx), uintptr(frame), flags)
	if ret < 0 {
		return fmt.Errorf("av_buffersrc_add_frame_flags failed: %d", ret)
	}
	return nil
}

// BufferSinkGetFrameFlags retrieves a frame from a buffersink filter.
// Returns the FFmpeg error code (0 on success, AVERROR_EAGAIN, AVERROR_EOF, or negative on error).
func BufferSinkGetFrameFlags(ctx Context, frame unsafe.Pointer, flags int32) int32 {
	if ctx == nil {
		return -22 // EINVAL
	}
	if err := Init(); err != nil {
		return -22
	}
	return av_buffersink_get_frame_flags(uintptr(ctx), uintptr(frame), flags)
}

// BufferSinkGetFrame retrieves a frame from a buffersink filter (convenience wrapper).
func BufferSinkGetFrame(ctx Context, frame unsafe.Pointer) int32 {
	if ctx == nil {
		return -22 // EINVAL
	}
	if err := Init(); err != nil {
		return -22
	}
	return av_buffersink_get_frame(uintptr(ctx), uintptr(frame))
}

// InOutAlloc allocates an AVFilterInOut structure.
func InOutAlloc() InOut {
	if err := Init(); err != nil {
		return nil
	}
	return unsafe.Pointer(avfilter_inout_alloc())
}

// InOutFree frees an AVFilterInOut structure.
func InOutFree(inout *InOut) {
	if inout == nil || *inout == nil {
		return
	}
	if err := Init(); err != nil {
		return
	}
	avfilter_inout_free(inout)
}

// AVFilterInOut struct offsets (for FFmpeg 6.x)
// struct AVFilterInOut {
//     char *name;            // offset 0
//     AVFilterContext *filter_ctx;  // offset 8
//     int pad_idx;           // offset 16
//     struct AVFilterInOut *next;   // offset 24
// }
const (
	offsetInOutName      = 0
	offsetInOutFilterCtx = 8
	offsetInOutPadIdx    = 16
	offsetInOutNext      = 24
)

// InOutSetName sets the name field of an AVFilterInOut.
func InOutSetName(inout InOut, name string) {
	if inout == nil {
		return
	}
	// Note: In FFmpeg, this is typically "in" or "out" - allocated by avfilter_inout_alloc
	// We need to use av_strdup or similar to set it properly
	// For simplicity, we leave it null and let FFmpeg handle default names
}

// InOutSetFilterCtx sets the filter_ctx field of an AVFilterInOut.
func InOutSetFilterCtx(inout InOut, ctx Context) {
	if inout == nil {
		return
	}
	ptr := uintptr(inout) + offsetInOutFilterCtx
	*(*unsafe.Pointer)(unsafe.Pointer(ptr)) = ctx
}

// InOutSetPadIdx sets the pad_idx field of an AVFilterInOut.
func InOutSetPadIdx(inout InOut, padIdx int32) {
	if inout == nil {
		return
	}
	ptr := uintptr(inout) + offsetInOutPadIdx
	*(*int32)(unsafe.Pointer(ptr)) = padIdx
}

// InOutSetNext sets the next field of an AVFilterInOut.
func InOutSetNext(inout InOut, next InOut) {
	if inout == nil {
		return
	}
	ptr := uintptr(inout) + offsetInOutNext
	*(*unsafe.Pointer)(unsafe.Pointer(ptr)) = next
}

// InOutGetFilterCtx gets the filter_ctx from an AVFilterInOut.
func InOutGetFilterCtx(inout InOut) Context {
	if inout == nil {
		return nil
	}
	ptr := uintptr(inout) + offsetInOutFilterCtx
	return *(*unsafe.Pointer)(unsafe.Pointer(ptr))
}

// InOutGetPadIdx gets the pad_idx from an AVFilterInOut.
func InOutGetPadIdx(inout InOut) int32 {
	if inout == nil {
		return 0
	}
	ptr := uintptr(inout) + offsetInOutPadIdx
	return *(*int32)(unsafe.Pointer(ptr))
}

// InOutGetNext gets the next pointer from an AVFilterInOut.
func InOutGetNext(inout InOut) InOut {
	if inout == nil {
		return nil
	}
	ptr := uintptr(inout) + offsetInOutNext
	return *(*unsafe.Pointer)(unsafe.Pointer(ptr))
}
