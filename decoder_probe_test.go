//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"testing"
	"time"
)

func TestBuildDecoderAVOptions_TypedOverridesRaw(t *testing.T) {
	opts := &DecoderOptions{
		AVOptions: map[string]string{
			"probesize":         "1",
			"analyzeduration":   "2",
			"max_probe_packets": "3",
		},
		ProbeSizeBytes:  10,
		AnalyzeDuration: 1500 * time.Microsecond,
		MaxProbePackets: 20,
		FormatWhitelist: []string{"mov", "mp4"},
		CodecWhitelist:  []string{"h264", "aac"},
	}

	m := buildDecoderAVOptions(opts)
	if got := m["probesize"]; got != "10" {
		t.Fatalf("probesize: expected 10, got %q", got)
	}
	if got := m["analyzeduration"]; got != "1500" {
		t.Fatalf("analyzeduration: expected 1500, got %q", got)
	}
	if got := m["max_probe_packets"]; got != "20" {
		t.Fatalf("max_probe_packets: expected 20, got %q", got)
	}
	if got := m["format_whitelist"]; got != "mov,mp4" {
		t.Fatalf("format_whitelist: expected mov,mp4, got %q", got)
	}
	if got := m["codec_whitelist"]; got != "h264,aac" {
		t.Fatalf("codec_whitelist: expected h264,aac, got %q", got)
	}
}

