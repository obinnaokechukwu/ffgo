//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"path/filepath"
	"testing"
)

func TestProbeFormat(t *testing.T) {
	if testing.Short() {
		t.Log("Skipping probe integration test in short mode")
		return
	}
	if !requireFFmpeg(t) {
		return
	}

	in := filepath.Join("testdata", "test.mp4")
	r, err := ProbeFormat(in)
	if err != nil {
		t.Fatalf("ProbeFormat failed: %v", err)
	}
	if r.Format == "" {
		t.Fatalf("expected non-empty format name")
	}
	// ProbeScore can be 0 on some builds/inputs, but mp4 should typically be >0.
}

func TestDecoderOptions_ProbeScoreThreshold(t *testing.T) {
	if testing.Short() {
		t.Log("Skipping probe score threshold test in short mode")
		return
	}
	if !requireFFmpeg(t) {
		return
	}

	in := filepath.Join("testdata", "test.mp4")
	r, err := ProbeFormat(in)
	if err != nil {
		t.Fatalf("ProbeFormat failed: %v", err)
	}
	if r.ProbeScore <= 0 {
		t.Log("FFmpeg did not provide a probe score for this input/build")
		return
	}

	// Require an impossible score: should fail.
	_, err = NewDecoderWithOptions(in, &DecoderOptions{
		ProbeScore: r.ProbeScore + 1,
	})
	if err == nil {
		t.Fatalf("expected error when requiring ProbeScore > actual (actual=%d)", r.ProbeScore)
	}

	// Require <= actual: should succeed.
	d, err := NewDecoderWithOptions(in, &DecoderOptions{
		ProbeScore:         r.ProbeScore,
		TryMultipleFormats: true,
	})
	if err != nil {
		t.Fatalf("NewDecoderWithOptions failed: %v", err)
	}
	_ = d.Close()
}

func TestCandidateDemuxers_BlacklistFiltersWhitelist(t *testing.T) {
	opts := &DecoderOptions{
		FormatWhitelist:    []string{"mov", "matroska"},
		FormatBlacklist:    []string{"mov"},
		TryMultipleFormats: true,
	}
	c := candidateDemuxers(opts)
	for _, v := range c {
		if v == "mov" {
			t.Fatalf("expected blacklist to exclude mov from candidates")
		}
	}
	if len(c) != 1 || c[0] != "matroska" {
		t.Fatalf("unexpected candidates: %#v", c)
	}
}
