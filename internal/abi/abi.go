//go:build !ios && !android && (amd64 || arm64)

// Package abi describes the public FFmpeg structure layouts used by ffgo.
//
// FFmpeg keeps these structures source-compatible within a release branch, but
// does not promise that their binary layout remains stable across major
// releases. ffgo accesses a small subset of their fields through purego, so it
// must select offsets from the versions of the libraries actually loaded.
package abi

import (
	"errors"
	"fmt"
)

// ErrUnsupported is returned for an unsupported or internally inconsistent
// set of FFmpeg shared libraries.
var ErrUnsupported = errors.New("ffgo: unsupported FFmpeg ABI")

// Layout contains every version-dependent structure layout used by ffgo.
type Layout struct {
	FFmpegMajor     int
	AVUtilMajor     int
	AVCodecMajor    int
	AVFormatMajor   int
	SWScaleMajor    int
	SWResampleMajor int
	AVFilterMajor   int
	AVDeviceMajor   int
	Frame           FrameLayout
	FormatContext   FormatContextLayout
	CodecParameters CodecParametersLayout
	CodecContext    CodecContextLayout
}

// FrameLayout contains offsets into AVFrame.
type FrameLayout struct {
	Data, Linesize, ExtendedData                         uintptr
	Width, Height, NbSamples, Format, KeyFrame, PTS      uintptr
	SampleRate, Buffer, ExtendedBuffer, NbExtendedBuffer uintptr
	ChannelLayout                                        uintptr
}

// FormatContextLayout contains offsets into AVFormatContext.
type FormatContextLayout struct {
	InputFormat, OutputFormat, IOContext, NumStreams, Streams uintptr
	Duration, BitRate, Flags                                  uintptr
	NumPrograms, Programs, NumChapters, Chapters, Metadata    uintptr
	ProbeScore                                                uintptr
}

// CodecParametersLayout contains offsets into AVCodecParameters.
type CodecParametersLayout struct {
	CodecType, CodecID, CodecTag, Extradata, ExtradataSize uintptr
	Format, Width, Height, SampleRate, ChannelLayout       uintptr
}

// CodecContextLayout contains offsets into AVCodecContext.
type CodecContextLayout struct {
	CodecType, CodecID, BitRate, Flags, TimeBase    uintptr
	Width, Height, GOPSize, PixelFormat, MaxBFrames uintptr
	SampleRate, SampleFormat, FrameSize, FrameRate  uintptr
	HWFramesContext, HWDeviceContext, ChannelLayout uintptr
}

var layouts = map[[3]int]Layout{
	{58, 60, 60}: {
		FFmpegMajor: 6, AVUtilMajor: 58, AVCodecMajor: 60, AVFormatMajor: 60,
		SWScaleMajor: 7, SWResampleMajor: 4, AVFilterMajor: 9, AVDeviceMajor: 60,
		Frame: FrameLayout{
			Data: 0, Linesize: 64, ExtendedData: 96,
			Width: 104, Height: 108, NbSamples: 112, Format: 116,
			KeyFrame: 120, PTS: 136, SampleRate: 208,
			Buffer: 224, ExtendedBuffer: 288, NbExtendedBuffer: 296,
			ChannelLayout: 448,
		},
		FormatContext: FormatContextLayout{
			InputFormat: 8, OutputFormat: 16, IOContext: 32,
			NumStreams: 44, Streams: 48, Duration: 72, BitRate: 80,
			Flags: 96, NumPrograms: 132, Programs: 136,
			NumChapters: 164, Chapters: 168, Metadata: 176,
			ProbeScore: 300,
		},
		CodecParameters: CodecParametersLayout{
			CodecType: 0, CodecID: 4, CodecTag: 8, Extradata: 16,
			ExtradataSize: 24, Format: 28, Width: 56, Height: 60,
			SampleRate: 116, ChannelLayout: 144,
		},
		CodecContext: CodecContextLayout{
			CodecType: 12, CodecID: 24, BitRate: 56, Flags: 76,
			TimeBase: 100, Width: 116, Height: 120, GOPSize: 132,
			PixelFormat: 136, MaxBFrames: 160, SampleRate: 352,
			SampleFormat: 360, FrameSize: 364, FrameRate: 704,
			HWFramesContext: 840, HWDeviceContext: 864, ChannelLayout: 912,
		},
	},
	{59, 61, 61}: {
		FFmpegMajor: 7, AVUtilMajor: 59, AVCodecMajor: 61, AVFormatMajor: 61,
		SWScaleMajor: 8, SWResampleMajor: 5, AVFilterMajor: 10, AVDeviceMajor: 61,
		Frame: FrameLayout{
			Data: 0, Linesize: 64, ExtendedData: 96,
			Width: 104, Height: 108, NbSamples: 112, Format: 116,
			KeyFrame: 120, PTS: 136, SampleRate: 192,
			Buffer: 200, ExtendedBuffer: 264, NbExtendedBuffer: 272,
			ChannelLayout: 408,
		},
		FormatContext: FormatContextLayout{
			InputFormat: 8, OutputFormat: 16, IOContext: 32,
			NumStreams: 44, Streams: 48, Duration: 104, BitRate: 112,
			Flags: 128, NumPrograms: 164, Programs: 168,
			NumChapters: 72, Chapters: 80, Metadata: 192,
			ProbeScore: 324,
		},
		CodecParameters: CodecParametersLayout{
			CodecType: 0, CodecID: 4, CodecTag: 8, Extradata: 16,
			ExtradataSize: 24, Format: 44, Width: 72, Height: 76,
			SampleRate: 152, ChannelLayout: 128,
		},
		CodecContext: CodecContextLayout{
			CodecType: 12, CodecID: 24, BitRate: 56, Flags: 64,
			TimeBase: 84, Width: 116, Height: 120, GOPSize: 332,
			PixelFormat: 140, MaxBFrames: 200, SampleRate: 344,
			SampleFormat: 348, FrameSize: 376, FrameRate: 100,
			HWFramesContext: 552, HWDeviceContext: 560, ChannelLayout: 352,
		},
	},
}

// Detect selects a layout from the runtime library versions. Version values
// use FFmpeg's AV_VERSION_INT representation.
func Detect(avutilVersion, avcodecVersion, avformatVersion uint32) (Layout, error) {
	key := [3]int{major(avutilVersion), major(avcodecVersion), major(avformatVersion)}
	if layout, ok := layouts[key]; ok {
		return layout, nil
	}
	return Layout{}, fmt.Errorf(
		"%w: libavutil %d, libavcodec %d, libavformat %d; supported combinations are FFmpeg 6 (58/60/60) and FFmpeg 7 (59/61/61)",
		ErrUnsupported, key[0], key[1], key[2],
	)
}

// LibraryMajor returns the shared-library major expected by this FFmpeg ABI.
func (l Layout) LibraryMajor(name string) (int, bool) {
	switch name {
	case "avutil":
		return l.AVUtilMajor, true
	case "avcodec":
		return l.AVCodecMajor, true
	case "avformat":
		return l.AVFormatMajor, true
	case "swscale":
		return l.SWScaleMajor, true
	case "swresample":
		return l.SWResampleMajor, true
	case "avfilter":
		return l.AVFilterMajor, true
	case "avdevice":
		return l.AVDeviceMajor, true
	default:
		return 0, false
	}
}

func major(version uint32) int {
	return int(version >> 16)
}
