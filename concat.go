//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
)

// WithConcatSafeMode sets the concat demuxer "safe" option.
//
// FFmpeg concat demuxer defaults to safe=1, which disallows absolute paths and some protocols.
// For most local concatenation use cases, you likely want safe=false (safe=0).
func WithConcatSafeMode(safe bool) DecoderOption {
	return func(o *DecoderOptions) {
		if o.AVOptions == nil {
			o.AVOptions = make(map[string]string)
		}
		if safe {
			o.AVOptions["safe"] = "1"
		} else {
			o.AVOptions["safe"] = "0"
		}
	}
}

// WithConcatAutoConvert sets the concat demuxer "auto_convert" option.
//
// When enabled, the concat demuxer may auto-insert bitstream conversions for some inputs.
func WithConcatAutoConvert(enabled bool) DecoderOption {
	return func(o *DecoderOptions) {
		if o.AVOptions == nil {
			o.AVOptions = make(map[string]string)
		}
		if enabled {
			o.AVOptions["auto_convert"] = "1"
		} else {
			o.AVOptions["auto_convert"] = "0"
		}
	}
}

// NewConcatDecoder opens a list of input files using FFmpeg's concat demuxer.
//
// This helper produces an in-memory ffconcat script and opens it via ffgo's custom I/O,
// so no temporary files are required.
//
// Example:
//
//	dec, err := ffgo.NewConcatDecoder([]string{"a.mp4", "b.mp4"}, ffgo.WithConcatSafeMode(false))
//	if err != nil { ... }
//	defer dec.Close()
func NewConcatDecoder(files []string, options ...DecoderOption) (*Decoder, error) {
	if len(files) == 0 {
		return nil, errors.New("ffgo: concat file list cannot be empty")
	}
	script, err := buildFFConcatScript(files)
	if err != nil {
		return nil, err
	}
	return NewConcatDecoderFromFFConcat(script, options...)
}

// NewConcatDecoderFromFile opens an ffconcat list file from disk using the concat demuxer.
func NewConcatDecoderFromFile(listPath string, options ...DecoderOption) (*Decoder, error) {
	if strings.TrimSpace(listPath) == "" {
		return nil, errors.New("ffgo: concat list path cannot be empty")
	}

	opts := &DecoderOptions{}
	for _, opt := range options {
		opt(opts)
	}
	ensureConcatDefaults(opts)

	return NewDecoderWithOptions(listPath, opts)
}

// NewConcatDecoderFromFFConcat opens an ffconcat script from memory using the concat demuxer.
//
// The script should follow the ffconcat format, e.g.:
//
//	ffconcat version 1.0
//	file '/path/to/a.mp4'
//	file '/path/to/b.mp4'
func NewConcatDecoderFromFFConcat(script []byte, options ...DecoderOption) (*Decoder, error) {
	if len(script) == 0 {
		return nil, errors.New("ffgo: ffconcat script cannot be empty")
	}

	opts := &DecoderOptions{}
	for _, opt := range options {
		opt(opts)
	}
	ensureConcatDefaults(opts)

	r := bytes.NewReader(script)
	return NewDecoderFromReaderWithOptions(r, opts)
}

func ensureConcatDefaults(opts *DecoderOptions) {
	if opts == nil {
		return
	}
	if opts.Format == "" {
		opts.Format = "concat"
	}
	// Default to safe=0 so we can use absolute paths in generated scripts.
	if opts.AVOptions == nil {
		opts.AVOptions = make(map[string]string)
	}
	if _, ok := opts.AVOptions["safe"]; !ok {
		opts.AVOptions["safe"] = "0"
	}
}

func buildFFConcatScript(files []string) ([]byte, error) {
	var b strings.Builder
	b.Grow(64 + len(files)*64)
	b.WriteString("ffconcat version 1.0\n")
	for _, f := range files {
		if strings.TrimSpace(f) == "" {
			return nil, errors.New("ffgo: concat file path cannot be empty")
		}
		abs, err := filepath.Abs(f)
		if err != nil {
			abs = f
		}
		abs = strings.ReplaceAll(abs, "\\", "\\\\")
		abs = strings.ReplaceAll(abs, "'", "\\'")
		b.WriteString("file '")
		b.WriteString(abs)
		b.WriteString("'\n")
	}
	return []byte(b.String()), nil
}
