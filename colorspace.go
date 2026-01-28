//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"unsafe"

	"github.com/obinnaokechukwu/ffgo/avutil"
	"github.com/obinnaokechukwu/ffgo/swscale"
)

// AVColorSpace (frame metadata). Kept in sync with libavutil/pixfmt.h.
const (
	ColorSpaceUnspecified            ColorSpace = 0
	ColorSpaceBT709                  ColorSpace = 1
	ColorSpaceFCC                    ColorSpace = 4
	ColorSpaceBT470BG                ColorSpace = 5
	ColorSpaceSMPTE170M              ColorSpace = 6 // commonly used for BT.601
	ColorSpaceSMPTE240M              ColorSpace = 7
	ColorSpaceBT2020NCL              ColorSpace = 9
	ColorSpaceBT2020CL               ColorSpace = 10
	ColorSpaceSMPTE2085              ColorSpace = 11
	ColorSpaceChromaticityDerivedNCL ColorSpace = 12
	ColorSpaceChromaticityDerivedCL  ColorSpace = 13
	ColorSpaceICTCP                  ColorSpace = 14

	// Aliases for readability.
	ColorSpaceBT601  ColorSpace = ColorSpaceSMPTE170M
	ColorSpaceBT2020 ColorSpace = ColorSpaceBT2020NCL
)

// AVColorPrimaries (frame metadata). Kept in sync with libavutil/pixfmt.h.
const (
	ColorPrimariesUnspecified ColorPrimaries = 2
	ColorPrimariesBT709       ColorPrimaries = 1
	ColorPrimariesBT470M      ColorPrimaries = 4
	ColorPrimariesBT470BG     ColorPrimaries = 5
	ColorPrimariesSMPTE170M   ColorPrimaries = 6
	ColorPrimariesSMPTE240M   ColorPrimaries = 7
	ColorPrimariesFilm        ColorPrimaries = 8
	ColorPrimariesBT2020      ColorPrimaries = 9
	ColorPrimariesSMPTE428    ColorPrimaries = 10
	ColorPrimariesSMPTE431    ColorPrimaries = 11
	ColorPrimariesSMPTE432    ColorPrimaries = 12
	ColorPrimariesEBU3213     ColorPrimaries = 22
)

// AVColorTransferCharacteristic (frame metadata). Kept in sync with libavutil/pixfmt.h.
const (
	ColorTransferUnspecified  ColorTransfer = 2
	ColorTransferBT709        ColorTransfer = 1
	ColorTransferGamma22      ColorTransfer = 4
	ColorTransferGamma28      ColorTransfer = 5
	ColorTransferSMPTE170M    ColorTransfer = 6
	ColorTransferSMPTE240M    ColorTransfer = 7
	ColorTransferLinear       ColorTransfer = 8
	ColorTransferLog          ColorTransfer = 9
	ColorTransferLogSqrt      ColorTransfer = 10
	ColorTransferIEC61966_2_4 ColorTransfer = 11
	ColorTransferBT1361Ecg    ColorTransfer = 12
	ColorTransferIEC61966_2_1 ColorTransfer = 13 // sRGB
	ColorTransferBT2020_10    ColorTransfer = 14
	ColorTransferBT2020_12    ColorTransfer = 15
	ColorTransferSMPTE2084    ColorTransfer = 16 // PQ (BT.2100 PQ)
	ColorTransferSMPTE428     ColorTransfer = 17
	ColorTransferARIB_STD_B67 ColorTransfer = 18 // HLG (BT.2100 HLG)
)

// SetColorspace configures the swscale context to use explicit colorspace conversion matrices.
//
// This controls the conversion matrix (e.g. BT.601/BT.709/BT.2020) used by swscale.
// Range handling is controlled separately via Scaler.SetColorConversion.
//
// Primaries/transfer characteristics are metadata (see Frame.SetColorSpec) and are not
// directly applied by swscale, but can be set on frames for downstream consumers.
func (s *Scaler) SetColorspace(src, dst ColorSpace) error {
	if s == nil || s.ctx == nil {
		return errors.New("ffgo: scaler is closed")
	}
	if !swscale.HasColorspaceDetails() {
		return errors.New("ffgo: swscale colorspace details not available")
	}

	var invTable unsafe.Pointer
	var table unsafe.Pointer
	var srcRange int32
	var dstRange int32
	var brightness, contrast, saturation int32

	ret := swscale.GetColorspaceDetails(s.ctx, &invTable, &srcRange, &table, &dstRange, &brightness, &contrast, &saturation)
	if ret < 0 {
		return avutil.NewError(ret, "sws_getColorspaceDetails")
	}

	if src != ColorSpaceUnspecified {
		if coeff := swscale.GetCoefficients(toSwsColorspace(src)); coeff != nil {
			invTable = coeff
		}
	}
	if dst != ColorSpaceUnspecified {
		if coeff := swscale.GetCoefficients(toSwsColorspace(dst)); coeff != nil {
			table = coeff
		}
	}

	ret = swscale.SetColorspaceDetails(s.ctx, invTable, srcRange, table, dstRange, brightness, contrast, saturation)
	if ret < 0 {
		return avutil.NewError(ret, "sws_setColorspaceDetails")
	}
	return nil
}

// toSwsColorspace maps AVColorSpace values to swscale SWS_CS_* values.
//
// swscale uses SWS_CS_SMPTE170M == 5 for BT.601 coefficients, while AVColorSpace uses
// AVCOL_SPC_SMPTE170M == 6. This helper normalizes common cases.
func toSwsColorspace(cs ColorSpace) int32 {
	switch cs {
	case ColorSpaceSMPTE170M, ColorSpaceBT470BG:
		return 5 // SWS_CS_ITU601/SMPTE170M
	case ColorSpaceBT709:
		return 1 // SWS_CS_ITU709
	case ColorSpaceSMPTE240M:
		return 7 // SWS_CS_SMPTE240M
	case ColorSpaceBT2020NCL, ColorSpaceBT2020CL:
		return 9 // SWS_CS_BT2020
	default:
		// Best-effort: pass through (matches many values, but not all).
		return int32(cs)
	}
}
