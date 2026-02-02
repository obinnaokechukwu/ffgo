//go:build !ios && !android && (amd64 || arm64)

package ffgo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func createMultiProgramTS(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()
	out := filepath.Join(tmpDir, "multi.ts")

	// Build an MPEG-TS with 2 programs:
	// - Program 1: video(0) + audio(1)
	// - Program 2: video(0) + audio(2)
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=1:size=160x120:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=1",
		"-f", "lavfi", "-i", "sine=frequency=880:duration=1",
		"-map", "0:v:0",
		"-map", "1:a:0",
		"-map", "2:a:0",
		"-c:v", "mpeg2video",
		"-c:a", "mp2",
		"-f", "mpegts",
		"-program", "program_num=1:title=prog1:st=0:st=1",
		"-program", "program_num=2:title=prog2:st=0:st=2",
		out,
	)
	if err := cmd.Run(); err != nil {
		t.Logf("ffmpeg not available or failed: %v", err)
		return ""
	}
	if _, err := os.Stat(out); err != nil {
		t.Logf("multi-program TS not created: %v", err)
		return ""
	}
	return out
}

func TestDecoderPrograms_ListAndSelect(t *testing.T) {
	if testing.Short() {
		t.Log("Skipping multi-program integration test in short mode")
		return
	}
	if !requireFFmpeg(t) {
		return
	}

	ts := createMultiProgramTS(t)
	if ts == "" {
		return
	}

	dec, err := NewDecoder(ts)
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}
	defer dec.Close()

	progs := dec.Programs()
	if len(progs) < 2 {
		t.Fatalf("expected at least 2 programs, got %d", len(progs))
	}

	ids := make(map[int]ProgramInfo)
	for _, p := range progs {
		ids[p.ID] = p
	}
	if _, ok := ids[1]; !ok {
		t.Fatalf("expected program id 1 to exist, got %#v", progs)
	}
	if _, ok := ids[2]; !ok {
		t.Fatalf("expected program id 2 to exist, got %#v", progs)
	}
	// Program metadata is muxer/build dependent; don't require it, but log it for debugging.
	t.Logf("program 1 metadata: %#v", ids[1].Metadata)

	// Selecting program 2 should choose streams within that program.
	dec2, err := NewDecoderWithOptions(ts, &DecoderOptions{
		ProgramID: 2,
	})
	if err != nil {
		t.Fatalf("NewDecoderWithOptions failed: %v", err)
	}
	defer dec2.Close()

	if !dec2.HasVideo() {
		t.Fatalf("expected selected program to have video")
	}
	if !dec2.HasAudio() {
		t.Fatalf("expected selected program to have audio")
	}
	if got := dec2.VideoStream().Index; got != 0 {
		t.Fatalf("expected video stream index 0, got %d", got)
	}
	if got := dec2.AudioStream().Index; got != 2 {
		t.Fatalf("expected audio stream index 2, got %d", got)
	}
}
