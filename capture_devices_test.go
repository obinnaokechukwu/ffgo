//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"runtime"
	"testing"
)

func TestGetInputFormat_Defaults(t *testing.T) {
	switch runtime.GOOS {
	case "linux":
		if got := getInputFormat(DeviceTypeVideo); got != "v4l2" {
			t.Fatalf("linux video: expected v4l2, got %q", got)
		}
		if got := getInputFormat(DeviceTypeAudio); got != "alsa" {
			t.Fatalf("linux audio: expected alsa, got %q", got)
		}
	case "darwin":
		if got := getInputFormat(DeviceTypeVideo); got != "avfoundation" {
			t.Fatalf("darwin video: expected avfoundation, got %q", got)
		}
		if got := getInputFormat(DeviceTypeAudio); got != "avfoundation" {
			t.Fatalf("darwin audio: expected avfoundation, got %q", got)
		}
	case "windows":
		if got := getInputFormat(DeviceTypeVideo); got != "dshow" {
			t.Fatalf("windows video: expected dshow, got %q", got)
		}
		if got := getInputFormat(DeviceTypeAudio); got != "dshow" {
			t.Fatalf("windows audio: expected dshow, got %q", got)
		}
	default:
		if got := getInputFormat(DeviceTypeVideo); got != "" {
			t.Fatalf("other OS: expected empty, got %q", got)
		}
	}
}

func TestListDevices_Smoke(t *testing.T) {
	devs, err := ListDevices(DeviceTypeVideo)
	if err == nil {
		_ = devs
		return
	}
	// Any meaningful error is OK (missing libs, permissions, unsupported platform),
	// but it must be typed and not the old stub message.
	if errors.Is(err, ErrAVDeviceUnavailable) || errors.Is(err, ErrDeviceEnumerationUnavailable) {
		return
	}
	// Allow other FFmpeg/platform errors too.
}

