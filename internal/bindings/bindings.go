//go:build !ios && !android && (amd64 || arm64)

// Package bindings handles loading FFmpeg shared libraries and registering
// function bindings using purego.
package bindings

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/obinnaokechukwu/ffgo/internal/platform"
)

// ErrNotLoaded is returned when FFmpeg functions are called before Load().
var ErrNotLoaded = errors.New("ffgo: FFmpeg libraries not loaded; call ffgo.Init() first")

// ErrLibraryNotFound is returned when a required FFmpeg library cannot be found.
var ErrLibraryNotFound = errors.New("ffgo: FFmpeg library not found")

// Library handles
var (
	libAVUtil   uintptr
	libAVCodec  uintptr
	libAVFormat uintptr
	libSWScale  uintptr
	libFFShim   uintptr

	loaded   bool
	loadOnce sync.Once
	loadErr  error
)

// Version function bindings
var (
	avutilVersion   func() uint32
	avcodecVersion  func() uint32
	avformatVersion func() uint32
	swscaleVersion  func() uint32
)

// IsLoaded returns true if FFmpeg libraries have been successfully loaded.
func IsLoaded() bool {
	return loaded
}

// Load loads FFmpeg libraries and registers all function bindings.
// It is safe to call multiple times; subsequent calls are no-ops.
// Returns an error if libraries cannot be found or loaded.
func Load() error {
	loadOnce.Do(func() {
		loadErr = doLoad()
		if loadErr == nil {
			loaded = true
		}
	})
	return loadErr
}

func doLoad() error {
	// Load libraries in dependency order (CRITICAL per design doc)
	// avutil must be first, then others that depend on it
	var err error

	// 1. Load avutil (no dependencies)
	libAVUtil, err = loadLibrary("avutil", []int{59, 58, 57, 56})
	if err != nil {
		return fmt.Errorf("loading libavutil: %w", err)
	}

	// 2. Load avcodec (depends on avutil)
	libAVCodec, err = loadLibrary("avcodec", []int{61, 60, 59, 58})
	if err != nil {
		return fmt.Errorf("loading libavcodec: %w", err)
	}

	// 3. Load avformat (depends on avcodec, avutil)
	libAVFormat, err = loadLibrary("avformat", []int{61, 60, 59, 58})
	if err != nil {
		return fmt.Errorf("loading libavformat: %w", err)
	}

	// 4. Load swscale (depends on avutil) - optional
	libSWScale, _ = loadLibrary("swscale", []int{8, 7, 6, 5})

	// 5. Load shim (optional - for logging and AVRational on non-Darwin)
	libFFShim, _ = loadLibrary("ffshim", []int{0})

	// Register version functions
	purego.RegisterLibFunc(&avutilVersion, libAVUtil, "avutil_version")
	purego.RegisterLibFunc(&avcodecVersion, libAVCodec, "avcodec_version")
	purego.RegisterLibFunc(&avformatVersion, libAVFormat, "avformat_version")

	if libSWScale != 0 {
		purego.RegisterLibFunc(&swscaleVersion, libSWScale, "swscale_version")
	}

	return nil
}

// loadLibrary attempts to load a library by trying versioned names.
func loadLibrary(name string, versions []int) (uintptr, error) {
	// Try each search path
	for _, searchPath := range LibrarySearchPaths() {
		// Try versioned names first (more specific)
		for _, ver := range versions {
			libName := platform.FormatLibraryName(name, ver)
			fullPath := filepath.Join(searchPath, libName)

			// Try to open
			lib, err := tryOpen(fullPath)
			if err == nil {
				return lib, nil
			}
		}

		// Try unversioned name
		libName := platform.FormatLibraryName(name, 0)
		fullPath := filepath.Join(searchPath, libName)
		lib, err := tryOpen(fullPath)
		if err == nil {
			return lib, nil
		}
	}

	// Try just the library name (let the system find it)
	for _, ver := range versions {
		libName := platform.FormatLibraryName(name, ver)
		lib, err := tryOpen(libName)
		if err == nil {
			return lib, nil
		}
	}

	// Try unversioned
	libName := platform.FormatLibraryName(name, 0)
	lib, err := tryOpen(libName)
	if err == nil {
		return lib, nil
	}

	return 0, fmt.Errorf("%w: %s", ErrLibraryNotFound, name)
}

// tryOpen attempts to open a library with RTLD_NOW | RTLD_GLOBAL.
// RTLD_GLOBAL is REQUIRED per design doc - FFmpeg libraries have cross-references.
func tryOpen(path string) (uintptr, error) {
	// Note: purego.RTLD_GLOBAL is critical for FFmpeg
	lib, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return 0, err
	}
	return lib, nil
}

