//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"sort"
	"strings"

	"github.com/obinnaokechukwu/ffgo/avformat"
	"github.com/obinnaokechukwu/ffgo/avutil"
	"github.com/obinnaokechukwu/ffgo/internal/bindings"
)

// FormatProbeResult contains detailed information about FFmpeg's demuxer probing result.
type FormatProbeResult struct {
	// Path is the probed input URL/path.
	Path string

	// Format is the selected demuxer short name (e.g. "mov", "matroska").
	Format string

	// LongName is the selected demuxer's long name (if available).
	LongName string

	// ProbeScore is FFmpeg's probe confidence score for the selected demuxer.
	ProbeScore int
}

// ProbeFormat probes the input format for a path/URL and returns detailed information.
//
// This opens the input with FFmpeg probing enabled (no explicit format hint) and then closes it.
func ProbeFormat(path string) (*FormatProbeResult, error) {
	if err := bindings.Load(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("ffgo: path cannot be empty")
	}

	var ctx avformat.FormatContext
	if err := avformat.OpenInput(&ctx, path, nil, nil); err != nil {
		return nil, err
	}
	defer avformat.CloseInput(&ctx)

	ifmt := avformat.GetInputFormat(ctx)
	return &FormatProbeResult{
		Path:       path,
		Format:     avformat.InputFormatName(ifmt),
		LongName:   avformat.InputFormatLongName(ifmt),
		ProbeScore: avformat.GetProbeScore(ctx),
	}, nil
}

// ProbeScore returns FFmpeg's probe confidence score for the decoder's selected input format.
func (d *Decoder) ProbeScore() int {
	if d == nil || d.formatCtx == nil {
		return 0
	}
	return avformat.GetProbeScore(d.formatCtx)
}

func openInputWithRetries(path string, opts *DecoderOptions) (avformat.FormatContext, error) {
	var (
		avOpts = buildDecoderAVOptions(opts)
	)

	// Optional explicit format hint (no retries).
	var forcedFmt avformat.InputFormat
	if opts != nil && opts.Format != "" {
		forcedFmt = avformat.FindInputFormat(opts.Format)
		if forcedFmt == nil {
			return nil, errors.New("ffgo: input format not found")
		}
		ctx, err := openInputOnce(path, forcedFmt, avOpts)
		if err != nil {
			return nil, err
		}
		if opts != nil && opts.ProbeScore > 0 {
			if score := avformat.GetProbeScore(ctx); score > 0 && score < opts.ProbeScore {
				avformat.CloseInput(&ctx)
				return nil, errors.New("ffgo: probe score below required threshold")
			}
		}
		return ctx, nil
	}

	// First try auto-detection.
	ctx, err := openInputOnce(path, nil, avOpts)
	if err == nil {
		if opts == nil || opts.ProbeScore <= 0 {
			return ctx, nil
		}
		score := avformat.GetProbeScore(ctx)
		// If FFmpeg provides a score and it's high enough, accept.
		if score <= 0 || score >= opts.ProbeScore {
			return ctx, nil
		}
		// Low score: retry with forced demuxers (if enabled).
		avformat.CloseInput(&ctx)
		err = errors.New("ffgo: probe score below required threshold")
	}

	if opts == nil || !opts.TryMultipleFormats {
		return nil, err
	}

	candidates := candidateDemuxers(opts)
	for _, name := range candidates {
		fmt := avformat.FindInputFormat(name)
		if fmt == nil {
			continue
		}
		ctx2, err2 := openInputOnce(path, fmt, avOpts)
		if err2 != nil {
			err = err2
			continue
		}
		if opts.ProbeScore > 0 {
			score := avformat.GetProbeScore(ctx2)
			if score > 0 && score < opts.ProbeScore {
				avformat.CloseInput(&ctx2)
				err = errors.New("ffgo: probe score below required threshold")
				continue
			}
		}
		return ctx2, nil
	}

	return nil, err
}

func openInputOnce(path string, fmt avformat.InputFormat, avOpts map[string]string) (avformat.FormatContext, error) {
	var dict avutil.Dictionary
	for k, v := range avOpts {
		if v == "" {
			continue
		}
		if err := avutil.DictSet(&dict, k, v, 0); err != nil {
			if dict != nil {
				avutil.DictFree(&dict)
			}
			return nil, err
		}
	}
	defer func() {
		if dict != nil {
			avutil.DictFree(&dict)
		}
	}()

	var ctx avformat.FormatContext
	if err := avformat.OpenInput(&ctx, path, fmt, &dict); err != nil {
		return nil, err
	}
	return ctx, nil
}

func candidateDemuxers(opts *DecoderOptions) []string {
	seen := make(map[string]struct{})
	var out []string

	add := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}

	black := make(map[string]struct{})
	for _, b := range opts.FormatBlacklist {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		black[b] = struct{}{}
	}

	if len(opts.FormatWhitelist) > 0 {
		for _, w := range opts.FormatWhitelist {
			if _, ok := black[w]; ok {
				continue
			}
			add(w)
		}
		return out
	}

	// No explicit whitelist: iterate all demuxers if possible.
	for _, name := range avformat.DemuxerNames() {
		if _, ok := black[name]; ok {
			continue
		}
		add(name)
	}

	sort.Strings(out)
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}
