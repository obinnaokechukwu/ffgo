//go:build !ios && !android && (amd64 || arm64)

package shim

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFindShimLibrary_RespectsFFGOShimDir(t *testing.T) {
	dir := t.TempDir()

	var name string
	switch runtime.GOOS {
	case "linux", "freebsd", "openbsd", "netbsd":
		name = "libffshim.so"
	case "darwin":
		name = "libffshim.dylib"
	case "windows":
		name = "ffshim.dll"
	default:
		t.Skip("unsupported OS for this test")
	}

	fake := filepath.Join(dir, name)
	if err := os.WriteFile(fake, []byte("not a real shim"), 0o644); err != nil {
		t.Fatalf("write fake shim: %v", err)
	}

	t.Setenv("FFGO_SHIM_DIR", dir)

	got, err := findShimLibrary()
	if err != nil {
		t.Fatalf("findShimLibrary error: %v", err)
	}
	if got != fake {
		t.Fatalf("expected %q, got %q", fake, got)
	}
}

