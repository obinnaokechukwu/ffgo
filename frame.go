//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"unsafe"

	"github.com/obinnaokechukwu/ffgo/avutil"
)

// FrameWrapper provides a high-level interface to an FFmpeg AVFrame.
// It wraps the low-level Frame (unsafe.Pointer) with convenient methods.
type FrameWrapper struct {
	frame     Frame
	mediaType MediaType
}

// WrapFrame creates a FrameWrapper from a raw Frame.
func WrapFrame(frame Frame, mediaType MediaType) *FrameWrapper {
	if frame.IsNil() {
		return nil
	}
	return &FrameWrapper{
		frame:     frame,
		mediaType: mediaType,
	}
}

// Raw returns the underlying raw FFmpeg frame.
func (f *FrameWrapper) Raw() Frame {
	return f.frame
}

// PTS returns the presentation timestamp of the frame.
func (f *FrameWrapper) PTS() int64 {
	if f == nil || f.frame.IsNil() {
		return avutil.NoPTSValue
	}
	return avutil.GetFramePTS(f.frame.ptr)
}

// MediaType returns the type of media (video or audio).
func (f *FrameWrapper) MediaType() MediaType {
	if f == nil {
		return MediaTypeUnknown
	}
	return f.mediaType
}

// Width returns the frame width (video only).
func (f *FrameWrapper) Width() int {
	if f == nil || f.frame.IsNil() {
		return 0
	}
	return int(avutil.GetFrameWidth(f.frame.ptr))
}

// Height returns the frame height (video only).
func (f *FrameWrapper) Height() int {
	if f == nil || f.frame.IsNil() {
		return 0
	}
	return int(avutil.GetFrameHeight(f.frame.ptr))
}

// Format returns the pixel format (video) or sample format (audio).
func (f *FrameWrapper) Format() int32 {
	if f == nil || f.frame.IsNil() {
		return -1
	}
	return avutil.GetFrameFormat(f.frame.ptr)
}

// PixelFormat returns the pixel format for video frames.
func (f *FrameWrapper) PixelFormat() PixelFormat {
	return PixelFormat(f.Format())
}

// SampleFormat returns the sample format for audio frames.
func (f *FrameWrapper) SampleFormat() SampleFormat {
	return SampleFormat(f.Format())
}

// Data returns a slice to the frame data for the specified plane.
// For video: plane 0 = Y, plane 1 = U/Cb, plane 2 = V/Cr for YUV formats.
// For audio: plane 0 contains interleaved samples (packed) or plane N for channel N (planar).
// Returns nil if the plane is not valid.
func (f *FrameWrapper) Data(plane int) []byte {
	if f == nil || f.frame.IsNil() || plane < 0 || plane >= 8 {
		return nil
	}

	data := avutil.GetFrameData(f.frame.ptr)
	linesize := avutil.GetFrameLinesize(f.frame.ptr)

	if data[plane] == nil {
		return nil
	}

	// Calculate the size based on frame type
	var size int
	if f.mediaType == MediaTypeVideo {
		height := f.Height()
		// For chroma planes in YUV420P, height is halved
		if plane > 0 && f.PixelFormat() == PixelFormatYUV420P {
			height /= 2
		}
		size = int(linesize[plane]) * height
	} else {
		// For audio, use linesize[0] which is the size of the plane
		size = int(linesize[plane])
	}

	if size <= 0 {
		return nil
	}

	return unsafe.Slice((*byte)(data[plane]), size)
}

// Linesize returns the line size (stride) for the specified plane.
func (f *FrameWrapper) Linesize(plane int) int {
	if f == nil || f.frame.IsNil() || plane < 0 || plane >= 8 {
		return 0
	}
	linesize := avutil.GetFrameLinesize(f.frame.ptr)
	return int(linesize[plane])
}

// NumSamples returns the number of audio samples in this frame (audio only).
func (f *FrameWrapper) NumSamples() int {
	if f == nil || f.frame.IsNil() {
		return 0
	}
	return int(avutil.GetFrameNbSamples(f.frame.ptr))
}

// SampleRate returns the sample rate for audio frames.
func (f *FrameWrapper) SampleRate() int {
	if f == nil || f.frame.IsNil() {
		return 0
	}
	return int(avutil.GetFrameSampleRate(f.frame.ptr))
}

// IsKeyFrame returns true if this is a keyframe (video only).
func (f *FrameWrapper) IsKeyFrame() bool {
	if f == nil || f.frame.IsNil() {
		return false
	}
	return avutil.GetFrameKeyFrame(f.frame.ptr) != 0
}

// Copy creates a reference to this frame.
// The returned frame shares the same data buffers.
func (f *FrameWrapper) Copy() (*FrameWrapper, error) {
	if f == nil || f.frame.IsNil() {
		return nil, nil
	}

	newFrame := avutil.FrameAlloc()
	if newFrame == nil {
		return nil, ErrOutOfMemory
	}

	if err := avutil.FrameRef(newFrame, f.frame.ptr); err != nil {
		avutil.FrameFree(&newFrame)
		return nil, err
	}

	return &FrameWrapper{
		frame:     Frame{ptr: newFrame, owned: true},
		mediaType: f.mediaType,
	}, nil
}

// Free releases the frame resources.
// After calling Free, the frame must not be used.
func (f *FrameWrapper) Free() error {
	if f == nil {
		return nil
	}
	return f.frame.Free()
}
