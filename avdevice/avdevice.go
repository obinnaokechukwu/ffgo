// Package avdevice provides minimal bindings to FFmpeg's libavdevice.
//
// ffgo uses avdevice for device/screen capture input formats (v4l2, x11grab, dshow, avfoundation, ...).
// This package is intentionally small: we only bind the registration entry point so that device demuxers
// become visible via avformat.FindInputFormat().
package avdevice

import (
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/obinnaokechukwu/ffgo/internal/bindings"
)

var (
	libAVDevice uintptr
	initOnce    sync.Once
	initErr     error

	avdevice_register_all func()
)

// Init loads libavdevice and registers the minimal function bindings.
func Init() error {
	initOnce.Do(func() {
		var err error
		// libavdevice major versions track FFmpeg majors (common: 58-61).
		libAVDevice, err = bindings.LoadLibrary("avdevice", []int{61, 60, 59, 58})
		if err != nil {
			initErr = fmt.Errorf("avdevice: failed to load library: %w", err)
			return
		}

		purego.RegisterLibFunc(&avdevice_register_all, libAVDevice, "avdevice_register_all")
	})
	return initErr
}

// RegisterAll registers all device demuxers/muxers with FFmpeg.
// Calling this makes device input formats discoverable via avformat.FindInputFormat.
func RegisterAll() error {
	if err := Init(); err != nil {
		return err
	}
	if avdevice_register_all != nil {
		avdevice_register_all()
	}
	return nil
}

