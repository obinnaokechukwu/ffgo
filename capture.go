//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"github.com/obinnaokechukwu/ffgo/avdevice"
	"github.com/obinnaokechukwu/ffgo/avformat"
	"github.com/obinnaokechukwu/ffgo/avutil"
	"github.com/obinnaokechukwu/ffgo/internal/bindings"
	"github.com/obinnaokechukwu/ffgo/internal/shim"
)

// DeviceType represents a capture device type.
type DeviceType int

const (
	// DeviceTypeVideo represents video capture devices (cameras, screen capture).
	DeviceTypeVideo DeviceType = iota
	// DeviceTypeAudio represents audio capture devices (microphones).
	DeviceTypeAudio
)

// DeviceInfo contains information about a capture device.
type DeviceInfo struct {
	Name        string
	Description string
	DeviceType  DeviceType
}

// DeviceListOptions configures device enumeration behavior.
type DeviceListOptions struct {
	// InputFormat is the FFmpeg input format used for device enumeration
	// (e.g. "v4l2", "alsa", "dshow", "avfoundation").
	// If empty, ffgo uses platform defaults based on deviceType.
	InputFormat string

	// DeviceName is an optional selector string passed to FFmpeg.
	DeviceName string

	// AVOptions are forwarded to FFmpeg's device listing function.
	AVOptions map[string]string
}

// CaptureConfig configures capture from a hardware device.
type CaptureConfig struct {
	// Device specifies the device to capture from.
	// Linux: "/dev/video0", ":0.0" (X11 screen), etc.
	// macOS: "0" (device index), "default" (default device)
	// Windows: "HD Webcam" (device name from dshow)
	Device string

	// DeviceType specifies whether this is a video or audio device.
	DeviceType DeviceType

	// Width specifies the capture width for video devices.
	// If 0, uses device default.
	Width int

	// Height specifies the capture height for video devices.
	// If 0, uses device default.
	Height int

	// FrameRate specifies the capture frame rate for video devices.
	// If zero, uses device default.
	FrameRate Rational

	// SampleRate specifies the sample rate for audio devices.
	// If 0, uses device default.
	SampleRate int

	// Channels specifies the number of channels for audio devices.
	// If 0, uses device default.
	Channels int

	// PixelFormat specifies the pixel format for video capture.
	// If unset, uses device default.
	PixelFormat PixelFormat
}

// ListDevices returns available capture devices of the specified type.
// Note: Device enumeration requires FFmpeg's libavdevice and may not be
// available on all platforms. Returns an error if device enumeration
// is not supported.
func ListDevices(deviceType DeviceType) ([]DeviceInfo, error) {
	return ListDevicesWithOptions(deviceType, nil)
}

// ListDevicesWithOptions returns available capture devices of the specified type
// using optional enumeration settings.
func ListDevicesWithOptions(deviceType DeviceType, opts *DeviceListOptions) ([]DeviceInfo, error) {
	if err := bindings.Load(); err != nil {
		return nil, err
	}

	// libavdevice is required (for underlying enumeration API and device demuxers)
	if err := avdevice.Init(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAVDeviceUnavailable, err)
	}
	_ = avdevice.RegisterAll()

	// The shim is required for enumeration (avoids parsing FFmpeg structs in Go).
	_ = shim.Load()
	if !shim.IsLoaded() {
		return nil, ErrDeviceEnumerationUnavailable
	}

	if opts == nil {
		opts = &DeviceListOptions{}
	}
	formatName := opts.InputFormat
	if formatName == "" {
		formatName = getInputFormat(deviceType)
	}
	if formatName == "" {
		return nil, ErrDeviceEnumerationUnavailable
	}

	// Build options dictionary
	var avDict avutil.Dictionary
	for k, v := range opts.AVOptions {
		if err := avutil.DictSet(&avDict, k, v, 0); err != nil {
			if avDict != nil {
				avutil.DictFree(&avDict)
			}
			return nil, err
		}
	}
	defer func() {
		if avDict != nil {
			avutil.DictFree(&avDict)
		}
	}()

	count, namesPtr, descsPtr, err := shim.AVDeviceListInputSources(formatName, opts.DeviceName, avDict)
	if err != nil {
		return nil, ErrDeviceEnumerationUnavailable
	}
	defer func() {
		shim.AVDeviceFreeStringArray(namesPtr, count)
		shim.AVDeviceFreeStringArray(descsPtr, count)
	}()

	names := cStringArrayToGo(namesPtr, count)
	descs := cStringArrayToGo(descsPtr, count)

	n := len(names)
	if len(descs) < n {
		n = len(descs)
	}
	out := make([]DeviceInfo, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, DeviceInfo{
			Name:        names[i],
			Description: descs[i],
			DeviceType:  deviceType,
		})
	}
	return out, nil
}

