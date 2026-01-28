//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"io"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/obinnaokechukwu/ffgo/avcodec"
	"github.com/obinnaokechukwu/ffgo/avformat"
	"github.com/obinnaokechukwu/ffgo/avutil"
	"github.com/obinnaokechukwu/ffgo/internal/bindings"
	"github.com/obinnaokechukwu/ffgo/internal/handles"
)

// IOCallbacks provides custom I/O operations for reading and writing media.
type IOCallbacks struct {
	// Read reads up to len(buf) bytes into buf.
	// Returns the number of bytes read and any error encountered.
	// At end of file, returns 0, io.EOF.
	Read func(buf []byte) (int, error)

	// Write writes len(buf) bytes from buf.
	// Returns the number of bytes written and any error encountered.
	Write func(buf []byte) (int, error)

	// Seek seeks to the given offset.
	// whence: 0 = SEEK_SET, 1 = SEEK_CUR, 2 = SEEK_END
	// Returns the new offset and any error encountered.
	Seek func(offset int64, whence int) (int64, error)
}

// CustomIOContext wraps an AVIOContext with custom callbacks.
type CustomIOContext struct {
	mu        sync.Mutex
	avioCtx   avformat.IOContext
	buffer    unsafe.Pointer // Allocated with av_malloc, owned by FFmpeg
	bufferGo  []byte         // Go slice view of buffer (for callbacks)
	callbacks *IOCallbacks
	handle    uintptr
	closed    bool
}

// Default buffer size for custom I/O (32KB)
const defaultIOBufferSize = 32 * 1024

// Pre-registered callbacks to avoid hitting purego's callback limit.
// These are registered once and reused across all CustomIOContext instances.
var (
	ioCallbacksOnce    sync.Once
	readCallbackPtr    uintptr
	writeCallbackPtr   uintptr
	seekCallbackPtr    uintptr
	ioCallbacksInitErr error
)

func initIOCallbacks() error {
	ioCallbacksOnce.Do(func() {
		// Read callback: int read_packet(void *opaque, uint8_t *buf, int buf_size)
		readCallbackPtr = purego.NewCallback(func(_ purego.CDecl, opaque unsafe.Pointer, buf *byte, bufSize int32) int32 {
			ctx := handles.Lookup(uintptr(opaque))
			if ctx == nil {
				return -1
			}
			ioCtx := ctx.(*CustomIOContext)
			if ioCtx.callbacks == nil || ioCtx.callbacks.Read == nil {
				return -1
			}

			// Create Go slice from C buffer
			goBuf := unsafe.Slice(buf, bufSize)

			n, err := ioCtx.callbacks.Read(goBuf)
			if err != nil {
				if err == io.EOF {
					if n > 0 {
						return int32(n)
					}
					return avutil.AVERROR_EOF
				}
				return -1
			}
			return int32(n)
		})

		// Write callback: int write_packet(void *opaque, uint8_t *buf, int buf_size)
		writeCallbackPtr = purego.NewCallback(func(_ purego.CDecl, opaque unsafe.Pointer, buf *byte, bufSize int32) int32 {
			ctx := handles.Lookup(uintptr(opaque))
			if ctx == nil {
				return -1
			}
			ioCtx := ctx.(*CustomIOContext)
			if ioCtx.callbacks == nil || ioCtx.callbacks.Write == nil {
				return -1
			}

			// Create Go slice from C buffer
			goBuf := unsafe.Slice(buf, bufSize)

			n, err := ioCtx.callbacks.Write(goBuf)
			if err != nil {
				return -1
			}
			return int32(n)
		})

		// Seek callback: int64_t seek(void *opaque, int64_t offset, int whence)
		seekCallbackPtr = purego.NewCallback(func(_ purego.CDecl, opaque unsafe.Pointer, offset int64, whence int32) int64 {
			ctx := handles.Lookup(uintptr(opaque))
			if ctx == nil {
				return -1
			}
			ioCtx := ctx.(*CustomIOContext)
			if ioCtx.callbacks == nil || ioCtx.callbacks.Seek == nil {
				// If no seek callback but whence is AVSEEK_SIZE, return -1 (unknown size)
				if whence == 0x10000 { // AVSEEK_SIZE
					return -1
				}
				return -1
			}

			// Handle AVSEEK_SIZE request
			if whence == 0x10000 { // AVSEEK_SIZE
				// Try to get size by seeking to end and back
				current, err := ioCtx.callbacks.Seek(0, io.SeekCurrent)
				if err != nil {
					return -1
				}
				end, err := ioCtx.callbacks.Seek(0, io.SeekEnd)
				if err != nil {
					return -1
				}
				_, err = ioCtx.callbacks.Seek(current, io.SeekStart)
				if err != nil {
					return -1
				}
				return end
			}

			newPos, err := ioCtx.callbacks.Seek(offset, int(whence))
			if err != nil {
				return -1
			}
			return newPos
		})
	})

	return ioCallbacksInitErr
}

