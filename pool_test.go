//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"testing"
	"unsafe"

	"github.com/obinnaokechukwu/ffgo/avutil"
)

func TestFramePool_GetPutAndLimit(t *testing.T) {
	if !requireFFmpeg(t) {
		return
	}

	p := NewFramePool(1)
	defer p.Close()

	f1, err := p.Get()
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if _, err := p.Get(); err == nil {
		t.Fatalf("expected pool exhausted error")
	}
	if err := p.Put(&f1); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if !f1.IsNil() {
		t.Fatalf("expected Put to clear frame")
	}
	if _, err := p.Get(); err != nil {
		t.Fatalf("Get after Put failed: %v", err)
	}
}

func TestFrameWrapBuffer_RGB24(t *testing.T) {
	if !requireFFmpeg(t) {
		return
	}

	SetWrappedBufferMemoryLimit(0)

	before := WrappedBufferMemoryUsage()

	var f Frame
	w, h := 8, 4
	buf := make([]byte, w*h*3)
	if err := f.WrapBuffer(buf, w, h, PixelFormatRGB24); err != nil {
		t.Fatalf("WrapBuffer failed: %v", err)
	}

	if got := int(avutil.GetFrameWidth(f.ptr)); got != w {
		t.Fatalf("width: got %d want %d", got, w)
	}
	if got := int(avutil.GetFrameHeight(f.ptr)); got != h {
		t.Fatalf("height: got %d want %d", got, h)
	}

	p0 := avutil.GetFrameDataPlane(f.ptr, 0)
	if p0 != unsafe.Pointer(&buf[0]) {
		t.Fatalf("data[0] pointer mismatch: got=%p want=%p", p0, unsafe.Pointer(&buf[0]))
	}
	ls0 := avutil.GetFrameLinesizePlane(f.ptr, 0)
	if int(ls0) != w*3 {
		t.Fatalf("linesize[0]: got %d want %d", ls0, w*3)
	}

	after := WrappedBufferMemoryUsage()
	if after.PinnedBuffers != before.PinnedBuffers+1 {
		t.Fatalf("pinned buffers: before=%d after=%d", before.PinnedBuffers, after.PinnedBuffers)
	}
	if after.PinnedBytes < before.PinnedBytes+int64(len(buf)) {
		t.Fatalf("pinned bytes did not increase as expected: before=%d after=%d", before.PinnedBytes, after.PinnedBytes)
	}

	if err := FrameFree(&f); err != nil {
		t.Fatalf("FrameFree failed: %v", err)
	}
	final := WrappedBufferMemoryUsage()
	if final.PinnedBuffers != before.PinnedBuffers || final.PinnedBytes != before.PinnedBytes {
		t.Fatalf("expected pinned usage to return to baseline: before=%v final=%v", before, final)
	}
}

func TestFrameWrapBuffer_MemoryLimit(t *testing.T) {
	if !requireFFmpeg(t) {
		return
	}

	defer SetWrappedBufferMemoryLimit(0)

	SetWrappedBufferMemoryLimit(16)

	var f Frame
	buf := make([]byte, 64)
	if err := f.WrapBuffer(buf, 8, 4, PixelFormatRGB24); err == nil {
		_ = FrameFree(&f)
		t.Fatalf("expected WrapBuffer to fail due to memory limit")
	}
}
