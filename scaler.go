//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"

	"github.com/obinnaokechukwu/ffgo/avutil"
	"github.com/obinnaokechukwu/ffgo/internal/bindings"
	"github.com/obinnaokechukwu/ffgo/swscale"
)

// ScaleFlags controls the scaling algorithm.
type ScaleFlags int32

const (
	// ScaleFastBilinear uses fast bilinear scaling (lowest quality, fastest).
	ScaleFastBilinear ScaleFlags = swscale.FlagFastBilinear

	// ScaleBilinear uses bilinear scaling (good balance of quality/speed).
	ScaleBilinear ScaleFlags = swscale.FlagBilinear

	// ScaleBicubic uses bicubic scaling (high quality).
	ScaleBicubic ScaleFlags = swscale.FlagBicubic

	// ScaleLanczos uses Lanczos scaling (highest quality, slowest).
	ScaleLanczos ScaleFlags = swscale.FlagLanczos

	// ScalePoint uses nearest neighbor (fastest, no interpolation).
	ScalePoint ScaleFlags = swscale.FlagPoint
)

// Scaler converts between pixel formats and scales video frames.
type Scaler struct {
	ctx swscale.Context

	srcWidth  int
	srcHeight int
	srcFormat PixelFormat

	dstWidth  int
	dstHeight int
	dstFormat PixelFormat

	// Reusable destination frame
	dstFrame avutil.Frame
}

// ScalerConfig contains configuration for creating a Scaler.
type ScalerConfig struct {
	SrcWidth  int
	SrcHeight int
	SrcFormat PixelFormat

	DstWidth  int
	DstHeight int
	DstFormat PixelFormat

	Flags ScaleFlags
}

// NewScaler creates a new scaler with the specified parameters.
// This is the recommended way to create scalers.
func NewScaler(srcW, srcH int, srcFmt PixelFormat, dstW, dstH int, dstFmt PixelFormat, flags ScaleFlags) (*Scaler, error) {
	return NewScalerWithConfig(ScalerConfig{
		SrcWidth:  srcW,
		SrcHeight: srcH,
		SrcFormat: srcFmt,
		DstWidth:  dstW,
		DstHeight: dstH,
		DstFormat: dstFmt,
		Flags:     flags,
	})
}

// NewScalerWithConfig creates a new scaler for the given configuration.
func NewScalerWithConfig(cfg ScalerConfig) (*Scaler, error) {
	// Ensure FFmpeg is loaded
	if err := bindings.Load(); err != nil {
		return nil, err
	}

	if !bindings.HasSWScale() {
		return nil, errors.New("ffgo: swscale library not available")
	}

	// Validate parameters
	if cfg.SrcWidth <= 0 || cfg.SrcHeight <= 0 {
		return nil, errors.New("ffgo: invalid source dimensions")
	}
	if cfg.DstWidth <= 0 || cfg.DstHeight <= 0 {
		return nil, errors.New("ffgo: invalid destination dimensions")
	}

	// Default to bilinear if no flags specified
	flags := cfg.Flags
	if flags == 0 {
		flags = ScaleBilinear
	}

	// Create swscale context
	ctx := swscale.GetContext(
		cfg.SrcWidth, cfg.SrcHeight, cfg.SrcFormat,
		cfg.DstWidth, cfg.DstHeight, cfg.DstFormat,
		int32(flags), nil, nil, nil,
	)
	if ctx == nil {
		return nil, errors.New("ffgo: failed to create scaler context")
	}

	s := &Scaler{
		ctx:       ctx,
		srcWidth:  cfg.SrcWidth,
		srcHeight: cfg.SrcHeight,
		srcFormat: cfg.SrcFormat,
		dstWidth:  cfg.DstWidth,
		dstHeight: cfg.DstHeight,
		dstFormat: cfg.DstFormat,
	}

	// Allocate destination frame
	s.dstFrame = avutil.FrameAlloc()
	if s.dstFrame == nil {
		swscale.FreeContext(ctx)
		return nil, errors.New("ffgo: failed to allocate destination frame")
	}

	// Set up destination frame
	avutil.SetFrameWidth(s.dstFrame, int32(cfg.DstWidth))
	avutil.SetFrameHeight(s.dstFrame, int32(cfg.DstHeight))
	avutil.SetFrameFormat(s.dstFrame, int32(cfg.DstFormat))

	// Allocate buffer
	if err := avutil.FrameGetBufferErr(s.dstFrame, 0); err != nil {
		avutil.FrameFree(&s.dstFrame)
		swscale.FreeContext(ctx)
		return nil, err
	}

	return s, nil
}

// Scale converts and scales the source frame.
// Returns the scaled frame (owned by Scaler, copy if you need to keep it).
func (s *Scaler) Scale(src Frame) (Frame, error) {
	if s.ctx == nil {
		return nil, errors.New("ffgo: scaler is closed")
	}

	// Make destination writable
	if err := avutil.FrameMakeWritable(s.dstFrame); err != nil {
		return nil, err
	}

	// Perform scaling
	ret := swscale.ScaleFrame(s.ctx, s.dstFrame, src)
	if ret < 0 {
		return nil, avutil.NewError(ret, "sws_scale_frame")
	}

	return s.dstFrame, nil
}

// ScaleTo scales the source frame into the provided destination frame.
// The destination frame must already have its format, width, and height set,
// and must have buffers allocated.
func (s *Scaler) ScaleTo(dst, src Frame) error {
	if s.ctx == nil {
		return errors.New("ffgo: scaler is closed")
	}

	ret := swscale.ScaleFrame(s.ctx, dst, src)
	if ret < 0 {
		return avutil.NewError(ret, "sws_scale_frame")
	}

	return nil
}

// SrcWidth returns the source width.
func (s *Scaler) SrcWidth() int {
	return s.srcWidth
}

// SrcHeight returns the source height.
func (s *Scaler) SrcHeight() int {
	return s.srcHeight
}

// SrcFormat returns the source pixel format.
func (s *Scaler) SrcFormat() PixelFormat {
	return s.srcFormat
}

// DstWidth returns the destination width.
func (s *Scaler) DstWidth() int {
	return s.dstWidth
}

// DstHeight returns the destination height.
func (s *Scaler) DstHeight() int {
	return s.dstHeight
}

// DstFormat returns the destination pixel format.
func (s *Scaler) DstFormat() PixelFormat {
	return s.dstFormat
}

// Close releases all resources.
func (s *Scaler) Close() error {
	if s.dstFrame != nil {
		avutil.FrameFree(&s.dstFrame)
	}
	if s.ctx != nil {
		swscale.FreeContext(s.ctx)
		s.ctx = nil
	}
	return nil
}