// NewCustomIOContext creates a new custom I/O context with the given callbacks.
func NewCustomIOContext(callbacks *IOCallbacks, writable bool) (*CustomIOContext, error) {
	return NewCustomIOContextWithSize(callbacks, writable, defaultIOBufferSize)
}

// NewCustomIOContextWithSize creates a new custom I/O context with a specific buffer size.
func NewCustomIOContextWithSize(callbacks *IOCallbacks, writable bool, bufferSize int) (*CustomIOContext, error) {
	if callbacks == nil {
		return nil, errors.New("ffgo: callbacks cannot be nil")
	}
	if !writable && callbacks.Read == nil {
		return nil, errors.New("ffgo: read callback required for readable I/O context")
	}
	if writable && callbacks.Write == nil {
		return nil, errors.New("ffgo: write callback required for writable I/O context")
	}

	// Ensure FFmpeg is loaded
	if err := bindings.Load(); err != nil {
		return nil, err
	}

	// Initialize global callbacks
	if err := initIOCallbacks(); err != nil {
		return nil, err
	}

	// Allocate buffer with av_malloc (required by FFmpeg - it will free it)
	buffer := avutil.Malloc(uintptr(bufferSize))
	if buffer == nil {
		return nil, errors.New("ffgo: failed to allocate I/O buffer")
	}

	ctx := &CustomIOContext{
		buffer:    buffer,
		bufferGo:  unsafe.Slice((*byte)(buffer), bufferSize),
		callbacks: callbacks,
	}

	// Register handle for callback lookup
	ctx.handle = handles.Register(ctx)

	// Determine which callbacks to use
	var readCb, writeCb, seekCb uintptr
	if callbacks.Read != nil {
		readCb = readCallbackPtr
	}
	if callbacks.Write != nil {
		writeCb = writeCallbackPtr
	}
	if callbacks.Seek != nil {
		seekCb = seekCallbackPtr
	}

	// Create AVIOContext
	ctx.avioCtx = avformat.IOAllocContext(
		buffer,
		bufferSize,
		writable,
		unsafe.Pointer(ctx.handle),
		readCb,
		writeCb,
		seekCb,
	)

	if ctx.avioCtx == nil {
		avutil.Free(buffer)
		handles.Unregister(ctx.handle)
		return nil, errors.New("ffgo: failed to create AVIOContext")
	}

	return ctx, nil
}

// Close releases the I/O context.
// Note: avio_context_free also frees the buffer, so we don't free it manually.
func (c *CustomIOContext) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	// Free AVIOContext (this also frees the buffer)
	if c.avioCtx != nil {
		avformat.IOContextFree(&c.avioCtx)
	}

	// Clear buffer references (buffer is freed by IOContextFree)
	c.buffer = nil
	c.bufferGo = nil

	// Unregister handle
	if c.handle != 0 {
		handles.Unregister(c.handle)
		c.handle = 0
	}

	return nil
}

// AVIOContext returns the underlying AVIOContext pointer.
func (c *CustomIOContext) AVIOContext() avformat.IOContext {
	return c.avioCtx
}

// NewDecoderFromIO creates a decoder with custom I/O.
// format is the format hint (e.g., "mp4", "mkv", "avi") - can be empty for auto-detection.
func NewDecoderFromIO(callbacks *IOCallbacks, format string) (*Decoder, error) {
	return NewDecoderFromIOWithOptions(callbacks, &DecoderOptions{Format: format})
}

