//go:build !ios && !android && (amd64 || arm64)

// Package shim provides bindings to the ffshim helper library.
//
// The shim is a small C library that wraps FFmpeg functionality that purego
// cannot handle directly:
//   - Variadic functions (av_log)
//   - Log callbacks with va_list parameters
//   - AVRational operations on non-Darwin platforms
//
// The shim is OPTIONAL - core ffgo functionality (decode, encode, transcode)
// works without it. Only logging and some advanced features require the shim.
//
// To build the shim for your platform:
//
//	cd shim && ./build.sh
//
// Or use the Makefile:
//
//	cd shim && make
//
// Pre-built shims are available in releases for:
//   - Linux amd64/arm64 (libffshim.so)
//   - macOS amd64/arm64 (libffshim.dylib)
//   - Windows amd64 (ffshim.dll)
package shim

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// ErrShimNotLoaded is returned when shim functions are called but the shim is not available.
var ErrShimNotLoaded = errors.New("ffgo: shim library not loaded; logging and some features unavailable")

// ErrShimNotFound is returned when the shim library cannot be found.
var ErrShimNotFound = errors.New("ffgo: shim library not found")

var (
	libShim   uintptr
	loaded    bool
	loadErr   error
	loadMu    sync.Mutex
	shimPath  string // Path where shim was found (for diagnostics)
	searchErr string // Detailed search error message

	// Function bindings
	shimLogSetCallback func(cb uintptr)
	shimLogSetLevel    func(level int32)
	shimLog            func(avcl uintptr, level int32, msg string)
	shimNewChapter     func(ctx uintptr, id int64, tbNum, tbDen int32, start, end int64, metadata uintptr) uintptr

	// Device enumeration helpers (libavdevice wrappers)
	shimAVDeviceListInputSources func(formatName, deviceName string, avdictOpts uintptr, outCount *int32, outNames, outDescs *unsafe.Pointer) int32
	shimAVDeviceFreeStringArray  func(arr uintptr, count int32)

	// AVFrame offset discovery helpers
	shimAVFrameColorOffsets func(outRange, outSpace, outPrimaries, outTransfer *int32) int32
)

// Load attempts to load the ffshim library.
// Returns nil if already loaded or if shim is not available.
// Shim is optional - logging will not work without it.
//
// The shim is searched for in the following locations (in order):
//  1. FFGO_SHIM_DIR environment variable
//  2. LD_LIBRARY_PATH / DYLD_LIBRARY_PATH / PATH
//  3. Standard library paths (/usr/local/lib, /usr/lib, etc.)
//  4. Executable directory
//  5. Module's shim/prebuilt/<os>-<arch>/ directory
//  6. Module's shim/ directory
//  7. Current working directory
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
		// Shim is optional, so don't fail - but save detailed error for diagnostics
		loadErr = err
		searchErr = err.Error()
		return nil
	}

	lib, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		loadErr = fmt.Errorf("failed to load shim at %s: %w", path, err)
		searchErr = loadErr.Error()
		return nil
	}

	libShim = lib
	shimPath = path
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

// Path returns the path where the shim was loaded from, or empty string if not loaded.
func Path() string {
	loadMu.Lock()
	defer loadMu.Unlock()
	return shimPath
}

// LoadError returns detailed error information if the shim failed to load.
// Returns nil if shim loaded successfully or Load() hasn't been called.
func LoadError() error {
	loadMu.Lock()
	defer loadMu.Unlock()
	return loadErr
}

// SearchError returns a human-readable description of why the shim wasn't found.
// This is useful for diagnostics and user-facing error messages.
func SearchError() string {
	loadMu.Lock()
	defer loadMu.Unlock()
	if searchErr != "" {
		return searchErr
	}
	if loaded {
		return ""
	}
	return "shim search not yet performed"
}

// Status returns a human-readable status of the shim library.
// Useful for diagnostics and logging.
func Status() string {
	loadMu.Lock()
	defer loadMu.Unlock()

	if loaded {
		return fmt.Sprintf("loaded from %s", shimPath)
	}
	if loadErr != nil {
		return fmt.Sprintf("not loaded: %s", loadErr)
	}
	return "not loaded (Load() not called)"
}

// ExpectedLibraryName returns the expected shim library filename for the current platform.
func ExpectedLibraryName() string {
	switch runtime.GOOS {
	case "darwin":
		return "libffshim.dylib"
	case "windows":
		return "ffshim.dll"
	default:
		return "libffshim.so"
	}
}

// BuildInstructions returns platform-specific instructions for building the shim.
func BuildInstructions() string {
	switch runtime.GOOS {
	case "linux":
		return `To build the shim on Linux:
  1. Install FFmpeg development libraries:
     sudo apt install libavcodec-dev libavformat-dev libavutil-dev
  2. Build the shim:
     cd shim && make
  3. Install or set path:
     sudo make install
     # OR
     export LD_LIBRARY_PATH=$PWD/shim:$LD_LIBRARY_PATH`
	case "darwin":
		return `To build the shim on macOS:
  1. Install FFmpeg via Homebrew:
     brew install ffmpeg
  2. Build the shim:
     cd shim && make
  3. Install or set path:
     sudo make install
     # OR
     export DYLD_LIBRARY_PATH=$PWD/shim:$DYLD_LIBRARY_PATH`
	case "windows":
		return `To build the shim on Windows:
  1. Install MSYS2 and MinGW-w64
  2. Install FFmpeg: pacman -S mingw-w64-x86_64-ffmpeg
  3. Build the shim:
     cd shim && make
  4. Copy ffshim.dll to your application directory or PATH`
	default:
		return fmt.Sprintf("Platform %s/%s is not supported for shim building", runtime.GOOS, runtime.GOARCH)
	}
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
		return fmt.Errorf("%w: SetLogCallback requires shim; %s", ErrShimNotLoaded, BuildInstructions())
	}
	if shimLogSetCallback == nil {
		return errors.New("ffgo: shimLogSetCallback symbol not available in shim")
	}
	shimLogSetCallback(cb)
	return nil
}

