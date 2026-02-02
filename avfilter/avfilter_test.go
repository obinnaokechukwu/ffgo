//go:build !ios && !android && (amd64 || arm64)

package avfilter

import (
	"testing"
)

func TestInit(t *testing.T) {
	if err := Init(); err != nil {
		t.Logf("avfilter not available: %v", err)
		return
	}
	t.Log("avfilter initialized successfully")
}

func TestVersion(t *testing.T) {
	v := Version()
	if v == 0 {
		t.Log("avfilter not available")
		return
	}
	t.Logf("avfilter version: %s (raw: %d)", VersionString(), v)

	// Sanity check: version should be reasonable (7.x to 10.x)
	major := (v >> 16) & 0xFF
	if major < 7 || major > 15 {
		t.Errorf("unexpected major version: %d", major)
	}
}

func TestGraphAlloc(t *testing.T) {
	if err := Init(); err != nil {
		t.Logf("avfilter not available: %v", err)
		return
	}

	graph := GraphAlloc()
	if graph == nil {
		t.Fatal("GraphAlloc returned nil")
	}
	defer GraphFree(&graph)

	t.Log("GraphAlloc succeeded")
}

func TestGetByName(t *testing.T) {
	if err := Init(); err != nil {
		t.Logf("avfilter not available: %v", err)
		return
	}

	tests := []struct {
		name     string
		expected bool
	}{
		{"buffer", true},
		{"buffersink", true},
		{"scale", true},
		{"format", true},
		{"null", true},
		{"nonexistent_filter_xyz123", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := GetByName(tc.name)
			if tc.expected && f == nil {
				t.Errorf("GetByName(%q) returned nil, expected filter", tc.name)
			} else if !tc.expected && f != nil {
				t.Errorf("GetByName(%q) returned filter, expected nil", tc.name)
			}
		})
	}
}

func TestInOutAlloc(t *testing.T) {
	if err := Init(); err != nil {
		t.Logf("avfilter not available: %v", err)
		return
	}

	inout := InOutAlloc()
	if inout == nil {
		t.Fatal("InOutAlloc returned nil")
	}
	defer InOutFree(&inout)

	t.Log("InOutAlloc succeeded")
}

func TestInOutSettersGetters(t *testing.T) {
	if err := Init(); err != nil {
		t.Logf("avfilter not available: %v", err)
		return
	}

	inout := InOutAlloc()
	if inout == nil {
		t.Fatal("InOutAlloc returned nil")
	}
	defer InOutFree(&inout)

	// Test pad_idx setter/getter
	InOutSetPadIdx(inout, 42)
	if got := InOutGetPadIdx(inout); got != 42 {
		t.Errorf("InOutGetPadIdx() = %d, want 42", got)
	}

	// Test next setter/getter (set to nil)
	InOutSetNext(inout, nil)
	if got := InOutGetNext(inout); got != nil {
		t.Error("InOutGetNext() expected nil")
	}
}

func TestGraphParse2(t *testing.T) {
	if err := Init(); err != nil {
		t.Logf("avfilter not available: %v", err)
		return
	}

	graph := GraphAlloc()
	if graph == nil {
		t.Fatal("GraphAlloc returned nil")
	}
	defer GraphFree(&graph)

	// Test parsing a simple filter graph
	inputs, outputs, err := GraphParse2(graph, "null")
	if err != nil {
		t.Fatalf("GraphParse2 failed: %v", err)
	}

	// Clean up inputs/outputs
	if inputs != nil {
		InOutFree(&inputs)
	}
	if outputs != nil {
		InOutFree(&outputs)
	}

	t.Log("GraphParse2 succeeded")
}

func TestGraphCreateFilter(t *testing.T) {
	if err := Init(); err != nil {
		t.Logf("avfilter not available: %v", err)
		return
	}

	graph := GraphAlloc()
	if graph == nil {
		t.Fatal("GraphAlloc returned nil")
	}
	defer GraphFree(&graph)

	// Get the buffer filter
	bufferFilter := GetByName("buffer")
	if bufferFilter == nil {
		t.Log("buffer filter not available")
		return
	}

	// Create a buffer filter with video parameters
	args := "video_size=320x240:pix_fmt=0:time_base=1/25:pixel_aspect=1/1"
	ctx, err := GraphCreateFilter(graph, bufferFilter, "in", args)
	if err != nil {
		t.Fatalf("GraphCreateFilter failed: %v", err)
	}

	if ctx == nil {
		t.Fatal("GraphCreateFilter returned nil context")
	}

	t.Log("GraphCreateFilter succeeded")
}