// NewDecoderFromIOWithOptions creates a decoder with custom I/O and DecoderOptions.
//
// It supports passing typed probing controls (probesize/analyzeduration/etc) and a format hint.
// The returned decoder owns the CustomIOContext and will close it on Decoder.Close().
func NewDecoderFromIOWithOptions(callbacks *IOCallbacks, opts *DecoderOptions) (*Decoder, error) {
	// Create custom I/O context
	ioCtx, err := NewCustomIOContext(callbacks, false)
	if err != nil {
		return nil, err
	}

	// Allocate format context
	formatCtx := avformat.AllocContext()
	if formatCtx == nil {
		ioCtx.Close()
		return nil, errors.New("ffgo: failed to allocate format context")
	}

	// Set custom I/O
	avformat.SetIOContext(formatCtx, ioCtx.AVIOContext())

	// Set CUSTOM_IO flag to tell FFmpeg we own the I/O context
	avformat.AddFlags(formatCtx, avformat.AVFMT_FLAG_CUSTOM_IO)

	// Optional format hint.
	var inputFmt avformat.InputFormat
	if opts != nil && opts.Format != "" {
		inputFmt = avformat.FindInputFormat(opts.Format)
		if inputFmt == nil {
			ioCtx.Close()
			avformat.FreeContext(formatCtx)
			return nil, errors.New("ffgo: input format not found")
		}
	}

	// Build AVDictionary from options.
	var avDict avutil.Dictionary
	for key, value := range buildDecoderAVOptions(opts) {
		if value == "" {
			continue
		}
		if err := avutil.DictSet(&avDict, key, value, 0); err != nil {
			if avDict != nil {
				avutil.DictFree(&avDict)
			}
			ioCtx.Close()
			avformat.FreeContext(formatCtx)
			return nil, err
		}
	}

	// Open input with custom I/O (pass empty string since we have custom I/O)
	if err := avformat.OpenInput(&formatCtx, "", inputFmt, &avDict); err != nil {
		if avDict != nil {
			avutil.DictFree(&avDict)
		}
		ioCtx.Close()
		avformat.FreeContext(formatCtx)
		return nil, err
	}

	// Free any remaining dictionary entries (FFmpeg may have consumed some)
	if avDict != nil {
		avutil.DictFree(&avDict)
	}

	// Find stream info
	if err := avformat.FindStreamInfo(formatCtx, nil); err != nil {
		avformat.CloseInput(&formatCtx)
		ioCtx.Close()
		return nil, err
	}

	d := &Decoder{
		formatCtx:      formatCtx,
		videoStreamIdx: -1,
		audioStreamIdx: -1,
	}

	// Ensure the custom I/O stays alive and is cleaned up.
	d.customIO = ioCtx

	// Find best video stream
	d.videoStreamIdx = int(avformat.FindBestStream(d.formatCtx, avutil.MediaTypeVideo, -1, -1, nil, 0))
	if d.videoStreamIdx >= 0 {
		d.videoInfo = d.getStreamInfo(d.videoStreamIdx)
	}

	// Find best audio stream
	d.audioStreamIdx = int(avformat.FindBestStream(d.formatCtx, avutil.MediaTypeAudio, -1, -1, nil, 0))
	if d.audioStreamIdx >= 0 {
		d.audioInfo = d.getStreamInfo(d.audioStreamIdx)
	}

	// Allocate packet and frame
	d.packet = avcodec.PacketAlloc()
	if d.packet == nil {
		d.Close()
		ioCtx.Close()
		return nil, errors.New("ffgo: failed to allocate packet")
	}

	d.frame = avutil.FrameAlloc()
	if d.frame == nil {
		d.Close()
		return nil, errors.New("ffgo: failed to allocate frame")
	}

	return d, nil
}

// NewDecoderFromReader creates a decoder that reads from an io.Reader.
// If r implements io.Seeker, seeking will be supported.
// format is the format hint (e.g., "mp4", "mkv") - can be empty for auto-detection.
func NewDecoderFromReader(r io.Reader, format string) (*Decoder, error) {
	if r == nil {
		return nil, errors.New("ffgo: reader cannot be nil")
	}

	callbacks := &IOCallbacks{
		Read: func(buf []byte) (int, error) {
			return r.Read(buf)
		},
	}

	// Check if reader supports seeking
	if seeker, ok := r.(io.Seeker); ok {
		callbacks.Seek = func(offset int64, whence int) (int64, error) {
			return seeker.Seek(offset, whence)
		}
	}

	return NewDecoderFromIO(callbacks, format)
}

