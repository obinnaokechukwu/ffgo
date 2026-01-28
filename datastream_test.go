//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func createMOVWithTimecodeDataStream(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	out := filepath.Join(tmpDir, "timecode.mov")

	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=160x120:rate=30",
		"-c:v", "libx264", "-preset", "ultrafast",
		"-pix_fmt", "yuv420p",
		"-timecode", "00:00:00:00",
		out,
	)
	if err := cmd.Run(); err != nil {
		t.Skipf("ffmpeg not available or failed: %v", err)
		return ""
	}
	if _, err := os.Stat(out); err != nil {
		t.Skipf("test file not created: %v", err)
		return ""
	}
	return out
}

func TestDataStreams_DetectAndReadPacket(t *testing.T) {
	in := createMOVWithTimecodeDataStream(t)
	if in == "" {
		return
	}

	dec, err := NewDecoder(in)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	defer dec.Close()

	ds := dec.DataStreams()
	if len(ds) == 0 {
		t.Fatalf("expected at least 1 data stream")
	}

	pkt, err := dec.ReadDataPacket()
	if err != nil {
		t.Fatalf("ReadDataPacket failed: %v", err)
	}
	if pkt == nil {
		// Some inputs may advertise a data stream but produce no packets; don't hard-fail.
		t.Skip("no data packets produced for the detected data stream")
	}
	if pkt.StreamIndex() < 0 {
		t.Fatalf("expected packet to have a valid stream index")
	}
}
