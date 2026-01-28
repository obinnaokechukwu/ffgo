//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"fmt"
	"math"

	"github.com/obinnaokechukwu/ffgo/avutil"
)

// FrameTiming helps generate and track frame PTS/DTS values in a given time base.
type FrameTiming struct {
	TimeBase Rational
	FPS      float64

	// Step is the per-frame increment in TimeBase units.
	Step int64

	NextPTS int64
	NextDTS int64
}

// NewFrameTiming constructs a FrameTiming for the given time base and nominal frame rate.
func NewFrameTiming(timebase Rational, fps float64) (*FrameTiming, error) {
	if timebase.Den <= 0 || timebase.Num <= 0 {
		return nil, errors.New("ffgo: invalid time base")
	}
	if fps <= 0 {
		return nil, errors.New("ffgo: fps must be positive")
	}
	step := int64(math.Round(float64(timebase.Den) / (float64(timebase.Num) * fps)))
	if step <= 0 {
		step = 1
	}
	return &FrameTiming{
		TimeBase: timebase,
		FPS:      fps,
		Step:     step,
	}, nil
}

// Next returns the next (pts, dts) pair and advances the internal counters.
func (t *FrameTiming) Next() (pts, dts int64) {
	if t == nil {
		return avutil.NoPTSValue, avutil.NoPTSValue
	}
	pts = t.NextPTS
	dts = t.NextDTS
	t.NextPTS += t.Step
	t.NextDTS += t.Step
	return pts, dts
}

// GenerateTimestamps generates count PTS values in the given time base for a nominal fps.
func GenerateTimestamps(count int, timebase Rational, fps float64) []int64 {
	if count <= 0 {
		return nil
	}
	t, err := NewFrameTiming(timebase, fps)
	if err != nil {
		return nil
	}
	out := make([]int64, 0, count)
	for i := 0; i < count; i++ {
		pts, _ := t.Next()
		out = append(out, pts)
	}
	return out
}

// ValidateTimestamps checks that frame PTS values are non-decreasing (ignoring AV_NOPTS_VALUE).
func ValidateTimestamps(frames []*Frame) error {
	var last int64 = avutil.NoPTSValue
	for i, f := range frames {
		if f == nil || f.ptr == nil {
			continue
		}
		pts := avutil.GetFramePTS(f.ptr)
		if pts == avutil.NoPTSValue {
			continue
		}
		if last != avutil.NoPTSValue && pts < last {
			return fmt.Errorf("ffgo: non-monotonic PTS at index %d: prev=%d curr=%d", i, last, pts)
		}
		last = pts
	}
	return nil
}

// FrameRateDetect attempts to determine the effective frame rate for the decoder's video stream.
//
// It prefers stream metadata (avg_frame_rate) when available. If missing/invalid, it decodes a small
// sample of frames from the current position and estimates fps from PTS deltas.
//
// Note: This advances the decoder (it consumes packets/frames).
func FrameRateDetect(decoder *Decoder) (float64, error) {
	if decoder == nil || decoder.VideoStream() == nil {
		return 0, errors.New("ffgo: decoder has no video stream")
	}

	// Prefer stream-level avg frame rate if it looks valid.
	fr := decoder.VideoStream().FrameRate
	if fr.Den > 0 && fr.Num > 0 {
		fps := float64(fr.Num) / float64(fr.Den)
		if fps > 0.1 && fps < 240 {
			return fps, nil
		}
	}

	tb := decoder.VideoStream().TimeBase
	if tb.Den <= 0 || tb.Num <= 0 {
		return 0, errors.New("ffgo: invalid stream time base")
	}

	if err := decoder.OpenVideoDecoder(); err != nil {
		return 0, err
	}

	const maxFrames = 90
	var firstPTS int64 = avutil.NoPTSValue
	var lastPTS int64 = avutil.NoPTSValue
	var count int

	for count < maxFrames {
		f, err := decoder.DecodeVideo()
		if err != nil {
			return 0, err
		}
		if f.IsNil() {
			break
		}
		pts := avutil.GetFramePTS(f.ptr)
		if pts == avutil.NoPTSValue {
			continue
		}
		if firstPTS == avutil.NoPTSValue {
			firstPTS = pts
		}
		lastPTS = pts
		count++
	}

	if count < 2 || firstPTS == avutil.NoPTSValue || lastPTS == avutil.NoPTSValue || lastPTS <= firstPTS {
		return 0, errors.New("ffgo: insufficient PTS data to estimate frame rate")
	}

	seconds := float64(lastPTS-firstPTS) * float64(tb.Num) / float64(tb.Den)
	if seconds <= 0 {
		return 0, errors.New("ffgo: invalid duration while estimating frame rate")
	}
	return float64(count-1) / seconds, nil
}