// NewDecoderFromReaderWithOptions creates a decoder that reads from an io.Reader using DecoderOptions.
// If r implements io.Seeker, seeking will be supported.
func NewDecoderFromReaderWithOptions(r io.Reader, opts *DecoderOptions) (*Decoder, error) {
	if r == nil {
		return nil, errors.New("ffgo: reader cannot be nil")
	}

	callbacks := &IOCallbacks{
		Read: func(buf []byte) (int, error) {
			return r.Read(buf)
		},
	}

	if seeker, ok := r.(io.Seeker); ok {
		callbacks.Seek = func(offset int64, whence int) (int64, error) {
			return seeker.Seek(offset, whence)
		}
	}

	return NewDecoderFromIOWithOptions(callbacks, opts)
}

// NewEncoderToWriter creates an encoder that writes to an io.Writer.
// If w implements io.Seeker, seeking will be supported.
// format is the output format (e.g., "mp4", "mkv", "avi").
func NewEncoderToWriter(w io.Writer, format string, config EncoderConfig) (*Encoder, error) {
	if w == nil {
		return nil, errors.New("ffgo: writer cannot be nil")
	}

	callbacks := &IOCallbacks{
		Write: func(buf []byte) (int, error) {
			return w.Write(buf)
		},
	}

	// Check if writer supports seeking
	if seeker, ok := w.(io.Seeker); ok {
		callbacks.Seek = func(offset int64, whence int) (int64, error) {
			return seeker.Seek(offset, whence)
		}
	}

	return NewEncoderFromIO(callbacks, format, config)
}

// NewEncoderToWriterWithOptions creates an encoder that writes to an io.Writer
// using the EncoderOptions configuration.
// If w implements io.Seeker, seeking will be supported.
// format is the output format (e.g., "mp4", "mkv", "avi").
func NewEncoderToWriterWithOptions(w io.Writer, format string, opts *EncoderOptions) (*Encoder, error) {
	if opts == nil || opts.Video == nil {
		return nil, errors.New("ffgo: EncoderOptions.Video is required")
	}

	video := opts.Video

	// Convert VideoEncoderConfig to EncoderConfig
	cfg := EncoderConfig{
		Width:       video.Width,
		Height:      video.Height,
		PixelFormat: video.PixelFormat,
		CodecID:     video.Codec,
		BitRate:     video.Bitrate,
		GOPSize:     video.GOPSize,
		MaxBFrames:  video.MaxBFrames,
	}

	// Handle frame rate
	if video.FrameRate.Den > 0 {
		cfg.FrameRate = int(video.FrameRate.Num / video.FrameRate.Den)
	}
	if cfg.FrameRate <= 0 {
		cfg.FrameRate = 30
	}

	// Apply defaults
	if cfg.CodecID == CodecIDNone {
		cfg.CodecID = CodecIDH264
	}
	if cfg.BitRate <= 0 {
		cfg.BitRate = 2000000
	}
	if cfg.PixelFormat == PixelFormatNone {
		cfg.PixelFormat = PixelFormatYUV420P
	}

	return NewEncoderToWriter(w, format, cfg)
}

