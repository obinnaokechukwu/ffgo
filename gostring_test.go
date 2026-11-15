package ffgo

import (
	"strings"
	"testing"
	"unsafe"
)

// The subtitle helper reads a C string longer than the old 4096-byte safety
// limit whole, never a slice-bounds panic (the same defect as avformat's
// helper).
func TestGoStringReadsAStringLongerThan4096Bytes(t *testing.T) {
	for _, n := range []int{0, 1, 4095, 4096, 4097, 70000} {
		buf := append([]byte(strings.Repeat("a", n)), 0)
		if got := goString(unsafe.Pointer(&buf[0])); len(got) != n {
			t.Fatalf("goString of %d bytes read %d", n, len(got))
		}
	}
}
