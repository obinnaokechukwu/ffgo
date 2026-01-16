//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type HLSSegmenterConfig struct {
	SegmentTime            time.Duration
	ListSize               int
	SegmentFilenamePattern string
	Flags                  []string
	AVOptions              map[string]string
}

func NewHLSSegmenter(playlistPath string, cfg *HLSSegmenterConfig) (*Muxer, error) {
	if cfg == nil {
		cfg = &HLSSegmenterConfig{}
	}
	return NewMuxer(playlistPath, "hls")
}

// HeaderOptions returns FFmpeg muxer options suitable to pass to Muxer.WriteHeaderWithOptions.
func (cfg *HLSSegmenterConfig) HeaderOptions() (map[string]string, error) {
	if cfg == nil {
		return nil, fmt.Errorf("ffgo: HLSSegmenterConfig is nil")
	}
	if cfg.SegmentFilenamePattern == "" {
		return nil, fmt.Errorf("ffgo: HLSSegmenterConfig.SegmentFilenamePattern is required")
	}

	opts := make(map[string]string, len(cfg.AVOptions)+8)
	for k, v := range cfg.AVOptions {
		opts[k] = v
	}
	if cfg.SegmentTime > 0 {
		opts["hls_time"] = strconv.FormatFloat(cfg.SegmentTime.Seconds(), 'f', -1, 64)
	}
	if cfg.ListSize > 0 {
		opts["hls_list_size"] = strconv.Itoa(cfg.ListSize)
	}
	if cfg.SegmentFilenamePattern != "" {
		opts["hls_segment_filename"] = cfg.SegmentFilenamePattern
	}
	if len(cfg.Flags) > 0 {
		opts["hls_flags"] = strings.Join(cfg.Flags, ",")
	}
	return opts, nil
}

type DASHSegmenterConfig struct {
	SegmentTime time.Duration
	InitName    string
	MediaName   string
	AVOptions   map[string]string
}

func NewDASHSegmenter(mpdPath string, cfg *DASHSegmenterConfig) (*Muxer, error) {
	if cfg == nil {
		cfg = &DASHSegmenterConfig{}
	}
	return NewMuxer(mpdPath, "dash")
}

// HeaderOptions returns FFmpeg muxer options suitable to pass to Muxer.WriteHeaderWithOptions.
func (cfg *DASHSegmenterConfig) HeaderOptions() map[string]string {
	if cfg == nil {
		return nil
	}
	opts := make(map[string]string, len(cfg.AVOptions)+8)
	for k, v := range cfg.AVOptions {
		opts[k] = v
	}
	if cfg.SegmentTime > 0 {
		opts["seg_duration"] = strconv.FormatFloat(cfg.SegmentTime.Seconds(), 'f', -1, 64)
	}
	if cfg.InitName != "" {
		opts["init_seg_name"] = cfg.InitName
	}
	if cfg.MediaName != "" {
		opts["media_seg_name"] = cfg.MediaName
	}
	return opts
}

