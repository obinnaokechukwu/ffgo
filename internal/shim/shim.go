//go:build !ios && !android && (amd64 || arm64)

// Package shim provides bindings to the ffshim helper library.
// The shim is required for variadic functions and log callbacks.
package shim

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

var (
	libShim uintptr
	loaded  bool
	loadErr error
	loadMu  sync.Mutex

	// Function bindings
	shimLogSetCallback func(cb uintptr)
	shimLogSetLevel    func(level int32)
	shimLog            func(avcl unsafe.Pointer, level int32, msg string)
	shimNewChapter     func(ctx unsafe.Pointer, id int64, tbNum, tbDen int32, start, end int64, metadata unsafe.Pointer) unsafe.Pointer

	// Device enumeration helpers (libavdevice wrappers)
	shimAVDeviceListInputSources func(formatName, deviceName string, avdictOpts unsafe.Pointer, outCount *int32, outNames, outDescs *unsafe.Pointer) int32
	shimAVDeviceFreeStringArray  func(arr unsafe.Pointer, count int32)

	// AVFrame offset discovery helpers
	shimAVFrameColorOffsets func(outRange, outSpace, outPrimaries, outTransfer *int32) int32
)

// Load attempts to load the ffshim library.
// Returns nil if already loaded or if shim is not available.
// Shim is optional - logging will not work without it.
func Load() error {
	loadMu.Lock()
	defer loadMu.Unlock()

	if loaded {
		return nil
	}
	if loadErr != nil {
		return loadErr
	}

	path, err := findShimLibrary()
	if err != nil {
		// Shim is optional, so don't fail
		loadErr = err
		return nil
	}

	lib, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		loadErr = err
		return nil
	}

	libShim = lib
	registerBindings()
	loaded = true
	return nil
}

// IsLoaded returns true if the shim library was successfully loaded.
func IsLoaded() bool {
	loadMu.Lock()
	defer loadMu.Unlock()
	return loaded
}

// Lib returns the shim library handle.
func Lib() uintptr {
	loadMu.Lock()
	defer loadMu.Unlock()
	return libShim
}

func registerBindings() {
	if libShim == 0 {
		return
	}

	// The shim library can exist in partial form depending on how it was built.
	// Treat all symbols as optional and expose feature-level errors when missing.
	registerOptionalLibFunc(&shimLogSetCallback, libShim, "ffshim_log_set_callback")
	registerOptionalLibFunc(&shimLogSetLevel, libShim, "ffshim_log_set_level")
	registerOptionalLibFunc(&shimLog, libShim, "ffshim_log")
	registerOptionalLibFunc(&shimNewChapter, libShim, "ffshim_new_chapter")

	// Optional newer symbols (present in newer shim builds)
	registerOptionalLibFunc(&shimAVDeviceListInputSources, libShim, "ffshim_avdevice_list_input_sources")
	registerOptionalLibFunc(&shimAVDeviceFreeStringArray, libShim, "ffshim_avdevice_free_string_array")
	registerOptionalLibFunc(&shimAVFrameColorOffsets, libShim, "ffshim_avframe_color_offsets")
}

func registerOptionalLibFunc(fptr any, handle uintptr, name string) {
	defer func() {
		_ = recover() // purego.RegisterLibFunc panics if symbol is missing
	}()
	purego.RegisterLibFunc(fptr, handle, name)
}

// SetLogCallback sets the FFmpeg log callback via the shim.
// cb is a purego callback created with purego.NewCallback.
func SetLogCallback(cb uintptr) error {
	if !loaded {
		return errors.New("ffgo: shim not loaded, logging not available")
	}
	if shimLogSetCallback == nil {
		return errors.New("ffgo: shimLogSetCallback not available")
	}
	shimLogSetCallback(cb)
	return nil
}

// SetLogLevel sets the FFmpeg log level via the shim.
func SetLogLevel(level int32) error {
	if !loaded {
		return errors.New("ffgo: shim not loaded, logging not available")
	}
	if shimLogSetLevel == nil {
		return errors.New("ffgo: shimLogSetLevel not available")
	}
	shimLogSetLevel(level)
	return nil
}

// Log sends a pre-formatted message to FFmpeg's logger via the shim.
func Log(avcl unsafe.Pointer, level int32, msg string) error {
	if !loaded {
		return errors.New("ffgo: shim not loaded, logging not available")
	}
	if shimLog == nil {
		return errors.New("ffgo: shimLog not available")
	}
	shimLog(avcl, level, msg)
	return nil
}

// NewChapter creates a new chapter in the format context via the shim.
// Returns the AVChapter pointer or nil on failure.
func NewChapter(ctx unsafe.Pointer, id int64, tbNum, tbDen int32, start, end int64, metadata unsafe.Pointer) (unsafe.Pointer, error) {
	if !loaded {
		return nil, errors.New("ffgo: shim not loaded, chapter writing not available")
	}
	if shimNewChapter == nil {
		return nil, errors.New("ffgo: shimNewChapter not available")
	}
	ch := shimNewChapter(ctx, id, tbNum, tbDen, start, end, metadata)
	if ch == nil {
		return nil, errors.New("ffgo: failed to create chapter")
	}
	return ch, nil
}

