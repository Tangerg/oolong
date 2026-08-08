package headless

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"
)

func TestEditorKillRingIsBoundedAndDropsItsOldestEntry(t *testing.T) {
	var ring editorKillRing
	for i := range editorKillRingLimit + 3 {
		ring.add(fmt.Sprintf("kill-%d", i), false, false)
	}
	if got := len(ring.entries); got != editorKillRingLimit {
		t.Fatalf("ring length = %d, want %d", got, editorKillRingLimit)
	}
	if got := ring.entries[0]; got != "kill-18" {
		t.Fatalf("newest = %q, want kill-18", got)
	}
	if got := ring.entries[len(ring.entries)-1]; got != "kill-3" {
		t.Fatalf("oldest retained = %q, want kill-3", got)
	}
}

func TestEditorKillRingOwnsAndJoinsText(t *testing.T) {
	source := strings.Repeat("x", 1024) + "middle" + strings.Repeat("x", 1024)
	middle := source[1024 : 1024+len("middle")]
	var ring editorKillRing
	ring.add(middle, false, false)
	owned, ok := ring.newest()
	if !ok || unsafe.StringData(owned) == unsafe.StringData(middle) { //nolint:gosec // compare ownership; no pointer is dereferenced.
		t.Fatal("kill ring retained the caller's string backing storage")
	}
	ring.add("end", false, true)
	ring.add("start", true, true)

	got, ok := ring.newest()
	if !ok || got != "startmiddleend" {
		t.Fatalf("joined kill = %q (ok=%v), want startmiddleend", got, ok)
	}
}
