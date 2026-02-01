//go:build !ios && !android && (amd64 || arm64)

package shim

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestFindShimLibrary_FFGO_SHIM_DIR_NotFound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FFGO_SHIM_DIR", dir)

	_, err := findShimLibrary()
	if err == nil {
		t.Fatal("expected error when FFGO_SHIM_DIR doesn't contain shim")
	}
	if !strings.Contains(err.Error(), "FFGO_SHIM_DIR") {
		t.Errorf("error should mention FFGO_SHIM_DIR: %v", err)
	}
}

func TestExpectedLibraryName(t *testing.T) {
	name := ExpectedLibraryName()

	switch runtime.GOOS {
	case "linux", "freebsd", "openbsd", "netbsd":
		if name != "libffshim.so" {
			t.Errorf("expected libffshim.so on %s, got %s", runtime.GOOS, name)
		}
	case "darwin":
		if name != "libffshim.dylib" {
			t.Errorf("expected libffshim.dylib on darwin, got %s", name)
		}
	case "windows":
		if name != "ffshim.dll" {
			t.Errorf("expected ffshim.dll on windows, got %s", name)
		}
	}
}

func TestBuildInstructions(t *testing.T) {
	instructions := BuildInstructions()

	if instructions == "" {
		t.Error("BuildInstructions should not be empty")
	}

	// Should contain platform-specific guidance
	switch runtime.GOOS {
	case "linux":
		if !strings.Contains(instructions, "apt") && !strings.Contains(instructions, "libav") {
			t.Error("Linux instructions should mention apt or libav packages")
		}
	case "darwin":
		if !strings.Contains(instructions, "brew") && !strings.Contains(instructions, "Homebrew") {
			t.Error("macOS instructions should mention Homebrew")
		}
	case "windows":
		if !strings.Contains(instructions, "MSYS2") && !strings.Contains(instructions, "MinGW") {
			t.Error("Windows instructions should mention MSYS2 or MinGW")
		}
	}
}

func TestStatus_BeforeLoad(t *testing.T) {
	// Create a fresh state by resetting (this is just for testing)
	loadMu.Lock()
	wasLoaded := loaded
	wasErr := loadErr
	wasPath := shimPath
	loaded = false
	loadErr = nil
	shimPath = ""
	loadMu.Unlock()

	// Restore after test
	defer func() {
		loadMu.Lock()
		loaded = wasLoaded
		loadErr = wasErr
		shimPath = wasPath
		loadMu.Unlock()
	}()

	status := Status()
	if !strings.Contains(status, "not loaded") {
		t.Errorf("Status should indicate not loaded: %s", status)
	}
}

func TestIsLoaded_Initial(t *testing.T) {
	// This test just verifies the function doesn't panic
	_ = IsLoaded()
}

func TestPath_WhenNotLoaded(t *testing.T) {
	// If shim is not loaded, Path should return empty string
	loadMu.Lock()
	wasLoaded := loaded
	wasPath := shimPath
	if !loaded {
		shimPath = ""
	}
	loadMu.Unlock()

	defer func() {
		loadMu.Lock()
		loaded = wasLoaded
		shimPath = wasPath
		loadMu.Unlock()
	}()

	if !IsLoaded() {
		path := Path()
		if path != "" && !IsLoaded() {
			t.Error("Path should be empty when shim is not loaded")
		}
	}
}

func TestSetLogCallback_WithoutShim(t *testing.T) {
	// Test that calling SetLogCallback without shim returns appropriate error
	loadMu.Lock()
	wasLoaded := loaded
	loaded = false
	loadMu.Unlock()

	defer func() {
		loadMu.Lock()
		loaded = wasLoaded
		loadMu.Unlock()
	}()

	err := SetLogCallback(0)
	if err == nil {
		t.Error("SetLogCallback should fail when shim is not loaded")
	}
	if !strings.Contains(err.Error(), "shim") {
		t.Errorf("error should mention shim: %v", err)
	}
}

func TestSetLogLevel_WithoutShim(t *testing.T) {
	loadMu.Lock()
	wasLoaded := loaded
	loaded = false
	loadMu.Unlock()

	defer func() {
		loadMu.Lock()
		loaded = wasLoaded
		loadMu.Unlock()
	}()

	err := SetLogLevel(32)
	if err == nil {
		t.Error("SetLogLevel should fail when shim is not loaded")
	}
}

func TestLog_WithoutShim(t *testing.T) {
	loadMu.Lock()
	wasLoaded := loaded
	loaded = false
	loadMu.Unlock()

	defer func() {
		loadMu.Lock()
		loaded = wasLoaded
		loadMu.Unlock()
	}()

	err := Log(nil, 32, "test message")
	if err == nil {
		t.Error("Log should fail when shim is not loaded")
	}
}

func TestNewChapter_WithoutShim(t *testing.T) {
	loadMu.Lock()
	wasLoaded := loaded
	loaded = false
	loadMu.Unlock()

	defer func() {
		loadMu.Lock()
		loaded = wasLoaded
		loadMu.Unlock()
	}()

	_, err := NewChapter(nil, 1, 1, 1000, 0, 1000, nil)
	if err == nil {
		t.Error("NewChapter should fail when shim is not loaded")
	}
}

func TestAVDeviceListInputSources_WithoutShim(t *testing.T) {
	loadMu.Lock()
	wasLoaded := loaded
	loaded = false
	loadMu.Unlock()

	defer func() {
		loadMu.Lock()
		loaded = wasLoaded
		loadMu.Unlock()
	}()

	_, _, _, err := AVDeviceListInputSources("v4l2", "", nil)
	if err == nil {
		t.Error("AVDeviceListInputSources should fail when shim is not loaded")
	}
}

func TestAVFrameColorOffsets_WithoutShim(t *testing.T) {
	loadMu.Lock()
	wasLoaded := loaded
	loaded = false
	loadMu.Unlock()

	defer func() {
		loadMu.Lock()
		loaded = wasLoaded
		loadMu.Unlock()
	}()

	_, _, _, _, err := AVFrameColorOffsets()
	if err == nil {
		t.Error("AVFrameColorOffsets should fail when shim is not loaded")
	}
}

// Integration test - only runs if shim is available
func TestLoad_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	err := Load()
	if err != nil {
		t.Logf("Load returned error (expected if shim not built): %v", err)
	}

	// After Load, we should be able to check status
	status := Status()
	t.Logf("Shim status: %s", status)

	if IsLoaded() {
		path := Path()
		t.Logf("Shim loaded from: %s", path)

		// Test that we can call shim functions without panic
		// (actual functionality depends on FFmpeg being available)
	} else {
		t.Log("Shim not loaded - this is OK, core functionality works without it")
		t.Logf("To enable logging, %s", BuildInstructions())
	}
}

// Test that SearchError provides useful information
func TestSearchError(t *testing.T) {
	// Force a load attempt
	_ = Load()

	if !IsLoaded() {
		searchErr := SearchError()
		if searchErr == "" {
			t.Log("SearchError is empty (shim might have loaded)")
		} else {
			t.Logf("SearchError: %s", searchErr)
			// Should mention something useful
			if !strings.Contains(searchErr, "shim") && !strings.Contains(searchErr, "not found") && !strings.Contains(searchErr, "FFGO_SHIM_DIR") {
				t.Error("SearchError should contain useful diagnostic information")
			}
		}
	}
}
