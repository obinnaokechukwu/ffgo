//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"fmt"
	"time"
)

// ProtocolOptions configures network protocol behavior for streaming.
type ProtocolOptions struct {
	// Connection options
	Timeout        time.Duration // Connection timeout (default: 5s)
	ReconnectCount int           // Auto-reconnect attempts (0 = disabled)
	ReconnectDelay time.Duration // Delay between reconnects

	// Buffer options
	BufferSize int           // Protocol buffer size in bytes
	MaxDelay   time.Duration // Maximum delay for buffering

	// RTMP specific
	RTMPApp      string // RTMP application name
	RTMPPlayPath string // RTMP stream name
	RTMPLive     bool   // Live stream mode (disables seeking)

	// HTTP specific
	HTTPHeaders map[string]string // Custom HTTP headers
	HTTPCookies string            // HTTP cookies

	// TLS/SSL options
	TLSVerify bool   // Verify TLS certificates (default: true)
	TLSCert   string // Client certificate path
	TLSKey    string // Client key path

	// Additional FFmpeg options
	AVOptions map[string]string // Additional raw FFmpeg options
}

// NewNetworkDecoder opens a network stream with protocol-specific options.
// Supports RTMP, RTSP, HLS (HTTP), SRT, and other FFmpeg-supported protocols.
func NewNetworkDecoder(url string, opts *ProtocolOptions) (*Decoder, error) {
	if opts == nil {
		opts = &ProtocolOptions{}
	}

	// Build AVOptions from ProtocolOptions
	avOpts := make(map[string]string)

	// Copy any additional AVOptions first
	for k, v := range opts.AVOptions {
		avOpts[k] = v
	}

	// Connection options
	if opts.Timeout > 0 {
		// FFmpeg uses microseconds for timeout
		avOpts["timeout"] = fmt.Sprintf("%d", opts.Timeout.Microseconds())
		// Also set connect timeout for TCP
		avOpts["stimeout"] = fmt.Sprintf("%d", opts.Timeout.Microseconds())
	}

	if opts.ReconnectCount > 0 {
		avOpts["reconnect"] = "1"
		avOpts["reconnect_streamed"] = "1"
		avOpts["reconnect_delay_max"] = fmt.Sprintf("%d", int(opts.ReconnectDelay.Seconds()))
		// Note: reconnect_at_eof and reconnect_on_network_error may also be useful
	}

	// Buffer options
	if opts.BufferSize > 0 {
		avOpts["buffer_size"] = fmt.Sprintf("%d", opts.BufferSize)
	}

	if opts.MaxDelay > 0 {
		avOpts["max_delay"] = fmt.Sprintf("%d", opts.MaxDelay.Microseconds())
	}

	// RTMP options
	if opts.RTMPApp != "" {
		avOpts["rtmp_app"] = opts.RTMPApp
	}
	if opts.RTMPPlayPath != "" {
		avOpts["rtmp_playpath"] = opts.RTMPPlayPath
	}
	if opts.RTMPLive {
		avOpts["rtmp_live"] = "live"
	}

	// HTTP options
	if len(opts.HTTPHeaders) > 0 {
		// Format headers as "Key: Value\r\nKey2: Value2\r\n"
		var headers string
		for k, v := range opts.HTTPHeaders {
			headers += fmt.Sprintf("%s: %s\r\n", k, v)
		}
		avOpts["headers"] = headers
	}
	if opts.HTTPCookies != "" {
		avOpts["cookies"] = opts.HTTPCookies
	}

	// TLS options
	if !opts.TLSVerify {
		// Only set if explicitly disabled - FFmpeg verifies by default
		avOpts["tls_verify"] = "0"
	}
	if opts.TLSCert != "" {
		avOpts["cert"] = opts.TLSCert
	}
	if opts.TLSKey != "" {
		avOpts["key"] = opts.TLSKey
	}

	// Create decoder with AVOptions
	return NewDecoderWithOptions(url, &DecoderOptions{
		AVOptions: avOpts,
	})
}

// Common timeout presets for network streams
const (
	NetworkTimeoutShort  = 5 * time.Second
	NetworkTimeoutMedium = 15 * time.Second
	NetworkTimeoutLong   = 30 * time.Second
)
