//go:build !ios && !android && (amd64 || arm64)

package platform

import (
	"runtime"
	"testing"
)

func TestSupportsStructByValue(t *testing.T) {
	// On Darwin (macOS), struct by value should be supported
	// On other platforms, it should not be supported
	if runtime.GOOS == "darwin" && (runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64") {
		if !SupportsStructByValue {
			t.Error("Darwin amd64/arm64 should support struct by value")
		}
	} else {
		if SupportsStructByValue {
			t.Errorf("%s/%s should not support struct by value", runtime.GOOS, runtime.GOARCH)
		}
	}
}

func TestIs64Bit(t *testing.T) {
	// We only support 64-bit platforms
	if !Is64Bit {
		t.Error("Platform should be 64-bit")
	}
}

func TestLibraryExtension(t *testing.T) {
	switch runtime.GOOS {
	case "darwin":
		if LibraryExtension != ".dylib" {
			t.Errorf("expected .dylib, got %s", LibraryExtension)
		}
	case "windows":
		if LibraryExtension != ".dll" {
			t.Errorf("expected .dll, got %s", LibraryExtension)
		}
	default:
		if LibraryExtension != ".so" {
			t.Errorf("expected .so, got %s", LibraryExtension)
		}
	}
}

func TestLibraryPrefix(t *testing.T) {
	switch runtime.GOOS {
	case "windows":
		if LibraryPrefix != "" {
			t.Errorf("expected empty prefix on Windows, got %s", LibraryPrefix)
		}
	default:
		if LibraryPrefix != "lib" {
			t.Errorf("expected 'lib' prefix, got %s", LibraryPrefix)
		}
	}
}

func TestFormatLibraryName(t *testing.T) {
	tests := []struct {
		name    string
		version int
		goos    string
		want    string
	}{
		{"avcodec", 60, "linux", "libavcodec.so.60"},
		{"avcodec", 0, "linux", "libavcodec.so"},
		{"avcodec", 60, "darwin", "libavcodec.60.dylib"},
		{"avcodec", 0, "darwin", "libavcodec.dylib"},
		{"avcodec", 60, "windows", "avcodec-60.dll"},
		{"avcodec", 0, "windows", "avcodec.dll"},
	}

	for _, tt := range tests {
		t.Run(tt.name+"_"+tt.goos, func(t *testing.T) {
			if runtime.GOOS != tt.goos {
				t.Skipf("test only applies to %s", tt.goos)
			}
			got := FormatLibraryName(tt.name, tt.version)
			if got != tt.want {
				t.Errorf("FormatLibraryName(%q, %d) = %q, want %q", tt.name, tt.version, got, tt.want)
			}
		})
	}
}