// FindLibrary searches for a library and returns its full path.
// This is useful for diagnostics.
func FindLibrary(name string, versions []int) (string, error) {
	for _, searchPath := range LibrarySearchPaths() {
		for _, ver := range versions {
			libName := platform.FormatLibraryName(name, ver)
			fullPath := filepath.Join(searchPath, libName)
			if _, err := os.Stat(fullPath); err == nil {
				return fullPath, nil
			}
		}
		// Try unversioned
		libName := platform.FormatLibraryName(name, 0)
		fullPath := filepath.Join(searchPath, libName)
		if _, err := os.Stat(fullPath); err == nil {
			return fullPath, nil
		}
	}
	return "", fmt.Errorf("%w: %s", ErrLibraryNotFound, name)
}

// LibrarySearchPaths returns platform-specific library search paths.
func LibrarySearchPaths() []string {
	var paths []string

	switch runtime.GOOS {
	case "linux":
		// Check LD_LIBRARY_PATH first
		if ldPath := os.Getenv("LD_LIBRARY_PATH"); ldPath != "" {
			paths = append(paths, filepath.SplitList(ldPath)...)
		}
		// Standard paths
		paths = append(paths,
			"/usr/lib/x86_64-linux-gnu",
			"/usr/lib/aarch64-linux-gnu",
			"/usr/local/lib",
			"/usr/lib",
			"/lib/x86_64-linux-gnu",
			"/lib",
		)

	case "darwin":
		// Check DYLD_LIBRARY_PATH first
		if dyldPath := os.Getenv("DYLD_LIBRARY_PATH"); dyldPath != "" {
			paths = append(paths, filepath.SplitList(dyldPath)...)
		}
		// Homebrew paths
		paths = append(paths,
			"/opt/homebrew/lib",                     // Apple Silicon
			"/usr/local/lib",                        // Intel
			"/opt/homebrew/opt/ffmpeg/lib",          // Homebrew FFmpeg
			"/usr/local/opt/ffmpeg/lib",             // Homebrew FFmpeg (Intel)
			"/opt/homebrew/Cellar/ffmpeg/7.1_4/lib", // Specific versions
			"/opt/homebrew/Cellar/ffmpeg/6.1.1_6/lib",
		)

	case "windows":
		// Check PATH
		if winPath := os.Getenv("PATH"); winPath != "" {
			paths = append(paths, filepath.SplitList(winPath)...)
		}
		// Executable directory
		if exe, err := os.Executable(); err == nil {
			paths = append(paths, filepath.Dir(exe))
		}
		// Common FFmpeg locations
		paths = append(paths,
			"C:\\ffmpeg\\bin",
			"C:\\Program Files\\ffmpeg\\bin",
		)

	case "freebsd":
		if ldPath := os.Getenv("LD_LIBRARY_PATH"); ldPath != "" {
			paths = append(paths, filepath.SplitList(ldPath)...)
		}
		paths = append(paths,
			"/usr/local/lib",
			"/usr/lib",
		)
	}

	return paths
}

// AVUtilVersion returns the avutil library version.
// Returns 0 if libraries are not loaded.
func AVUtilVersion() uint32 {
	if !loaded || avutilVersion == nil {
		return 0
	}
	return avutilVersion()
}

// AVCodecVersion returns the avcodec library version.
// Returns 0 if libraries are not loaded.
func AVCodecVersion() uint32 {
	if !loaded || avcodecVersion == nil {
		return 0
	}
	return avcodecVersion()
}

// AVFormatVersion returns the avformat library version.
// Returns 0 if libraries are not loaded.
func AVFormatVersion() uint32 {
	if !loaded || avformatVersion == nil {
		return 0
	}
	return avformatVersion()
}

// SWScaleVersion returns the swscale library version.
// Returns 0 if libraries are not loaded or swscale is not available.
func SWScaleVersion() uint32 {
	if !loaded || swscaleVersion == nil {
		return 0
	}
	return swscaleVersion()
}

// LibAVUtil returns the avutil library handle.
func LibAVUtil() uintptr {
	return libAVUtil
}

// LibAVCodec returns the avcodec library handle.
func LibAVCodec() uintptr {
	return libAVCodec
}

// LibAVFormat returns the avformat library handle.
func LibAVFormat() uintptr {
	return libAVFormat
}

// LibSWScale returns the swscale library handle.
func LibSWScale() uintptr {
	return libSWScale
}

// LibFFShim returns the ffshim library handle.
func LibFFShim() uintptr {
	return libFFShim
}

// HasSWScale returns true if swscale library is available.
func HasSWScale() bool {
	return libSWScale != 0
}

// HasFFShim returns true if the ffshim library is available.
func HasFFShim() bool {
	return libFFShim != 0
}

// LoadLibrary loads a library by name, trying the specified versions.
// This is exported for use by optional packages like swresample and avfilter.
func LoadLibrary(name string, versions []int) (uintptr, error) {
	// Ensure core libraries are loaded first
	if err := Load(); err != nil {
		return 0, err
	}
	return loadLibrary(name, versions)
}
