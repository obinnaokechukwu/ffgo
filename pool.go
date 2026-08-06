//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"errors"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/obinnaokechukwu/ffgo/avutil"
	"github.com/obinnaokechukwu/ffgo/internal/handles"
)

// FramePool reuses AVFrame allocations to reduce GC/FFmpeg allocation churn.
//
// Frames returned from Get() are OWNED by the caller and must be returned via Put().
type FramePool struct {
	mu       sync.Mutex
	idle     []avutil.Frame
	closed   bool
	inUse    int
	maxInUse int
}

// NewFramePool creates a new pool. If maxInUse <= 0, the pool is unbounded.
func NewFramePool(maxInUse int) *FramePool {
	return &FramePool{maxInUse: maxInUse}
}

// Get returns an owned frame from the pool.
func (p *FramePool) Get() (Frame, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return Frame{}, errors.New("ffgo: frame pool is closed")
	}
	if p.maxInUse > 0 && p.inUse >= p.maxInUse {
		return Frame{}, errors.New("ffgo: frame pool exhausted")
	}

	var fr avutil.Frame
	n := len(p.idle)
	if n > 0 {
		fr = p.idle[n-1]
		p.idle = p.idle[:n-1]
	} else {
		fr = avutil.FrameAlloc()
		if fr == nil {
			return Frame{}, ErrOutOfMemory
		}
	}

	avutil.FrameUnref(fr)
	p.inUse++
	return Frame{ptr: fr, owned: true}, nil
}

// Put returns an owned frame to the pool and clears the caller's reference.
func (p *FramePool) Put(f *Frame) error {
	if p == nil {
		return nil
	}
	if f == nil || f.ptr == nil {
		return nil
	}
	if !f.owned {
		return errors.New("ffgo: cannot put borrowed frame into pool")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		// Pool is closed: free the frame.
		avutil.FrameFree(&f.ptr)
		f.ptr = nil
		f.owned = false
		return nil
	}

	avutil.FrameUnref(f.ptr)
	p.idle = append(p.idle, f.ptr)
	p.inUse--

	f.ptr = nil
	f.owned = false
	return nil
}

// Close releases all idle frames in the pool. Frames still in use are not affected.
func (p *FramePool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true
	for i := range p.idle {
		fr := p.idle[i]
		if fr != nil {
			avutil.FrameFree(&fr)
		}
	}
	p.idle = nil
	return nil
}

// WrappedBufferUsage reports memory currently pinned by Frame.WrapBuffer.
type WrappedBufferUsage struct {
	PinnedBuffers int
	PinnedBytes   int64
}

var (
	wrapOnce        sync.Once
	wrapFreeCBPtr   uintptr
	wrapLimitBytes  atomic.Int64
	wrapPinnedBytes atomic.Int64
	wrapPinnedCount atomic.Int64
	wrapMu          sync.Mutex
)

// SetWrappedBufferMemoryLimit sets a best-effort limit for total bytes pinned by Frame.WrapBuffer.
// A limit <= 0 disables enforcement.
func SetWrappedBufferMemoryLimit(bytes int64) {
	wrapLimitBytes.Store(bytes)
}

// WrappedBufferMemoryUsage returns the current pinned buffer count/bytes.
func WrappedBufferMemoryUsage() WrappedBufferUsage {
	return WrappedBufferUsage{
		PinnedBuffers: int(wrapPinnedCount.Load()),
		PinnedBytes:   wrapPinnedBytes.Load(),
	}
}

func initWrapCallback() {
	wrapOnce.Do(func() {
		wrapFreeCBPtr = purego.NewCallback(func(_ purego.CDecl, opaque uintptr, _ *byte) {
			h := opaque
			v := handles.Lookup(h)
			if v == nil {
				return
			}
			ent := v.(wrappedBufferHold)
			handles.Unregister(h)
			wrapPinnedBytes.Add(-ent.size)
			wrapPinnedCount.Add(-1)
		})
	})
}

type wrappedBufferHold struct {
	data []byte
	size int64
}

