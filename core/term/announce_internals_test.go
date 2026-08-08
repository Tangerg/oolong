package term

import (
	"strings"
	"testing"
	"unsafe"
)

func TestTitleOwnsTheTextItRetainsAcrossHandover(t *testing.T) {
	backing := strings.Repeat("x", 4096) + "kept"
	source := backing[len(backing)-len("kept"):]
	var state title
	state.to(source)
	if state.text != source {
		t.Fatalf("retained title = %q, want %q", state.text, source)
	}
	if unsafe.StringData(state.text) == unsafe.StringData(source) { //nolint:gosec // Test compares allocation identity and never dereferences the address.
		t.Fatal("a short title retained its caller's larger backing string")
	}
}
