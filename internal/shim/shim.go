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

	// AVCodecParameters field helpers (optional)
	shimCodecParWidth      func(par uintptr) int32
	shimCodecParHeight     func(par uintptr) int32
	shimCodecParFormat     func(par uintptr) int32
	shimCodecParSampleRate func(par uintptr) int32
	shimCodecParChannels   func(par uintptr) int32

	// AVCodecContext field helpers (optional)
	shimCodecCtxWidth        func(ctx uintptr) int32
	shimCodecCtxSetWidth     func(ctx uintptr, width int32)
	shimCodecCtxHeight       func(ctx uintptr) int32
	shimCodecCtxSetHeight    func(ctx uintptr, height int32)
	shimCodecCtxPixFmt       func(ctx uintptr) int32
	shimCodecCtxSetPixFmt    func(ctx uintptr, pixFmt int32)
	shimCodecCtxSampleFmt    func(ctx uintptr) int32
	shimCodecCtxSetSampleFmt func(ctx uintptr, sampleFmt int32)
	shimCodecCtxTimeBase     func(ctx uintptr, outNum, outDen *int32)
	shimCodecCtxSetTimeBase  func(ctx uintptr, num, den int32)
	shimCodecCtxFramerate    func(ctx uintptr, outNum, outDen *int32)
	shimCodecCtxSetFramerate func(ctx uintptr, num, den int32)
	shimCodecCtxSetChLayout  func(ctx uintptr, nbChannels int32)

	// AVFormatContext / chapter / program helpers (optional)
	shimFormatCtxDuration    func(ctx uintptr) int64
	shimFormatCtxBitRate     func(ctx uintptr) int64
	shimFormatCtxNbChapters  func(ctx uintptr) uint32
	shimFormatCtxChapter     func(ctx uintptr, index int32) uintptr
	shimFormatCtxNbPrograms  func(ctx uintptr) uint32
	shimFormatCtxProgram     func(ctx uintptr, index int32) uintptr

	shimChapterID       func(ch uintptr) int64
	shimChapterTimeBase func(ch uintptr, outNum, outDen *int32)
	shimChapterStart    func(ch uintptr) int64
	shimChapterEnd      func(ch uintptr) int64
	shimChapterMetadata func(ch uintptr) uintptr

	shimProgramID             func(p uintptr) int32
	shimProgramNbStreamIdx    func(p uintptr) uint32
	shimProgramStreamIndexPtr func(p uintptr) uintptr
	shimProgramMetadata       func(p uintptr) uintptr
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

	// AVCodecParameters field helpers (optional)
	registerOptionalLibFunc(&shimCodecParWidth, libShim, "ffshim_codecpar_width")
	registerOptionalLibFunc(&shimCodecParHeight, libShim, "ffshim_codecpar_height")
	registerOptionalLibFunc(&shimCodecParFormat, libShim, "ffshim_codecpar_format")
	registerOptionalLibFunc(&shimCodecParSampleRate, libShim, "ffshim_codecpar_sample_rate")
	registerOptionalLibFunc(&shimCodecParChannels, libShim, "ffshim_codecpar_channels")

	// AVCodecContext field helpers (optional)
	registerOptionalLibFunc(&shimCodecCtxWidth, libShim, "ffshim_codecctx_width")
	registerOptionalLibFunc(&shimCodecCtxSetWidth, libShim, "ffshim_codecctx_set_width")
	registerOptionalLibFunc(&shimCodecCtxHeight, libShim, "ffshim_codecctx_height")
	registerOptionalLibFunc(&shimCodecCtxSetHeight, libShim, "ffshim_codecctx_set_height")
	registerOptionalLibFunc(&shimCodecCtxPixFmt, libShim, "ffshim_codecctx_pix_fmt")
	registerOptionalLibFunc(&shimCodecCtxSetPixFmt, libShim, "ffshim_codecctx_set_pix_fmt")
	registerOptionalLibFunc(&shimCodecCtxSampleFmt, libShim, "ffshim_codecctx_sample_fmt")
	registerOptionalLibFunc(&shimCodecCtxSetSampleFmt, libShim, "ffshim_codecctx_set_sample_fmt")
	registerOptionalLibFunc(&shimCodecCtxTimeBase, libShim, "ffshim_codecctx_time_base")
	registerOptionalLibFunc(&shimCodecCtxSetTimeBase, libShim, "ffshim_codecctx_set_time_base")
	registerOptionalLibFunc(&shimCodecCtxFramerate, libShim, "ffshim_codecctx_framerate")
	registerOptionalLibFunc(&shimCodecCtxSetFramerate, libShim, "ffshim_codecctx_set_framerate")
	registerOptionalLibFunc(&shimCodecCtxSetChLayout, libShim, "ffshim_codecctx_set_ch_layout_default")

	// AVFormatContext / chapter / program helpers (optional)
	registerOptionalLibFunc(&shimFormatCtxDuration, libShim, "ffshim_formatctx_duration")
	registerOptionalLibFunc(&shimFormatCtxBitRate, libShim, "ffshim_formatctx_bit_rate")
	registerOptionalLibFunc(&shimFormatCtxNbChapters, libShim, "ffshim_formatctx_nb_chapters")
	registerOptionalLibFunc(&shimFormatCtxChapter, libShim, "ffshim_formatctx_chapter")
	registerOptionalLibFunc(&shimFormatCtxNbPrograms, libShim, "ffshim_formatctx_nb_programs")
	registerOptionalLibFunc(&shimFormatCtxProgram, libShim, "ffshim_formatctx_program")

	registerOptionalLibFunc(&shimChapterID, libShim, "ffshim_chapter_id")
	registerOptionalLibFunc(&shimChapterTimeBase, libShim, "ffshim_chapter_time_base")
	registerOptionalLibFunc(&shimChapterStart, libShim, "ffshim_chapter_start")
	registerOptionalLibFunc(&shimChapterEnd, libShim, "ffshim_chapter_end")
	registerOptionalLibFunc(&shimChapterMetadata, libShim, "ffshim_chapter_metadata")

	registerOptionalLibFunc(&shimProgramID, libShim, "ffshim_program_id")
	registerOptionalLibFunc(&shimProgramNbStreamIdx, libShim, "ffshim_program_nb_stream_indexes")
	registerOptionalLibFunc(&shimProgramStreamIndexPtr, libShim, "ffshim_program_stream_index")
	registerOptionalLibFunc(&shimProgramMetadata, libShim, "ffshim_program_metadata")
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
//   - count: number of devices
//   - names/descs: pointers to shim-allocated string arrays (char**). Caller MUST free both arrays
//     using AVDeviceFreeStringArray.
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