func cStringToGo(ptr unsafe.Pointer) string {
	if ptr == nil {
		return ""
	}
	// FFmpeg strings can be long; cap scanning to a reasonable limit.
	const max = 4096
	n := 0
	for n < max {
		if *(*byte)(unsafe.Add(ptr, n)) == 0 {
			break
		}
		n++
	}
	if n == 0 {
		return ""
	}
	return string(unsafe.Slice((*byte)(ptr), n))
}

func cStringArrayToGo(arr unsafe.Pointer, count int) []string {
	if arr == nil || count <= 0 {
		return nil
	}
	s := unsafe.Slice((*unsafe.Pointer)(arr), count)
	out := make([]string, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, cStringToGo(s[i]))
	}
	return out
}

// NewCapture creates a decoder that captures from a hardware device.
// This is useful for capturing from cameras, microphones, screens, etc.
//
// Example (Linux webcam):
//
//	decoder, err := ffgo.NewCapture(ffgo.CaptureConfig{
//	    Device:     "/dev/video0",
//	    DeviceType: ffgo.DeviceTypeVideo,
//	    Width:      1920,
//	    Height:     1080,
//	    FrameRate:  ffgo.NewRational(30, 1),
//	})
//
// Example (macOS camera):
//
//	decoder, err := ffgo.NewCapture(ffgo.CaptureConfig{
//	    Device:     "0",
//	    DeviceType: ffgo.DeviceTypeVideo,
//	})
func NewCapture(cfg CaptureConfig) (*Decoder, error) {
	if err := bindings.Load(); err != nil {
		return nil, err
	}
	if err := avdevice.RegisterAll(); err != nil {
		return nil, fmt.Errorf("ffgo: device capture requires libavdevice: %w", err)
	}

	if cfg.Device == "" {
		return nil, errors.New("ffgo: device must be specified")
	}

	// Determine the input format based on platform and device type
	inputFormatName := getInputFormat(cfg.DeviceType)
	if inputFormatName == "" {
		return nil, fmt.Errorf("ffgo: capture not supported on this platform")
	}

	// Get the input format
	inputFmt := avformat.FindInputFormat(inputFormatName)
	if inputFmt == nil {
		return nil, fmt.Errorf("ffgo: input format %s not found (is libavdevice installed?)", inputFormatName)
	}

	// Build options dictionary
	var avDict avutil.Dictionary

	// Set video capture options
	if cfg.DeviceType == DeviceTypeVideo {
		if cfg.Width > 0 && cfg.Height > 0 {
			if err := avutil.DictSet(&avDict, "video_size", fmt.Sprintf("%dx%d", cfg.Width, cfg.Height), 0); err != nil {
				if avDict != nil {
					avutil.DictFree(&avDict)
				}
				return nil, err
			}
		}
		if cfg.FrameRate.Num > 0 && cfg.FrameRate.Den > 0 {
			if err := avutil.DictSet(&avDict, "framerate", fmt.Sprintf("%d/%d", cfg.FrameRate.Num, cfg.FrameRate.Den), 0); err != nil {
				if avDict != nil {
					avutil.DictFree(&avDict)
				}
				return nil, err
			}
		}
		if cfg.PixelFormat != PixelFormatNone {
			if err := avutil.DictSet(&avDict, "pixel_format", getPixelFormatName(cfg.PixelFormat), 0); err != nil {
				if avDict != nil {
					avutil.DictFree(&avDict)
				}
				return nil, err
			}
		}
	}

	// Set audio capture options
	if cfg.DeviceType == DeviceTypeAudio {
		if cfg.SampleRate > 0 {
			if err := avutil.DictSet(&avDict, "sample_rate", fmt.Sprintf("%d", cfg.SampleRate), 0); err != nil {
				if avDict != nil {
					avutil.DictFree(&avDict)
				}
				return nil, err
			}
		}
		if cfg.Channels > 0 {
			if err := avutil.DictSet(&avDict, "channels", fmt.Sprintf("%d", cfg.Channels), 0); err != nil {
				if avDict != nil {
					avutil.DictFree(&avDict)
				}
				return nil, err
			}
		}
	}

	// Build the device URL based on platform
	deviceURL := buildDeviceURL(cfg.Device, cfg.DeviceType)

	// Create decoder struct
	d := &Decoder{
		videoStreamIdx: -1,
		audioStreamIdx: -1,
	}

	// Open the input with specific format
	if err := avformat.OpenInput(&d.formatCtx, deviceURL, inputFmt, &avDict); err != nil {
		if avDict != nil {
			avutil.DictFree(&avDict)
		}
		return nil, fmt.Errorf("ffgo: failed to open capture device: %w", err)
	}

	// Free any remaining dictionary entries
	if avDict != nil {
		avutil.DictFree(&avDict)
	}

	// Find stream info
	if err := avformat.FindStreamInfo(d.formatCtx, nil); err != nil {
		avformat.CloseInput(&d.formatCtx)
		return nil, fmt.Errorf("ffgo: failed to find stream info: %w", err)
	}

	// Find best streams (same as regular decoder)
	d.videoStreamIdx = int(avformat.FindBestStream(d.formatCtx, avutil.MediaTypeVideo, -1, -1, nil, 0))
	if d.videoStreamIdx >= 0 {
		d.videoInfo = d.getStreamInfo(d.videoStreamIdx)
	}

	d.audioStreamIdx = int(avformat.FindBestStream(d.formatCtx, avutil.MediaTypeAudio, -1, -1, nil, 0))
	if d.audioStreamIdx >= 0 {
		d.audioInfo = d.getStreamInfo(d.audioStreamIdx)
	}

	return d, nil
}

