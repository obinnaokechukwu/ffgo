package ffgo

import (
	"fmt"
	"unsafe"

	"github.com/obinnaokechukwu/ffgo/avutil"
	"github.com/obinnaokechukwu/ffgo/swresample"
)

// AudioFormat describes audio characteristics for resampling
type AudioFormat struct {
	SampleRate    int           // e.g., 44100, 48000
	Channels      int           // e.g., 1, 2, 6
	ChannelLayout ChannelLayout // e.g., ChannelLayoutStereo
	SampleFormat  SampleFormat  // e.g., SampleFormatS16, SampleFormatFLTP
}

// ChannelLayout represents audio channel configuration
type ChannelLayout int64

const (
	ChannelLayoutMono        ChannelLayout = 0x4   // AV_CH_LAYOUT_MONO
	ChannelLayoutStereo      ChannelLayout = 0x3   // AV_CH_LAYOUT_STEREO
	ChannelLayout2Point1     ChannelLayout = 0xB   // AV_CH_LAYOUT_2POINT1
	ChannelLayoutSurround    ChannelLayout = 0x7   // AV_CH_LAYOUT_SURROUND
	ChannelLayout5Point0     ChannelLayout = 0x607 // AV_CH_LAYOUT_5POINT0
	ChannelLayout5Point1     ChannelLayout = 0x60F // AV_CH_LAYOUT_5POINT1
	ChannelLayout6Point1     ChannelLayout = 0x70F // AV_CH_LAYOUT_6POINT1
	ChannelLayout7Point1     ChannelLayout = 0x63F // AV_CH_LAYOUT_7POINT1
	ChannelLayout7Point1Wide ChannelLayout = 0xFF  // AV_CH_LAYOUT_7POINT1_WIDE
)

// Resampler converts audio between formats
type Resampler struct {
	ctx       swresample.SwrContext
	srcFormat AudioFormat
	dstFormat AudioFormat
	closed    bool
}

// NewResampler creates an audio resampler
//
// Example:
//
//	resampler, err := ffgo.NewResampler(
//	    ffgo.AudioFormat{SampleRate: 44100, Channels: 2, SampleFormat: ffgo.SampleFormatS16},
//	    ffgo.AudioFormat{SampleRate: 48000, Channels: 2, SampleFormat: ffgo.SampleFormatFLTP},
//	)
func NewResampler(src, dst AudioFormat) (*Resampler, error) {
	// Validate inputs
	if src.SampleRate <= 0 || dst.SampleRate <= 0 {
		return nil, fmt.Errorf("invalid sample rate: src=%d, dst=%d", src.SampleRate, dst.SampleRate)
	}
	if src.Channels <= 0 || dst.Channels <= 0 {
		return nil, fmt.Errorf("invalid channel count: src=%d, dst=%d", src.Channels, dst.Channels)
	}

	// Set default channel layouts if not specified
	if src.ChannelLayout == 0 {
		src.ChannelLayout = defaultChannelLayout(src.Channels)
	}
	if dst.ChannelLayout == 0 {
		dst.ChannelLayout = defaultChannelLayout(dst.Channels)
	}

	// Initialize swresample
	if err := swresample.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize swresample: %w", err)
	}

	// Prefer the AVChannelLayout API (FFmpeg 5.1+). FFmpeg 7 removed the legacy
	// swr_alloc_set_opts symbol in some builds, so this path is required on macOS CI.
	if ctx := swresample.Alloc(); ctx != nil {
		const avChannelLayoutSize = 24 // sizeof(AVChannelLayout) on 64-bit FFmpeg 5.1+

		outLayout := avutil.Malloc(avChannelLayoutSize)
		inLayout := avutil.Malloc(avChannelLayoutSize)
		if outLayout != nil && inLayout != nil {
			setChannelLayoutMask(outLayout, dst.Channels, uint64(dst.ChannelLayout))
			setChannelLayoutMask(inLayout, src.Channels, uint64(src.ChannelLayout))

			if err := swresample.AllocSetOpts2(&ctx, outLayout, inLayout,
				int32(dst.SampleFormat), int32(src.SampleFormat),
				int32(dst.SampleRate), int32(src.SampleRate)); err == nil {
				avutil.Free(outLayout)
				avutil.Free(inLayout)

				if err := swresample.InitContext(ctx); err != nil {
					swresample.Free(&ctx)
					return nil, fmt.Errorf("failed to initialize swresample context: %w", err)
				}
				return &Resampler{
					ctx:       ctx,
					srcFormat: src,
					dstFormat: dst,
				}, nil
			}
		}
		if outLayout != nil {
			avutil.Free(outLayout)
		}
		if inLayout != nil {
			avutil.Free(inLayout)
		}
		swresample.Free(&ctx)
	}

	// Fallback to legacy channel layout bitmask API (older FFmpeg).
	ctx := swresample.AllocSetOpts(nil,
		int64(dst.ChannelLayout), int32(dst.SampleFormat), int32(dst.SampleRate),
		int64(src.ChannelLayout), int32(src.SampleFormat), int32(src.SampleRate))
	if ctx == nil {
		return nil, fmt.Errorf("failed to allocate swresample context")
	}

	// Initialize the context
	if err := swresample.InitContext(ctx); err != nil {
		swresample.Free(&ctx)
		return nil, fmt.Errorf("failed to initialize swresample context: %w", err)
	}

	return &Resampler{
		ctx:       ctx,
		srcFormat: src,
		dstFormat: dst,
	}, nil
}