func CodecParWidth(par unsafe.Pointer) (int32, error) {
	if par == nil {
		return 0, nil
	}
	if !loaded || shimCodecParWidth == nil {
		return 0, ErrShimNotLoaded
	}
	return shimCodecParWidth(uintptr(par)), nil
}

func CodecParHeight(par unsafe.Pointer) (int32, error) {
	if par == nil {
		return 0, nil
	}
	if !loaded || shimCodecParHeight == nil {
		return 0, ErrShimNotLoaded
	}
	return shimCodecParHeight(uintptr(par)), nil
}

func CodecParFormat(par unsafe.Pointer) (int32, error) {
	if par == nil {
		return -1, nil
	}
	if !loaded || shimCodecParFormat == nil {
		return 0, ErrShimNotLoaded
	}
	return shimCodecParFormat(uintptr(par)), nil
}

func CodecParSampleRate(par unsafe.Pointer) (int32, error) {
	if par == nil {
		return 0, nil
	}
	if !loaded || shimCodecParSampleRate == nil {
		return 0, ErrShimNotLoaded
	}
	return shimCodecParSampleRate(uintptr(par)), nil
}

func CodecParChannels(par unsafe.Pointer) (int32, error) {
	if par == nil {
		return 0, nil
	}
	if !loaded || shimCodecParChannels == nil {
		return 0, ErrShimNotLoaded
	}
	return shimCodecParChannels(uintptr(par)), nil
}

func CodecCtxWidth(ctx unsafe.Pointer) (int32, error) {
	if ctx == nil {
		return 0, nil
	}
	if !loaded || shimCodecCtxWidth == nil {
		return 0, ErrShimNotLoaded
	}
	return shimCodecCtxWidth(uintptr(ctx)), nil
}

func CodecCtxSetWidth(ctx unsafe.Pointer, width int32) error {
	if ctx == nil {
		return nil
	}
	if !loaded || shimCodecCtxSetWidth == nil {
		return ErrShimNotLoaded
	}
	shimCodecCtxSetWidth(uintptr(ctx), width)
	return nil
}

