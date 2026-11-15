package avformat

import (
	"strings"
	"testing"
	"unsafe"
)

// A C string longer than the old 4096-byte safety limit is read whole, never
// a slice-bounds panic: a metadata value over 4 KiB (a lyrics tag, read
// through Decoder.GetMetadata) crashed the caller.
func TestGoStringReadsAStringLongerThan4096Bytes(t *testing.T) {
	for _, n := range []int{0, 1, 4095, 4096, 4097, 5000, 70000} {
		buf := append([]byte(strings.Repeat("a", n)), 0)
		got := goString(unsafe.Pointer(&buf[0]))
		if len(got) != n {
			t.Fatalf("goString of %d bytes read %d", n, len(got))
		}
	}
	if goString(nil) != "" {
		t.Fatal("nil is the empty string")
	}
}
