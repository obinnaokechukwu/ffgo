package avutil

import (
	"strings"
	"testing"
	"unsafe"
)

// A C string longer than the old 256-byte safety limit is read whole, never a
// slice-bounds panic (the same defect as avformat's helper).
func TestGoStringReadsAStringLongerThan256Bytes(t *testing.T) {
	for _, n := range []int{0, 1, 255, 256, 257, 5000} {
		buf := append([]byte(strings.Repeat("a", n)), 0)
		if got := goString(unsafe.Pointer(&buf[0])); len(got) != n {
			t.Fatalf("goString of %d bytes read %d", n, len(got))
		}
	}
}
