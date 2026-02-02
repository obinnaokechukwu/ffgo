//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"testing"
	"time"
)

func TestNewStreamingEncoder_URLMappingAndLazyIO(t *testing.T) {
	if !requireFFmpeg(t) {
		return
	}

	enc, err := NewStreamingEncoder(
		"rtmp://example.com/live/stream",
		WithVideoEncoder(&VideoEncoderConfig{
			Codec:       CodecIDH264,
			Width:       160,
			Height:      120,
			FrameRate:   NewRational(30, 1),
			PixelFormat: PixelFormatYUV420P,
			RateControl: RateControlCRF,
			CRF:         28,
		}),
		WithStreamingOptions(&StreamingOptions{
			Timeout:  5 * time.Second,
			MaxDelay: 250 * time.Millisecond,
		}),
	)
	if err != nil {
		t.Fatalf("NewStreamingEncoder failed: %v", err)
	}
	defer enc.Close()

	// Must not connect/open IO eagerly for URL outputs.
	if enc.ioCtx != nil {
		t.Fatalf("expected ioCtx to be nil (lazy open), got %v", enc.ioCtx)
	}
	if enc.formatCtx == nil {
		t.Fatalf("expected formatCtx to be initialized")
	}
}

func TestNewStreamingEncoder_UnsupportedScheme(t *testing.T) {
	_, err := NewStreamingEncoder("ftp://example.com/out", WithEncoderFormat(""))
	if err == nil {
		t.Fatalf("expected error for unsupported scheme")
	}
}

func TestWithStreamingOptions_SetsIOOptions(t *testing.T) {
	opts := &EncoderOptions{}
	WithStreamingOptions(&StreamingOptions{
		Timeout:        2 * time.Second,
		ReconnectCount: 1,
		ReconnectDelay: 3 * time.Second,
		BufferSize:     12345,
		MaxDelay:       500 * time.Millisecond,
	})(opts)

	if opts.IOOptions["timeout"] == "" || opts.IOOptions["rw_timeout"] == "" {
		t.Fatalf("expected timeout options to be set")
	}
	if opts.IOOptions["reconnect"] != "1" {
		t.Fatalf("expected reconnect=1, got %q", opts.IOOptions["reconnect"])
	}
	if opts.IOOptions["buffer_size"] != "12345" {
		t.Fatalf("expected buffer_size=12345, got %q", opts.IOOptions["buffer_size"])
	}
}