// WrapBuffer wraps an existing buffer as a video frame without copying.
//
// The buffer is reference-counted by FFmpeg via AVBufferRef. The caller must keep using the frame
// only until it is freed (Frame.Free / FrameFree). The underlying []byte is kept alive internally
// until FFmpeg releases the AVBufferRef.
//
// Supported formats:
// - PixelFormatRGB24
// - PixelFormatRGBA / PixelFormatBGRA
// - PixelFormatYUV420P
// - PixelFormatNV12
func (f *Frame) WrapBuffer(data []byte, width, height int, format PixelFormat) error {
	if f == nil {
		return errors.New("ffgo: frame is nil")
	}
	if f.ptr != nil && !f.owned {
		return errors.New("ffgo: cannot WrapBuffer into a borrowed frame")
	}
	if width <= 0 || height <= 0 {
		return errors.New("ffgo: invalid dimensions")
	}
	if len(data) == 0 {
		return errors.New("ffgo: data cannot be empty")
	}

	if f.ptr == nil {
		f.ptr = avutil.FrameAlloc()
		if f.ptr == nil {
			return ErrOutOfMemory
		}
		f.owned = true
	}

	planes, linesizes, need, err := planVideoLayout(width, height, format)
	if err != nil {
		return err
	}
	if len(data) < need {
		return errors.New("ffgo: buffer is too small for frame layout")
	}

	initWrapCallback()

	wrapMu.Lock()
	defer wrapMu.Unlock()

	if lim := wrapLimitBytes.Load(); lim > 0 {
		if wrapPinnedBytes.Load()+int64(need) > lim {
			return errors.New("ffgo: WrapBuffer exceeds configured memory limit")
		}
	}

	// Clear existing refs/buffers.
	avutil.FrameUnref(f.ptr)

	// Keep the backing []byte alive until FFmpeg releases the AVBufferRef.
	h := handles.Register(wrappedBufferHold{data: data[:need], size: int64(need)})
	wrapPinnedBytes.Add(int64(need))
	wrapPinnedCount.Add(1)

	bufRef := avutil.BufferCreate(unsafe.Pointer(&data[0]), need, wrapFreeCBPtr, unsafe.Pointer(h), 0)
	if bufRef == nil {
		handles.Unregister(h)
		wrapPinnedBytes.Add(-int64(need))
		wrapPinnedCount.Add(-1)
		return errors.New("ffgo: av_buffer_create failed")
	}

	// Fill frame fields.
	avutil.SetFrameWidth(f.ptr, int32(width))
	avutil.SetFrameHeight(f.ptr, int32(height))
	avutil.SetFrameFormat(f.ptr, int32(format))

	// Set data pointers/linesizes and buffer bookkeeping using the selected ABI.
	avutil.ConfigureFrameBuffer(f.ptr, bufRef)
	for i, off := range planes {
		avutil.SetFrameDataPlane(
			f.ptr,
			i,
			unsafe.Pointer(uintptr(unsafe.Pointer(&data[0]))+uintptr(off)),
			int32(linesizes[i]),
		)
	}

	return nil
}

func planVideoLayout(w, h int, fmt PixelFormat) (planeOffsets []int, linesizes []int, total int, err error) {
	switch fmt {
	case PixelFormatRGB24:
		ls := w * 3
		total = ls * h
		return []int{0}, []int{ls}, total, nil
	case PixelFormatRGBA, PixelFormatBGRA:
		ls := w * 4
		total = ls * h
		return []int{0}, []int{ls}, total, nil
	case PixelFormatNV12:
		ls0 := w
		ls1 := w
		ySize := ls0 * h
		uvSize := ls1 * (h / 2)
		total = ySize + uvSize
		return []int{0, ySize}, []int{ls0, ls1}, total, nil
	case PixelFormatYUV420P:
		ls0 := w
		ls1 := w / 2
		ls2 := w / 2
		ySize := ls0 * h
		uSize := ls1 * (h / 2)
		vSize := ls2 * (h / 2)
		total = ySize + uSize + vSize
		return []int{0, ySize, ySize + uSize}, []int{ls0, ls1, ls2}, total, nil
	default:
		return nil, nil, 0, errors.New("ffgo: unsupported pixel format for WrapBuffer")
	}
}