func CodecCtxHeight(ctx unsafe.Pointer) (int32, error) {
	if ctx == nil {
		return 0, nil
	}
	if !loaded || shimCodecCtxHeight == nil {
		return 0, ErrShimNotLoaded
	}
	return shimCodecCtxHeight(uintptr(ctx)), nil
}

func CodecCtxSetHeight(ctx unsafe.Pointer, height int32) error {
	if ctx == nil {
		return nil
	}
	if !loaded || shimCodecCtxSetHeight == nil {
		return ErrShimNotLoaded
	}
	shimCodecCtxSetHeight(uintptr(ctx), height)
	return nil
}

func CodecCtxPixFmt(ctx unsafe.Pointer) (int32, error) {
	if ctx == nil {
		return -1, nil
	}
	if !loaded || shimCodecCtxPixFmt == nil {
		return 0, ErrShimNotLoaded
	}
	return shimCodecCtxPixFmt(uintptr(ctx)), nil
}

func CodecCtxSetPixFmt(ctx unsafe.Pointer, pixFmt int32) error {
	if ctx == nil {
		return nil
	}
	if !loaded || shimCodecCtxSetPixFmt == nil {
		return ErrShimNotLoaded
	}
	shimCodecCtxSetPixFmt(uintptr(ctx), pixFmt)
	return nil
}

func CodecCtxSampleFmt(ctx unsafe.Pointer) (int32, error) {
	if ctx == nil {
		return -1, nil
	}
	if !loaded || shimCodecCtxSampleFmt == nil {
		return 0, ErrShimNotLoaded
	}
	return shimCodecCtxSampleFmt(uintptr(ctx)), nil
}

func CodecCtxSetSampleFmt(ctx unsafe.Pointer, sampleFmt int32) error {
	if ctx == nil {
		return nil
	}
	if !loaded || shimCodecCtxSetSampleFmt == nil {
		return ErrShimNotLoaded
	}
	shimCodecCtxSetSampleFmt(uintptr(ctx), sampleFmt)
	return nil
}

func CodecCtxTimeBase(ctx unsafe.Pointer) (num, den int32, err error) {
	if ctx == nil {
		return 0, 0, nil
	}
	if !loaded || shimCodecCtxTimeBase == nil {
		return 0, 0, ErrShimNotLoaded
	}
	shimCodecCtxTimeBase(uintptr(ctx), &num, &den)
	return num, den, nil
}

func CodecCtxSetTimeBase(ctx unsafe.Pointer, num, den int32) error {
	if ctx == nil {
		return nil
	}
	if !loaded || shimCodecCtxSetTimeBase == nil {
		return ErrShimNotLoaded
	}
	shimCodecCtxSetTimeBase(uintptr(ctx), num, den)
	return nil
}

func CodecCtxSetChLayoutDefault(ctx unsafe.Pointer, nbChannels int32) error {
	if ctx == nil {
		return nil
	}
	if !loaded || shimCodecCtxSetChLayout == nil {
		return ErrShimNotLoaded
	}
	shimCodecCtxSetChLayout(uintptr(ctx), nbChannels)
	return nil
}

func CodecCtxFramerate(ctx unsafe.Pointer) (num, den int32, err error) {
	if ctx == nil {
		return 0, 0, nil
	}
	if !loaded || shimCodecCtxFramerate == nil {
		return 0, 0, ErrShimNotLoaded
	}
	shimCodecCtxFramerate(uintptr(ctx), &num, &den)
	return num, den, nil
}

func CodecCtxSetFramerate(ctx unsafe.Pointer, num, den int32) error {
	if ctx == nil {
		return nil
	}
	if !loaded || shimCodecCtxSetFramerate == nil {
		return ErrShimNotLoaded
	}
	shimCodecCtxSetFramerate(uintptr(ctx), num, den)
	return nil
}

func FormatCtxDuration(ctx unsafe.Pointer) (int64, error) {
	if ctx == nil {
		return 0, nil
	}
	if !loaded || shimFormatCtxDuration == nil {
		return 0, ErrShimNotLoaded
	}
	return shimFormatCtxDuration(uintptr(ctx)), nil
}

func FormatCtxBitRate(ctx unsafe.Pointer) (int64, error) {
	if ctx == nil {
		return 0, nil
	}
	if !loaded || shimFormatCtxBitRate == nil {
		return 0, ErrShimNotLoaded
	}
	return shimFormatCtxBitRate(uintptr(ctx)), nil
}