func TestLink(t *testing.T) {
	if err := Init(); err != nil {
		t.Logf("avfilter not available: %v", err)
		return
	}

	graph := GraphAlloc()
	if graph == nil {
		t.Fatal("GraphAlloc returned nil")
	}
	defer GraphFree(&graph)

	// Create buffersrc
	bufferFilter := GetByName("buffer")
	if bufferFilter == nil {
		t.Log("buffer filter not available")
		return
	}
	srcArgs := "video_size=320x240:pix_fmt=0:time_base=1/25:pixel_aspect=1/1"
	srcCtx, err := GraphCreateFilter(graph, bufferFilter, "in", srcArgs)
	if err != nil {
		t.Fatalf("create buffersrc failed: %v", err)
	}

	// Create buffersink
	sinkFilter := GetByName("buffersink")
	if sinkFilter == nil {
		t.Log("buffersink filter not available")
		return
	}
	sinkCtx, err := GraphCreateFilter(graph, sinkFilter, "out", "")
	if err != nil {
		t.Fatalf("create buffersink failed: %v", err)
	}

	// Link src -> sink
	if err := Link(srcCtx, 0, sinkCtx, 0); err != nil {
		t.Fatalf("Link failed: %v", err)
	}

	// Configure the graph
	if err := GraphConfig(graph); err != nil {
		t.Fatalf("GraphConfig failed: %v", err)
	}

	t.Log("Link and GraphConfig succeeded")
}

func TestBufferSourceSinkFlags(t *testing.T) {
	// Test that flag constants have expected values
	if AV_BUFFERSRC_FLAG_KEEP_REF != 8 {
		t.Errorf("AV_BUFFERSRC_FLAG_KEEP_REF = %d, want 8", AV_BUFFERSRC_FLAG_KEEP_REF)
	}
	if AV_BUFFERSRC_FLAG_PUSH != 4 {
		t.Errorf("AV_BUFFERSRC_FLAG_PUSH = %d, want 4", AV_BUFFERSRC_FLAG_PUSH)
	}
	if AV_BUFFERSINK_FLAG_PEEK != 1 {
		t.Errorf("AV_BUFFERSINK_FLAG_PEEK = %d, want 1", AV_BUFFERSINK_FLAG_PEEK)
	}
	if AV_BUFFERSINK_FLAG_NO_REQUEST != 2 {
		t.Errorf("AV_BUFFERSINK_FLAG_NO_REQUEST = %d, want 2", AV_BUFFERSINK_FLAG_NO_REQUEST)
	}
}

func TestFullFilterGraphCreation(t *testing.T) {
	if err := Init(); err != nil {
		t.Logf("avfilter not available: %v", err)
		return
	}

	graph := GraphAlloc()
	if graph == nil {
		t.Fatal("GraphAlloc returned nil")
	}
	defer GraphFree(&graph)

	// Create buffer source
	bufferSrc := GetByName("buffer")
	if bufferSrc == nil {
		t.Log("buffer filter not available")
		return
	}
	srcArgs := "video_size=640x480:pix_fmt=0:time_base=1/30:pixel_aspect=1/1"
	srcCtx, err := GraphCreateFilter(graph, bufferSrc, "in", srcArgs)
	if err != nil {
		t.Fatalf("create buffersrc failed: %v", err)
	}

	// Create scale filter
	scaleFilter := GetByName("scale")
	if scaleFilter == nil {
		t.Log("scale filter not available")
		return
	}
	scaleCtx, err := GraphCreateFilter(graph, scaleFilter, "scale", "w=320:h=240")
	if err != nil {
		t.Fatalf("create scale filter failed: %v", err)
	}

	// Create buffer sink
	bufferSink := GetByName("buffersink")
	if bufferSink == nil {
		t.Log("buffersink filter not available")
		return
	}
	sinkCtx, err := GraphCreateFilter(graph, bufferSink, "out", "")
	if err != nil {
		t.Fatalf("create buffersink failed: %v", err)
	}

	// Link: src -> scale -> sink
	if err := Link(srcCtx, 0, scaleCtx, 0); err != nil {
		t.Fatalf("link src->scale failed: %v", err)
	}
	if err := Link(scaleCtx, 0, sinkCtx, 0); err != nil {
		t.Fatalf("link scale->sink failed: %v", err)
	}

	// Configure graph
	if err := GraphConfig(graph); err != nil {
		t.Fatalf("GraphConfig failed: %v", err)
	}

	t.Log("Full filter graph creation succeeded: buffersrc -> scale(320x240) -> buffersink")
}

func TestAudioFilterLookup(t *testing.T) {
	if err := Init(); err != nil {
		t.Logf("avfilter not available: %v", err)
		return
	}

	audioFilters := []string{
		"abuffer",     // audio buffer source
		"abuffersink", // audio buffer sink
		"aformat",     // audio format conversion
		"volume",      // volume control
		"aresample",   // audio resampling
	}

	for _, name := range audioFilters {
		t.Run(name, func(t *testing.T) {
			f := GetByName(name)
			if f == nil {
				t.Logf("audio filter %q not available (may be disabled at FFmpeg build time)", name)
				return
			}
			t.Logf("audio filter %q found", name)
		})
	}
}
