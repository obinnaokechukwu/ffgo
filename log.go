//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/obinnaokechukwu/ffgo/internal/shim"
)

// LogLevel represents FFmpeg log levels.
type LogLevel int32

// Log level constants matching FFmpeg's AV_LOG_* values.
const (
	LogQuiet   LogLevel = -8  // Print no output
	LogPanic   LogLevel = 0   // Something went really wrong, crash
	LogFatal   LogLevel = 8   // Something went wrong, exit now
	LogError   LogLevel = 16  // Something went wrong, recovery possible
	LogWarning LogLevel = 24  // Something unexpected but recovery possible
	LogInfo    LogLevel = 32  // Standard information
	LogVerbose LogLevel = 40  // Detailed information
	LogDebug   LogLevel = 48  // Stuff for debugging
	LogTrace   LogLevel = 56  // Extremely verbose debugging
)

// String returns the string representation of the log level.
func (l LogLevel) String() string {
	switch {
	case l <= LogQuiet:
		return "quiet"
	case l <= LogPanic:
		return "panic"
	case l <= LogFatal:
		return "fatal"
	case l <= LogError:
		return "error"
	case l <= LogWarning:
		return "warning"
	case l <= LogInfo:
		return "info"
	case l <= LogVerbose:
		return "verbose"
	case l <= LogDebug:
		return "debug"
	default:
		return "trace"
	}
}

// LogCallback is called for each FFmpeg log message.
// level is the log level, message is the formatted message.
type LogCallback func(level LogLevel, message string)

var (
	logCallbackMu sync.Mutex
	logCallback   LogCallback
	logCBHandle   uintptr
)

// SetLogLevel sets the FFmpeg log level.
// This requires the ffshim library to be available.
// Returns an error if the shim is not loaded.
func SetLogLevel(level LogLevel) error {
	if err := shim.Load(); err != nil {
		return err
	}
	return shim.SetLogLevel(int32(level))
}

// SetLogCallback sets a custom log handler for FFmpeg messages.
// Pass nil to restore the default logging behavior.
// This requires the ffshim library to be available.
func SetLogCallback(cb LogCallback) error {
	if err := shim.Load(); err != nil {
		return err
	}

	logCallbackMu.Lock()
	defer logCallbackMu.Unlock()

	if cb == nil {
		// Restore default callback
		logCallback = nil
		return shim.SetLogCallback(0)
	}

	logCallback = cb

	// Create a purego callback if we haven't yet
	if logCBHandle == 0 {
		logCBHandle = purego.NewCallback(logCallbackTrampoline)
	}

	return shim.SetLogCallback(logCBHandle)
}

// logCallbackTrampoline is called by the shim and forwards to the Go callback.
// Signature: void (*)(void *avcl, int level, const char *msg)
func logCallbackTrampoline(_ purego.CDecl, _ unsafe.Pointer, level int32, msg *byte) {
	logCallbackMu.Lock()
	cb := logCallback
	logCallbackMu.Unlock()

	if cb == nil {
		return
	}

	// Convert C string to Go string
	goMsg := ""
	if msg != nil {
		// Find the length
		ptr := unsafe.Pointer(msg)
		for i := 0; ; i++ {
			b := *(*byte)(unsafe.Pointer(uintptr(ptr) + uintptr(i)))
			if b == 0 {
				goMsg = string(unsafe.Slice(msg, i))
				break
			}
			if i > 4096 { // Safety limit
				goMsg = string(unsafe.Slice(msg, i))
				break
			}
		}
	}

	cb(LogLevel(level), goMsg)
}

// IsLoggingAvailable returns true if logging functionality is available.
// Logging requires the ffshim helper library to be installed.
func IsLoggingAvailable() bool {
	if err := shim.Load(); err != nil {
		return false
	}
	return shim.IsLoaded()
}