// SetLogLevel sets the FFmpeg log level via the shim.
func SetLogLevel(level int32) error {
	if !loaded {
		return fmt.Errorf("%w: SetLogLevel requires shim; %s", ErrShimNotLoaded, BuildInstructions())
	}
	if shimLogSetLevel == nil {
		return errors.New("ffgo: shimLogSetLevel symbol not available in shim")
	}
	shimLogSetLevel(level)
	return nil
}

// Log sends a pre-formatted message to FFmpeg's logger via the shim.
func Log(avcl unsafe.Pointer, level int32, msg string) error {
	if !loaded {
		return fmt.Errorf("%w: Log requires shim; %s", ErrShimNotLoaded, BuildInstructions())
	}
	if shimLog == nil {
		return errors.New("ffgo: shimLog symbol not available in shim")
	}
	shimLog(uintptr(avcl), level, msg)
	return nil
}

// NewChapter creates a new chapter in the format context via the shim.
// Returns the AVChapter pointer or nil on failure.
func NewChapter(ctx unsafe.Pointer, id int64, tbNum, tbDen int32, start, end int64, metadata unsafe.Pointer) (unsafe.Pointer, error) {
	if !loaded {
		return nil, fmt.Errorf("%w: NewChapter requires shim", ErrShimNotLoaded)
	}
	if shimNewChapter == nil {
		return nil, errors.New("ffgo: shimNewChapter symbol not available in shim")
	}
	ch := unsafe.Pointer(shimNewChapter(uintptr(ctx), id, tbNum, tbDen, start, end, uintptr(metadata)))
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
// This requires a shim build that links against libavdevice (built with -DFFSHIM_HAVE_AVDEVICE=1).
func AVDeviceListInputSources(formatName, deviceName string, avdictOpts unsafe.Pointer) (count int, names, descs unsafe.Pointer, err error) {
	if !loaded {
		return 0, nil, nil, fmt.Errorf("%w: device listing requires shim", ErrShimNotLoaded)
	}
	if shimAVDeviceListInputSources == nil {
		return 0, nil, nil, errors.New("ffgo: shim was not built with libavdevice support (-DFFSHIM_HAVE_AVDEVICE=1)")
	}
	var c int32
	var n unsafe.Pointer
	var d unsafe.Pointer
	ret := shimAVDeviceListInputSources(formatName, deviceName, uintptr(avdictOpts), &c, &n, &d)
	if ret < 0 {
		return 0, nil, nil, fmt.Errorf("ffgo: avdevice_list_input_sources failed (error %d)", ret)
	}
	return int(c), n, d, nil
}

// AVDeviceFreeStringArray frees a string array allocated by AVDeviceListInputSources.
func AVDeviceFreeStringArray(arr unsafe.Pointer, count int) {
	if !loaded || shimAVDeviceFreeStringArray == nil || arr == nil || count <= 0 {
		return
	}
	shimAVDeviceFreeStringArray(uintptr(arr), int32(count))
}

// AVFrameColorOffsets returns AVFrame field offsets (bytes) for:
// - color_range
// - colorspace
// - color_primaries
// - color_trc
func AVFrameColorOffsets() (rangeOff, spaceOff, primariesOff, transferOff int32, err error) {
	if !loaded {
		return 0, 0, 0, 0, fmt.Errorf("%w: AVFrameColorOffsets requires shim", ErrShimNotLoaded)
	}
	if shimAVFrameColorOffsets == nil {
		return 0, 0, 0, 0, errors.New("ffgo: shimAVFrameColorOffsets symbol not available in shim")
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
		return "", fmt.Errorf("%w: unsupported platform %s/%s", ErrShimNotFound, runtime.GOOS, runtime.GOARCH)
	}

	// FFGO_SHIM_DIR override (highest priority)
	if dir := os.Getenv("FFGO_SHIM_DIR"); dir != "" {
		for _, name := range names {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
		return "", fmt.Errorf("%w: FFGO_SHIM_DIR=%s does not contain %s", ErrShimNotFound, dir, names[0])
	}

	// Build search paths list
	var searchPaths []string
	var searchedPaths []string // For error message

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
		if runtime.GOARCH == "amd64" {
			searchPaths = append(searchPaths, "/usr/lib/x86_64-linux-gnu")
		} else if runtime.GOARCH == "arm64" {
			searchPaths = append(searchPaths, "/usr/lib/aarch64-linux-gnu")
		}
	case "darwin":
		searchPaths = append(searchPaths,
			"/opt/homebrew/lib",
			"/usr/local/opt/ffmpeg/lib",
		)
	case "windows":
		searchPaths = append(searchPaths,
			"C:\\ffmpeg\\bin",
			"C:\\Program Files\\ffmpeg\\bin",
		)
	}

	// Executable directory
	if exe, err := os.Executable(); err == nil {
		searchPaths = append(searchPaths, filepath.Dir(exe))
	}

	// Module-local prebuilt shim (dev/test and releases)
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
			searchedPaths = append(searchedPaths, path)
		}
	}

	// Build detailed error message
	expectedName := names[0]
	return "", fmt.Errorf("%w: looked for %s in %d locations. Set FFGO_SHIM_DIR or build the shim: cd shim && make",
		ErrShimNotFound, expectedName, len(searchedPaths))
}
