//go:build !ios && !android && (amd64 || arm64)

package abi

import (
	"errors"
	"testing"
)

func version(major, minor, patch uint32) uint32 {
	return major<<16 | minor<<8 | patch
}

func TestDetectSupportedLayouts(t *testing.T) {
	tests := []struct {
		name                                              string
		versions                                          [3]uint32
		ffmpeg, duration, frameSampleRate                 uintptr
		codecParFormat, codecParSampleRate, channelLayout uintptr
	}{
		{
			name: "FFmpeg 6", versions: [3]uint32{version(58, 29, 100), version(60, 31, 102), version(60, 16, 100)},
			ffmpeg: 6, duration: 72, frameSampleRate: 208,
			codecParFormat: 28, codecParSampleRate: 116, channelLayout: 912,
		},
		{
			name: "FFmpeg 7", versions: [3]uint32{version(59, 39, 100), version(61, 19, 100), version(61, 7, 100)},
			ffmpeg: 7, duration: 104, frameSampleRate: 192,
			codecParFormat: 44, codecParSampleRate: 152, channelLayout: 352,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout, err := Detect(tt.versions[0], tt.versions[1], tt.versions[2])
			if err != nil {
				t.Fatal(err)
			}
			if uintptr(layout.FFmpegMajor) != tt.ffmpeg {
				t.Fatalf("FFmpeg major = %d, want %d", layout.FFmpegMajor, tt.ffmpeg)
			}
			if layout.FormatContext.Duration != tt.duration {
				t.Fatalf("duration offset = %d, want %d", layout.FormatContext.Duration, tt.duration)
			}
			if layout.Frame.SampleRate != tt.frameSampleRate {
				t.Fatalf("frame sample_rate offset = %d, want %d", layout.Frame.SampleRate, tt.frameSampleRate)
			}
			if layout.CodecParameters.Format != tt.codecParFormat {
				t.Fatalf("codecpar format offset = %d, want %d", layout.CodecParameters.Format, tt.codecParFormat)
			}
			if layout.CodecParameters.SampleRate != tt.codecParSampleRate {
				t.Fatalf("codecpar sample_rate offset = %d, want %d", layout.CodecParameters.SampleRate, tt.codecParSampleRate)
			}
			if layout.CodecContext.ChannelLayout != tt.channelLayout {
				t.Fatalf("codec ch_layout offset = %d, want %d", layout.CodecContext.ChannelLayout, tt.channelLayout)
			}
		})
	}
}

func TestDetectRejectsMixedLibraries(t *testing.T) {
	_, err := Detect(version(59, 39, 100), version(60, 31, 102), version(61, 7, 100))
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Detect() error = %v, want ErrUnsupported", err)
	}
}

func TestDetectRejectsUnknownMajor(t *testing.T) {
	_, err := Detect(version(60, 1, 0), version(62, 1, 0), version(62, 1, 0))
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Detect() error = %v, want ErrUnsupported", err)
	}
}

func TestLibraryMajor(t *testing.T) {
	layout, err := Detect(version(59, 39, 100), version(61, 19, 100), version(61, 7, 100))
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := layout.LibraryMajor("swresample"); !ok || got != 5 {
		t.Fatalf("LibraryMajor(swresample) = %d, %v; want 5, true", got, ok)
	}
	if _, ok := layout.LibraryMajor("unknown"); ok {
		t.Fatal("LibraryMajor(unknown) unexpectedly succeeded")
	}
}
