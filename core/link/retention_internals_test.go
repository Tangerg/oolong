package link

import (
	"strings"
	"testing"
	"unsafe"
)

func TestFilteringLinksClearsDiscardedStorage(t *testing.T) {
	found := []Link{
		{Start: 0, End: 10, Target: "kept"},
		{Start: 2, End: 5, Target: "discarded"},
		{Start: 20, End: 25, Target: "also kept"},
	}
	ordered := inOrder(found)
	if len(ordered) != 2 {
		t.Fatalf("kept %d links, want two", len(ordered))
	}
	for i, discarded := range found[len(ordered):] {
		if discarded != (Link{}) {
			t.Fatalf("discarded slot %d retained %+v", i, discarded)
		}
	}
}

func TestDetectedTargetsDoNotRetainTheirSource(t *testing.T) {
	source := strings.Repeat("ordinary prose ", 1<<12) + "/tmp/result.txt"
	links := Detect(source, nil)
	if len(links) != 1 {
		t.Fatalf("found %d links, want one", len(links))
	}
	sourceStart := uintptr(unsafe.Pointer(unsafe.StringData(source)))          //nolint:gosec // Test compares allocation identity and never dereferences the address.
	targetStart := uintptr(unsafe.Pointer(unsafe.StringData(links[0].Target))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	if targetStart >= sourceStart && targetStart < sourceStart+uintptr(len(source)) {
		t.Fatal("a short detected target still shares the complete source allocation")
	}
}