// NewEncoderFromIO creates an encoder with custom I/O.
// format is the output format (e.g., "mp4", "mkv", "avi").
func NewEncoderFromIO(callbacks *IOCallbacks, format string, config EncoderConfig) (*Encoder, error) {
	// Ensure FFmpeg is loaded
	if err := bindings.Load(); err != nil {
		return nil, err
	}

	// Create custom I/O context (writable)
	ioCtx, err := NewCustomIOContext(callbacks, true)
	if err != nil {
		return nil, err
	}

	// Allocate output context with format
	var formatCtx avformat.FormatContext
	if err := avformat.AllocOutputContext2(&formatCtx, nil, format, ""); err != nil {
		ioCtx.Close()
		return nil, err
	}

	if formatCtx == nil {
		ioCtx.Close()
		return nil, errors.New("ffgo: failed to allocate output context")
	}

	// Set custom I/O
	avformat.SetIOContext(formatCtx, ioCtx.AVIOContext())

	// Create a new stream in the output container
	stream := avformat.NewStream(formatCtx, nil)
	if stream == nil {
		avformat.FreeContext(formatCtx)
		ioCtx.Close()
		return nil, errors.New("ffgo: failed to create output stream")
	}

	// Set stream time base
	if config.FrameRate > 0 {
		avformat.SetStreamTimeBase(stream, 1, int32(config.FrameRate))
	} else {
		avformat.SetStreamTimeBase(stream, 1, 30) // Default to 30 fps
	}

	// Find encoder
	codec := avcodec.FindEncoder(config.CodecID)
	if codec == nil {
		avformat.FreeContext(formatCtx)
		ioCtx.Close()
		return nil, errors.New("ffgo: encoder not found")
	}

	// Allocate codec context
	codecCtx := avcodec.AllocContext3(codec)
	if codecCtx == nil {
		avformat.FreeContext(formatCtx)
		ioCtx.Close()
		return nil, errors.New("ffgo: failed to allocate codec context")
	}

	// Configure codec context
	avcodec.SetCtxWidth(codecCtx, int32(config.Width))
	avcodec.SetCtxHeight(codecCtx, int32(config.Height))
	avcodec.SetCtxPixFmt(codecCtx, int32(config.PixelFormat))
	avcodec.SetCtxBitRate(codecCtx, config.BitRate)

	if config.GOPSize > 0 {
		avcodec.SetCtxGopSize(codecCtx, int32(config.GOPSize))
	} else {
		avcodec.SetCtxGopSize(codecCtx, 12)
	}

	avcodec.SetCtxMaxBFrames(codecCtx, int32(config.MaxBFrames))

	if config.FrameRate > 0 {
		avcodec.SetCtxFramerate(codecCtx, int32(config.FrameRate), 1)
		avcodec.SetCtxTimeBase(codecCtx, 1, int32(config.FrameRate))
	} else {
		avcodec.SetCtxFramerate(codecCtx, 30, 1)
		avcodec.SetCtxTimeBase(codecCtx, 1, 30)
	}

	// Check if global header is needed
	if avformat.NeedsGlobalHeader(formatCtx) {
		flags := avcodec.GetCtxFlags(codecCtx)
		avcodec.SetCtxFlags(codecCtx, flags|avcodec.CodecFlagGlobalHeader)
	}

	// Open codec
	if err := avcodec.Open2(codecCtx, codec, nil); err != nil {
		avcodec.FreeContext(&codecCtx)
		avformat.FreeContext(formatCtx)
		ioCtx.Close()
		return nil, err
	}

	// Copy codec parameters to stream
	codecPar := avformat.GetStreamCodecPar(stream)
	if err := avcodec.ParametersFromContext(codecPar, codecCtx); err != nil {
		avcodec.FreeContext(&codecCtx)
		avformat.FreeContext(formatCtx)
		ioCtx.Close()
		return nil, err
	}

	// Write header
	if err := avformat.WriteHeader(formatCtx, nil); err != nil {
		avcodec.FreeContext(&codecCtx)
		avformat.FreeContext(formatCtx)
		ioCtx.Close()
		return nil, err
	}

	// Allocate packet
	packet := avcodec.PacketAlloc()
	if packet == nil {
		avcodec.FreeContext(&codecCtx)
		avformat.FreeContext(formatCtx)
		ioCtx.Close()
		return nil, errors.New("ffgo: failed to allocate packet")
	}

	// Determine frame rate for time base
	frameRate := config.FrameRate
	if frameRate <= 0 {
		frameRate = 30
	}

	return &Encoder{
		formatCtx:     formatCtx,
		codecCtx:      codecCtx,
		stream:        stream,
		packet:        packet,
		width:         config.Width,
		height:        config.Height,
		pixFmt:        config.PixelFormat,
		frameCount:    0,
		timeBaseNum:   1,
		timeBaseDen:   int32(frameRate),
		headerWritten: true, // Header was already written above
	}, nil
}
