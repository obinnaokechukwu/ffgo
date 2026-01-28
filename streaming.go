//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

// EncoderOption configures EncoderOptions using the functional options pattern.
type EncoderOption func(*EncoderOptions)

// WithEncoderFormat forces the output muxer format (e.g. "flv", "mpegts", "rtp").
func WithEncoderFormat(format string) EncoderOption {
	return func(o *EncoderOptions) { o.Format = format }
}

// WithVideoEncoder sets the video encoding configuration.
func WithVideoEncoder(cfg *VideoEncoderConfig) EncoderOption {
	return func(o *EncoderOptions) { o.Video = cfg }
}

// WithAudioEncoder sets the audio encoding configuration.
func WithAudioEncoder(cfg *AudioEncoderConfig) EncoderOption {
	return func(o *EncoderOptions) { o.Audio = cfg }
}

// WithCopyVideo enables/disables video stream copy mode.
func WithCopyVideo(enabled bool) EncoderOption {
	return func(o *EncoderOptions) { o.CopyVideo = enabled }
}

// WithCopyAudio enables/disables audio stream copy mode.
func WithCopyAudio(enabled bool) EncoderOption {
	return func(o *EncoderOptions) { o.CopyAudio = enabled }
}

// WithStreamCopySource sets source codec parameters/timebases for stream copy mode.
func WithStreamCopySource(src *StreamCopySource) EncoderOption {
	return func(o *EncoderOptions) { o.SourceStreams = src }
}

// StreamingOptions configures protocol-level streaming behavior (timeouts, buffers, reconnect).
//
// These options are passed via avio_open2. Support varies by protocol.
type StreamingOptions struct {
	Timeout        time.Duration
	ReconnectCount int
	ReconnectDelay time.Duration

	BufferSize int
	MaxDelay   time.Duration

	// IOOptions are additional raw avio_open2 options.
	IOOptions map[string]string

	// MuxerOptions are options passed to avformat_write_header (muxer-specific).
	MuxerOptions map[string]string
}

// WithStreamingOptions applies streaming protocol/muxer options.
func WithStreamingOptions(s *StreamingOptions) EncoderOption {
	return func(o *EncoderOptions) {
		if s == nil {
			return
		}
		if o.IOOptions == nil {
			o.IOOptions = make(map[string]string)
		}
		if o.MuxerOptions == nil {
			o.MuxerOptions = make(map[string]string)
		}

		for k, v := range s.IOOptions {
			o.IOOptions[k] = v
		}
		for k, v := range s.MuxerOptions {
			o.MuxerOptions[k] = v
		}

		if s.Timeout > 0 {
			// FFmpeg uses microseconds for timeout-like options.
			o.IOOptions["timeout"] = int64ToString(s.Timeout.Microseconds())
			o.IOOptions["rw_timeout"] = int64ToString(s.Timeout.Microseconds())
		}
		if s.ReconnectCount > 0 {
			o.IOOptions["reconnect"] = "1"
			o.IOOptions["reconnect_streamed"] = "1"
			if s.ReconnectDelay > 0 {
				o.IOOptions["reconnect_delay_max"] = int64ToString(int64(s.ReconnectDelay.Seconds()))
			}
		}
		if s.BufferSize > 0 {
			o.IOOptions["buffer_size"] = int64ToString(int64(s.BufferSize))
		}
		if s.MaxDelay > 0 {
			o.IOOptions["max_delay"] = int64ToString(s.MaxDelay.Microseconds())
		}
	}
}

// NewStreamingEncoder creates an encoder configured for network streaming outputs (RTMP/UDP/RTP/etc).
//
// ffgo selects a sensible default muxer based on the URL scheme:
//   - rtmp/rtmps -> flv
//   - udp/srt    -> mpegts
//   - rtp        -> rtp
//   - rtsp       -> rtsp
//
// You can override the muxer via WithEncoderFormat.
func NewStreamingEncoder(outURL string, options ...EncoderOption) (*Encoder, error) {
	if strings.TrimSpace(outURL) == "" {
		return nil, errors.New("ffgo: output url cannot be empty")
	}
	u, err := url.Parse(outURL)
	if err != nil || u.Scheme == "" {
		return nil, errors.New("ffgo: invalid streaming url")
	}

	encOpts := &EncoderOptions{}
	for _, opt := range options {
		opt(encOpts)
	}

	if encOpts.Format == "" {
		switch strings.ToLower(u.Scheme) {
		case "rtmp", "rtmps":
			encOpts.Format = "flv"
		case "udp", "srt":
			encOpts.Format = "mpegts"
		case "rtp":
			encOpts.Format = "rtp"
		case "rtsp":
			encOpts.Format = "rtsp"
		default:
			return nil, errors.New("ffgo: unsupported streaming scheme")
		}
	}

	// Ensure the constructor does not eagerly connect for streaming URLs by using lazy IO open.
	if encOpts.IOOptions == nil {
		encOpts.IOOptions = map[string]string{}
	}

	return NewEncoderWithOptions(outURL, encOpts)
}

func int64ToString(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var b [32]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + (v % 10))
		v /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
