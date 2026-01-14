//go:build !ios && !android && (amd64 || arm64)

package avcodec

// CodecID represents FFmpeg codec identifiers.
type CodecID int32

// Video codec IDs
const (
	CodecIDNone CodecID = 0

	// Video codecs
	CodecIDMPEG1VIDEO CodecID = 1
	CodecIDMPEG2VIDEO CodecID = 2
	CodecIDH261       CodecID = 3
	CodecIDH263       CodecID = 4
	CodecIDRV10       CodecID = 5
	CodecIDRV20       CodecID = 6
	CodecIDMJPEG      CodecID = 7
	CodecIDMJPEGB     CodecID = 8
	CodecIDLJPEG      CodecID = 9
	CodecIDSP5X       CodecID = 10
	CodecIDJPEGLS     CodecID = 11
	CodecIDMPEG4      CodecID = 12
	CodecIDRAWVIDEO   CodecID = 13
	CodecIDMSMPEG4V1  CodecID = 14
	CodecIDMSMPEG4V2  CodecID = 15
	CodecIDMSMPEG4V3  CodecID = 16
	CodecIDWMV1       CodecID = 17
	CodecIDWMV2       CodecID = 18
	CodecIDH263P      CodecID = 19
	CodecIDH263I      CodecID = 20
	CodecIDFLV1       CodecID = 21
	CodecIDSVQ1       CodecID = 22
	CodecIDSVQ3       CodecID = 23
	CodecIDDVVIDEO    CodecID = 24
	CodecIDHUFFYUV    CodecID = 25
	CodecIDCYUV       CodecID = 26
	CodecIDH264       CodecID = 27
	CodecIDINDEO3     CodecID = 28
	CodecIDVP3        CodecID = 29
	CodecIDTHEORA     CodecID = 30

	CodecIDVP5 CodecID = 60
	CodecIDVP6 CodecID = 61
	CodecIDVP7 CodecID = 65
	CodecIDVP8 CodecID = 139
	CodecIDVP9 CodecID = 167

	CodecIDHEVC CodecID = 173 // H.265
	CodecIDAV1  CodecID = 226

	// Image codecs
	CodecIDPNG CodecID = 61   // PNG
	CodecIDBMP CodecID = 66   // BMP
	CodecIDGIF CodecID = 97   // GIF
	CodecIDTIFF CodecID = 98  // TIFF
	CodecIDWEBP CodecID = 224 // WebP

	// Audio codecs (start at 0x10000 in FFmpeg, but we use actual values)
	CodecIDPCMS16LE CodecID = 65536
	CodecIDPCMS16BE CodecID = 65537
	CodecIDPCMU16LE CodecID = 65538
	CodecIDPCMU16BE CodecID = 65539
	CodecIDPCMS8    CodecID = 65540
	CodecIDPCMU8    CodecID = 65541

	CodecIDMP2     CodecID = 86016
	CodecIDMP3     CodecID = 86017
	CodecIDAAC     CodecID = 86018
	CodecIDAC3     CodecID = 86019
	CodecIDDTS     CodecID = 86020
	CodecIDVORBIS  CodecID = 86021
	CodecIDFLAC    CodecID = 86028
	CodecIDOPUS    CodecID = 86076
	CodecIDALAC    CodecID = 86032

	// Data/Attachment codec IDs (FFmpeg 0x18000+)
	CodecIDTTF     CodecID = 98304 // TrueType font
	CodecIDBinData CodecID = 98312 // Binary data (generic attachment)
)

// String returns the string representation of the codec ID.
func (id CodecID) String() string {
	switch id {
	case CodecIDNone:
		return "none"
	case CodecIDH264:
		return "h264"
	case CodecIDHEVC:
		return "hevc"
	case CodecIDAV1:
		return "av1"
	case CodecIDVP8:
		return "vp8"
	case CodecIDVP9:
		return "vp9"
	case CodecIDMPEG4:
		return "mpeg4"
	case CodecIDMJPEG:
		return "mjpeg"
	case CodecIDAAC:
		return "aac"
	case CodecIDMP3:
		return "mp3"
	case CodecIDOPUS:
		return "opus"
	case CodecIDFLAC:
		return "flac"
	default:
		return "unknown"
	}
}

// IsVideo returns true if the codec ID is for a video codec.
func (id CodecID) IsVideo() bool {
	return id > 0 && id < 65536
}

// IsAudio returns true if the codec ID is for an audio codec.
func (id CodecID) IsAudio() bool {
	return id >= 65536
}