// AVDeviceListInputSources lists available input devices for a given avdevice input format.
//
// Returns:
// - count: number of devices
// - names/descs: pointers to shim-allocated string arrays (char**). Caller MUST free both arrays
//   using AVDeviceFreeStringArray.
//
// This requires a shim build that links against libavdevice.
func AVDeviceListInputSources(formatName, deviceName string, avdictOpts unsafe.Pointer) (count int, names, descs unsafe.Pointer, err error) {
	if !loaded || shimAVDeviceListInputSources == nil {
		return 0, nil, nil, errors.New("ffgo: shim avdevice helpers not available")
	}
	var c int32
	var n unsafe.Pointer
	var d unsafe.Pointer
	ret := shimAVDeviceListInputSources(formatName, deviceName, avdictOpts, &c, &n, &d)
	if ret < 0 {
		return 0, nil, nil, errors.New("ffgo: ffshim_avdevice_list_input_sources failed")
	}
	return int(c), n, d, nil
}

// AVDeviceFreeStringArray frees a string array allocated by AVDeviceListInputSources.
func AVDeviceFreeStringArray(arr unsafe.Pointer, count int) {
	if !loaded || shimAVDeviceFreeStringArray == nil || arr == nil || count <= 0 {
		return
	}
	shimAVDeviceFreeStringArray(arr, int32(count))
}

// AVFrameColorOffsets returns AVFrame field offsets (bytes) for:
// - color_range
// - colorspace
// - color_primaries
// - color_trc
func AVFrameColorOffsets() (rangeOff, spaceOff, primariesOff, transferOff int32, err error) {
	if !loaded || shimAVFrameColorOffsets == nil {
		return 0, 0, 0, 0, errors.New("ffgo: shim AVFrame offset helpers not available")
	}
	var r, s, p, t int32
	ret := shimAVFrameColorOffsets(&r, &s, &p, &t)
	if ret < 0 {
		return 0, 0, 0, 0, errors.New("ffgo: ffshim_avframe_color_offsets failed")
	}
	return r, s, p, t, nil
}

// findShimLibrary looks for the shim library in standard locations.
func findShimLibrary() (string, error) {
	var names []string

	switch runtime.GOOS {
	case "linux", "freebsd", "openbsd", "netbsd":
		names = []string{"libffshim.so", "libffshim.so.1"}
	case "darwin":
		names = []string{"libffshim.dylib", "libffshim.1.dylib"}
	case "windows":
		names = []string{"ffshim.dll", "libffshim.dll"}
	default:
		return "", errors.New("unsupported platform")
	}

	// FFGO_SHIM_DIR override (highest priority)
	if dir := os.Getenv("FFGO_SHIM_DIR"); dir != "" {
		for _, name := range names {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
		return "", errors.New("ffshim library not found in FFGO_SHIM_DIR")
	}

	// Search paths
	var searchPaths []string

	// Check LD_LIBRARY_PATH / DYLD_LIBRARY_PATH first
	if runtime.GOOS == "darwin" {
		if p := os.Getenv("DYLD_LIBRARY_PATH"); p != "" {
			searchPaths = append(searchPaths, filepath.SplitList(p)...)
		}
	} else {
		if p := os.Getenv("LD_LIBRARY_PATH"); p != "" {
			searchPaths = append(searchPaths, filepath.SplitList(p)...)
		}
	}

	// Check PATH on Windows
	if runtime.GOOS == "windows" {
		if p := os.Getenv("PATH"); p != "" {
			searchPaths = append(searchPaths, filepath.SplitList(p)...)
		}
	}

	// Standard library paths
	searchPaths = append(searchPaths,
		"/usr/local/lib",
		"/usr/lib",
		"/lib",
		"/opt/ffmpeg/lib",
	)

	// Platform-specific paths
	switch runtime.GOOS {
	case "linux":
		searchPaths = append(searchPaths,
			"/usr/lib/x86_64-linux-gnu",
			"/usr/lib/aarch64-linux-gnu",
		)
	case "darwin":
		searchPaths = append(searchPaths,
			"/opt/homebrew/lib",
			"/usr/local/opt/ffmpeg/lib",
		)
	}

	// Executable directory
	if exe, err := os.Executable(); err == nil {
		searchPaths = append(searchPaths, filepath.Dir(exe))
	}

	// Module-local prebuilt shim (dev/test)
	// <module_root>/shim/prebuilt/<goos>-<goarch>/<filename>
	if _, file, _, ok := runtime.Caller(0); ok {
		// internal/shim/shim.go -> <module_root>
		moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
		prebuiltDir := filepath.Join(moduleRoot, "shim", "prebuilt", runtime.GOOS+"-"+runtime.GOARCH)
		searchPaths = append(searchPaths, prebuiltDir)
		// also allow <module_root>/shim/ (source tree)
		searchPaths = append(searchPaths, filepath.Join(moduleRoot, "shim"))
		// and <module_root>/ (repo root) for existing check-in binaries
		searchPaths = append(searchPaths, moduleRoot)
	}

	// Current working directory
	if cwd, err := os.Getwd(); err == nil {
		searchPaths = append(searchPaths, cwd)
	}

	// Try each name in each path
	for _, name := range names {
		// Try direct load first (uses system path resolution)
		if _, err := os.Stat(name); err == nil {
			return name, nil
		}

		for _, dir := range searchPaths {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}

	return "", errors.New("ffshim library not found")
}
