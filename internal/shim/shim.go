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

	purego.RegisterLibFunc(&shimLogSetCallback, libShim, "ffshim_log_set_callback")
	purego.RegisterLibFunc(&shimLogSetLevel, libShim, "ffshim_log_set_level")
	purego.RegisterLibFunc(&shimLog, libShim, "ffshim_log")
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