// getInputFormat returns the FFmpeg input format name for capture on the current platform.
func getInputFormat(deviceType DeviceType) string {
	switch runtime.GOOS {
	case "linux":
		if deviceType == DeviceTypeVideo {
			return "v4l2" // Video4Linux2
		}
		return "alsa" // ALSA audio
	case "darwin":
		return "avfoundation" // macOS uses avfoundation for both audio and video
	case "windows":
		return "dshow" // DirectShow for Windows
	default:
		return ""
	}
}

// buildDeviceURL builds the device URL based on platform and device type.
func buildDeviceURL(device string, deviceType DeviceType) string {
	switch runtime.GOOS {
	case "linux":
		// Linux v4l2/alsa: device path is used directly
		return device
	case "darwin":
		// macOS avfoundation: "video_device_index:audio_device_index"
		// or "device_name" or ":audio_device_index"
		if deviceType == DeviceTypeVideo {
			// Video device - if numeric, use as index; otherwise use as name
			if _, err := fmt.Sscanf(device, "%d", new(int)); err == nil {
				return device + ":" // Video only: "0:"
			}
			return device + ":"
		}
		// Audio device
		return ":" + device
	case "windows":
		// Windows dshow: "video=device_name" or "audio=device_name"
		if deviceType == DeviceTypeVideo {
			return "video=" + device
		}
		return "audio=" + device
	default:
		return device
	}
}

// getPixelFormatName returns the FFmpeg pixel format name for common formats.
func getPixelFormatName(pixFmt PixelFormat) string {
	switch pixFmt {
	case PixelFormatYUV420P:
		return "yuv420p"
	case PixelFormatYUVJ420P:
		return "yuvj420p"
	case PixelFormatRGB24:
		return "rgb24"
	case PixelFormatBGR24:
		return "bgr24"
	case PixelFormatRGBA:
		return "rgba"
	case PixelFormatBGRA:
		return "bgra"
	case PixelFormatNV12:
		return "nv12"
	default:
		return ""
	}
}

// CaptureScreen captures the screen on supported platforms.
// This is a convenience function that sets up screen capture with appropriate defaults.
//
// Example (Linux X11):
//
//	decoder, err := ffgo.CaptureScreen(":0.0")
//
// Example (macOS):
//
//	decoder, err := ffgo.CaptureScreen("Capture screen 0")
//
// Note: Screen capture may require additional permissions on some platforms.
func CaptureScreen(display string) (*Decoder, error) {
	return CaptureScreenWithOptions(ScreenCaptureOptions{
		Display: display,
	})
}

// getScreenCaptureFormat returns the FFmpeg input format for screen capture.
func getScreenCaptureFormat() string {
	switch runtime.GOOS {
	case "linux":
		return "x11grab" // X11 screen capture
	case "darwin":
		return "avfoundation" // Screen capture via avfoundation
	case "windows":
		return "gdigrab" // GDI screen capture
	default:
		return ""
	}
}

// getDefaultDisplay returns the default display identifier for screen capture.
func getDefaultDisplay() string {
	switch runtime.GOOS {
	case "linux":
		return ":0.0" // Default X11 display
	case "darwin":
		return "Capture screen 0"
	case "windows":
		return "desktop"
	default:
		return ""
	}
}