func setChannelLayoutMask(layout unsafe.Pointer, channels int, mask uint64) {
	if layout == nil {
		return
	}
	if channels <= 0 {
		return
	}

	// AVChannelLayout (FFmpeg 5.1+):
	// - order (int32): 0
	// - nb_channels (int32): 4
	// - u.mask (uint64): 8
	// - opaque (void*): 16
	const channelOrderNative = int32(1) // AV_CHANNEL_ORDER_NATIVE

	*(*int32)(layout) = channelOrderNative
	*(*int32)(unsafe.Add(layout, 4)) = int32(channels)
	*(*uint64)(unsafe.Add(layout, 8)) = mask
	*(*unsafe.Pointer)(unsafe.Add(layout, 16)) = nil
}

// Resample converts an audio frame
// Returns a new frame with resampled audio, or nil if more input needed
func (r *Resampler) Resample(frame Frame) (Frame, error) {
	if r.closed {
		return Frame{}, fmt.Errorf("resampler is closed")
	}
	if frame.IsNil() {
		return Frame{}, nil
	}

	// Allocate output frame
	outFrame := avutil.FrameAlloc()
	if outFrame == nil {
		return Frame{}, fmt.Errorf("failed to allocate output frame")
	}

	// Set output frame parameters
	avutil.FrameSetSampleRate(outFrame, int32(r.dstFormat.SampleRate))
	avutil.FrameSetChannels(outFrame, int32(r.dstFormat.Channels))
	avutil.FrameSetFormat(outFrame, int32(r.dstFormat.SampleFormat))

	// Calculate output samples
	inSamples := int(avutil.GetFrameNbSamples(frame.ptr))
	outSamples := swresample.GetOutSamples(r.ctx, inSamples)
	if outSamples <= 0 {
		outSamples = inSamples*r.dstFormat.SampleRate/r.srcFormat.SampleRate + 256
	}
	avutil.FrameSetNbSamples(outFrame, int32(outSamples))

	// Get buffer for output frame
	if err := avutil.FrameGetBufferErr(outFrame, 0); err != nil {
		avutil.FrameFree(&outFrame)
		return Frame{}, fmt.Errorf("failed to allocate output frame buffer: %w", err)
	}

	// Convert
	err := swresample.ConvertFrame(r.ctx, outFrame, frame.ptr)
	if err != nil {
		avutil.FrameFree(&outFrame)
		return Frame{}, fmt.Errorf("failed to convert frame: %w", err)
	}

	return Frame{ptr: outFrame, owned: true}, nil
}

// Flush drains any remaining samples from the resampler
func (r *Resampler) Flush() (Frame, error) {
	if r.closed {
		return Frame{}, fmt.Errorf("resampler is closed")
	}

	// Check if there's any delay
	delay := swresample.GetDelay(r.ctx, int64(r.dstFormat.SampleRate))
	if delay <= 0 {
		return Frame{}, nil
	}

	// Allocate output frame
	outFrame := avutil.FrameAlloc()
	if outFrame == nil {
		return Frame{}, fmt.Errorf("failed to allocate output frame")
	}

	// Set output frame parameters
	avutil.FrameSetSampleRate(outFrame, int32(r.dstFormat.SampleRate))
	avutil.FrameSetChannels(outFrame, int32(r.dstFormat.Channels))
	avutil.FrameSetFormat(outFrame, int32(r.dstFormat.SampleFormat))
	avutil.FrameSetNbSamples(outFrame, int32(delay))

	// Get buffer for output frame
	if err := avutil.FrameGetBufferErr(outFrame, 0); err != nil {
		avutil.FrameFree(&outFrame)
		return Frame{}, fmt.Errorf("failed to allocate output frame buffer: %w", err)
	}

	// Flush (convert with NULL input)
	err := swresample.ConvertFrame(r.ctx, outFrame, nil)
	if err != nil {
		avutil.FrameFree(&outFrame)
		return Frame{}, fmt.Errorf("failed to flush resampler: %w", err)
	}

	return Frame{ptr: outFrame, owned: true}, nil
}

// Close releases resources
func (r *Resampler) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	if r.ctx != nil {
		swresample.Free(&r.ctx)
	}
	return nil
}

// SrcFormat returns the source audio format
func (r *Resampler) SrcFormat() AudioFormat {
	return r.srcFormat
}

// DstFormat returns the destination audio format
func (r *Resampler) DstFormat() AudioFormat {
	return r.dstFormat
}

// defaultChannelLayout returns a default channel layout for the given number of channels
func defaultChannelLayout(channels int) ChannelLayout {
	switch channels {
	case 1:
		return ChannelLayoutMono
	case 2:
		return ChannelLayoutStereo
	case 3:
		return ChannelLayoutSurround
	case 6:
		return ChannelLayout5Point1
	case 8:
		return ChannelLayout7Point1
	default:
		// Return stereo for unknown, or compute based on channels
		return ChannelLayoutStereo
	}
}

// String returns the name of the channel layout
func (cl ChannelLayout) String() string {
	switch cl {
	case ChannelLayoutMono:
		return "mono"
	case ChannelLayoutStereo:
		return "stereo"
	case ChannelLayout2Point1:
		return "2.1"
	case ChannelLayoutSurround:
		return "surround"
	case ChannelLayout5Point0:
		return "5.0"
	case ChannelLayout5Point1:
		return "5.1"
	case ChannelLayout6Point1:
		return "6.1"
	case ChannelLayout7Point1:
		return "7.1"
	case ChannelLayout7Point1Wide:
		return "7.1(wide)"
	default:
		return fmt.Sprintf("custom(%d)", cl)
	}
}

// NumChannels returns the number of channels in this layout
func (cl ChannelLayout) NumChannels() int {
	// Count bits set in the layout
	count := 0
	layout := int64(cl)
	for layout > 0 {
		count += int(layout & 1)
		layout >>= 1
	}
	return count
}