func FormatCtxNbChapters(ctx unsafe.Pointer) (int, error) {
	if ctx == nil {
		return 0, nil
	}
	if !loaded || shimFormatCtxNbChapters == nil {
		return 0, ErrShimNotLoaded
	}
	return int(shimFormatCtxNbChapters(uintptr(ctx))), nil
}

func FormatCtxChapter(ctx unsafe.Pointer, index int) (unsafe.Pointer, error) {
	if ctx == nil {
		return nil, nil
	}
	if !loaded || shimFormatCtxChapter == nil {
		return nil, ErrShimNotLoaded
	}
	return unsafe.Pointer(shimFormatCtxChapter(uintptr(ctx), int32(index))), nil
}

func FormatCtxNbPrograms(ctx unsafe.Pointer) (int, error) {
	if ctx == nil {
		return 0, nil
	}
	if !loaded || shimFormatCtxNbPrograms == nil {
		return 0, ErrShimNotLoaded
	}
	return int(shimFormatCtxNbPrograms(uintptr(ctx))), nil
}

func FormatCtxProgram(ctx unsafe.Pointer, index int) (unsafe.Pointer, error) {
	if ctx == nil {
		return nil, nil
	}
	if !loaded || shimFormatCtxProgram == nil {
		return nil, ErrShimNotLoaded
	}
	return unsafe.Pointer(shimFormatCtxProgram(uintptr(ctx), int32(index))), nil
}

func ChapterID(ch unsafe.Pointer) (int64, error) {
	if ch == nil {
		return 0, nil
	}
	if !loaded || shimChapterID == nil {
		return 0, ErrShimNotLoaded
	}
	return shimChapterID(uintptr(ch)), nil
}

func ChapterTimeBase(ch unsafe.Pointer) (num, den int32, err error) {
	if ch == nil {
		return 0, 1, nil
	}
	if !loaded || shimChapterTimeBase == nil {
		return 0, 0, ErrShimNotLoaded
	}
	shimChapterTimeBase(uintptr(ch), &num, &den)
	return num, den, nil
}

func ChapterStart(ch unsafe.Pointer) (int64, error) {
	if ch == nil {
		return 0, nil
	}
	if !loaded || shimChapterStart == nil {
		return 0, ErrShimNotLoaded
	}
	return shimChapterStart(uintptr(ch)), nil
}

func ChapterEnd(ch unsafe.Pointer) (int64, error) {
	if ch == nil {
		return 0, nil
	}
	if !loaded || shimChapterEnd == nil {
		return 0, ErrShimNotLoaded
	}
	return shimChapterEnd(uintptr(ch)), nil
}

func ChapterMetadata(ch unsafe.Pointer) (unsafe.Pointer, error) {
	if ch == nil {
		return nil, nil
	}
	if !loaded || shimChapterMetadata == nil {
		return nil, ErrShimNotLoaded
	}
	return unsafe.Pointer(shimChapterMetadata(uintptr(ch))), nil
}

func ProgramID(p unsafe.Pointer) (int32, error) {
	if p == nil {
		return 0, nil
	}
	if !loaded || shimProgramID == nil {
		return 0, ErrShimNotLoaded
	}
	return shimProgramID(uintptr(p)), nil
}

func ProgramNbStreamIndexes(p unsafe.Pointer) (int, error) {
	if p == nil {
		return 0, nil
	}
	if !loaded || shimProgramNbStreamIdx == nil {
		return 0, ErrShimNotLoaded
	}
	return int(shimProgramNbStreamIdx(uintptr(p))), nil
}

func ProgramStreamIndexPtr(p unsafe.Pointer) (unsafe.Pointer, error) {
	if p == nil {
		return nil, nil
	}
	if !loaded || shimProgramStreamIndexPtr == nil {
		return nil, ErrShimNotLoaded
	}
	return unsafe.Pointer(shimProgramStreamIndexPtr(uintptr(p))), nil
}

func ProgramMetadata(p unsafe.Pointer) (unsafe.Pointer, error) {
	if p == nil {
		return nil, nil
	}
	if !loaded || shimProgramMetadata == nil {
		return nil, ErrShimNotLoaded
	}
	return unsafe.Pointer(shimProgramMetadata(uintptr(p))), nil
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