// ScreenCaptureOptions configures screen capture behavior.
type ScreenCaptureOptions struct {
	// Display specifies the display to capture.
	// Linux: ":0.0" (X11 display)
	// macOS: "Capture screen 0"
	// Windows: "desktop" or window title
	Display string

	// OffsetX specifies the X offset for capture region (Linux/Windows).
	OffsetX int

	// OffsetY specifies the Y offset for capture region (Linux/Windows).
	OffsetY int

	// Width specifies the capture width. 0 means full screen.
	Width int

	// Height specifies the capture height. 0 means full screen.
	Height int

	// FrameRate specifies the capture frame rate.
	FrameRate Rational

	// DrawMouse indicates whether to draw the mouse cursor (Linux).
	DrawMouse bool

	// FollowMouse indicates whether to follow the mouse cursor (Linux).
	FollowMouse bool
}

// CaptureScreenWithOptions captures the screen with custom options.
func CaptureScreenWithOptions(opts ScreenCaptureOptions) (*Decoder, error) {
	if err := bindings.Load(); err != nil {
		return nil, err
	}
	if err := avdevice.RegisterAll(); err != nil {
		return nil, fmt.Errorf("ffgo: screen capture requires libavdevice: %w", err)
	}

	display := opts.Display
	if display == "" {
		display = getDefaultDisplay()
	}

	inputFormatName := getScreenCaptureFormat()
	if inputFormatName == "" {
		return nil, fmt.Errorf("ffgo: screen capture not supported on this platform")
	}

	inputFmt := avformat.FindInputFormat(inputFormatName)
	if inputFmt == nil {
		return nil, fmt.Errorf("ffgo: input format %s not found", inputFormatName)
	}

	// Build options dictionary
	var avDict avutil.Dictionary

	if opts.Width > 0 && opts.Height > 0 {
		if err := avutil.DictSet(&avDict, "video_size", fmt.Sprintf("%dx%d", opts.Width, opts.Height), 0); err != nil {
			if avDict != nil {
				avutil.DictFree(&avDict)
			}
			return nil, err
		}
	}

	if opts.FrameRate.Num > 0 && opts.FrameRate.Den > 0 {
		if err := avutil.DictSet(&avDict, "framerate", fmt.Sprintf("%d/%d", opts.FrameRate.Num, opts.FrameRate.Den), 0); err != nil {
			if avDict != nil {
				avutil.DictFree(&avDict)
			}
			return nil, err
		}
	}

	// Platform-specific options
	if runtime.GOOS == "linux" {
		// X11grab-specific options
		if opts.DrawMouse {
			if err := avutil.DictSet(&avDict, "draw_mouse", "1", 0); err != nil {
				if avDict != nil {
					avutil.DictFree(&avDict)
				}
				return nil, err
			}
		}
		if opts.FollowMouse {
			if err := avutil.DictSet(&avDict, "follow_mouse", "centered", 0); err != nil {
				if avDict != nil {
					avutil.DictFree(&avDict)
				}
				return nil, err
			}
		}

		// For x11grab, display format is ":display+x,y"
		if opts.OffsetX > 0 || opts.OffsetY > 0 {
			if !strings.Contains(display, "+") {
				display = fmt.Sprintf("%s+%d,%d", display, opts.OffsetX, opts.OffsetY)
			}
		}
	}

	// Create decoder struct
	d := &Decoder{
		videoStreamIdx: -1,
		audioStreamIdx: -1,
	}

	// Open the input with specific format
	if err := avformat.OpenInput(&d.formatCtx, display, inputFmt, &avDict); err != nil {
		if avDict != nil {
			avutil.DictFree(&avDict)
		}
		return nil, fmt.Errorf("ffgo: failed to open screen capture: %w", err)
	}

	// Free any remaining dictionary entries
	if avDict != nil {
		avutil.DictFree(&avDict)
	}

	// Find stream info
	if err := avformat.FindStreamInfo(d.formatCtx, nil); err != nil {
		avformat.CloseInput(&d.formatCtx)
		return nil, fmt.Errorf("ffgo: failed to find stream info: %w", err)
	}

	// Find best streams
	d.videoStreamIdx = int(avformat.FindBestStream(d.formatCtx, avutil.MediaTypeVideo, -1, -1, nil, 0))
	if d.videoStreamIdx >= 0 {
		d.videoInfo = d.getStreamInfo(d.videoStreamIdx)
	}

	d.audioStreamIdx = int(avformat.FindBestStream(d.formatCtx, avutil.MediaTypeAudio, -1, -1, nil, 0))
	if d.audioStreamIdx >= 0 {
		d.audioInfo = d.getStreamInfo(d.audioStreamIdx)
	}

	return d, nil
}
