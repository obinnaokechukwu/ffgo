//go:build !ios && !android && (amd64 || arm64)

package avcodec

import (
	"testing"

	"github.com/obinnaokechukwu/ffgo/internal/bindings"
)

func init() {
	if err := bindings.Load(); err != nil {
		panic("Failed to load FFmpeg: " + err.Error())
	}
}

func TestFindDecoder(t *testing.T) {
	// H.264 decoder should always be available
	codec := FindDecoder(CodecIDH264)
	if codec == nil {
		t.Fatal("FindDecoder(H264) returned nil")
	}

	name := GetCodecName(codec)
	if name == "" {
		t.Error("GetCodecName returned empty string")
	}
	t.Logf("H.264 decoder: %s", name)
}

func TestFindEncoder(t *testing.T) {
	// Try to find an encoder - some systems may not have all encoders
	codecs := []struct {
		id   CodecID
		name string
	}{
		{CodecIDH264, "H.264"},
		{CodecIDMPEG4, "MPEG4"},
		{CodecIDMJPEG, "MJPEG"},
	}

	found := false
	for _, c := range codecs {
		codec := FindEncoder(c.id)
		if codec != nil {
			name := GetCodecName(codec)
			t.Logf("Found %s encoder: %s", c.name, name)
			found = true
			break
		}
	}

	if !found {
		t.Log("No common encoders found - this may be expected on some systems")
	}
}

func TestFindDecoderByName(t *testing.T) {
	codec := FindDecoderByName("h264")
	if codec == nil {
		t.Skip("h264 decoder not found by name")
	}

	name := GetCodecName(codec)
	t.Logf("Found decoder: %s", name)
}

func TestAllocContext3(t *testing.T) {
	codec := FindDecoder(CodecIDH264)
	if codec == nil {
		t.Skip("H264 decoder not found")
	}

	ctx := AllocContext3(codec)
	if ctx == nil {
		t.Fatal("AllocContext3 returned nil")
	}
	defer FreeContext(&ctx)

	if ctx == nil {
		t.Error("Context should still be valid before free")
	}
}

func TestFreeContext(t *testing.T) {
	codec := FindDecoder(CodecIDH264)
	if codec == nil {
		t.Skip("H264 decoder not found")
	}

	ctx := AllocContext3(codec)
	if ctx == nil {
		t.Fatal("AllocContext3 returned nil")
	}

	FreeContext(&ctx)

	if ctx != nil {
		t.Error("Context should be nil after free")
	}

	// Double free should be safe
	FreeContext(&ctx)
}

func TestPacketAllocFree(t *testing.T) {
	pkt := PacketAlloc()
	if pkt == nil {
		t.Fatal("PacketAlloc returned nil")
	}

	PacketFree(&pkt)

	if pkt != nil {
		t.Error("Packet should be nil after free")
	}

	// Double free should be safe
	PacketFree(&pkt)
}

func TestCodecIDConstants(t *testing.T) {
	// Verify codec IDs match FFmpeg constants
	if CodecIDH264 != 27 {
		t.Errorf("CodecIDH264: expected 27, got %d", CodecIDH264)
	}
	if CodecIDHEVC != 173 {
		t.Errorf("CodecIDHEVC: expected 173, got %d", CodecIDHEVC)
	}
	if CodecIDAV1 != 226 {
		t.Errorf("CodecIDAV1: expected 226, got %d", CodecIDAV1)
	}
}

func TestVersion(t *testing.T) {
	ver := bindings.AVCodecVersion()
	if ver == 0 {
		t.Error("AVCodecVersion returned 0")
	}
	t.Logf("avcodec version: %d.%d.%d", ver>>16, (ver>>8)&0xFF, ver&0xFF)
}
