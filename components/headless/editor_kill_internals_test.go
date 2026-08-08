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
	if got := ring.entries[0].materialise(); got != "kill-18" {
		t.Fatalf("newest = %q, want kill-18", got)
	}
	if got := ring.entries[len(ring.entries)-1].materialise(); got != "kill-3" {
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

func TestEditorKillJoinsStayChunkedUntilRead(t *testing.T) {
	const chunks = 4096
	var ring editorKillRing
	ring.add("middle", false, false)
	for range chunks {
		ring.add("a", false, true)
		ring.add("b", true, true)
	}
	entry := &ring.entries[0]
	if len(entry.before) != chunks || len(entry.after) != chunks || entry.value != "middle" {
		t.Fatalf("joined kill was eagerly assembled: before=%d after=%d value bytes=%d",
			len(entry.before), len(entry.after), len(entry.value))
	}
	got := entry.materialise()
	want := strings.Repeat("b", chunks) + "middle" + strings.Repeat("a", chunks)
	if got != want {
		t.Fatalf("settled kill has %d bytes, want %d", len(got), len(want))
	}
	if entry.before != nil || entry.after != nil {
		t.Fatal("settled kill retained its chunks")
	}
}

func TestEditorKillsDetachTheSurvivingHalfLine(t *testing.T) {
	const kept = "kept"
	for _, tc := range []struct {
		name string
		text string
		col  int
		kill func(*Editor)
	}{
		{
			name: "end",
			text: kept + strings.Repeat("discarded", 1<<12),
			col:  len(kept),
			kill: (*Editor).KillToEnd,
		},
		{
			name: "start",
			text: strings.Repeat("discarded", 1<<12) + kept,
			col:  len(strings.Repeat("discarded", 1<<12)),
			kill: (*Editor).KillToStart,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			editor := NewEditor()
			editor.SetText(tc.text)
			editor.SetCursor(0, tc.col)
			source := editor.lines[0]
			sourceStart := uintptr(unsafe.Pointer(unsafe.StringData(source))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
			sourceEnd := sourceStart + uintptr(len(source))
			tc.kill(editor)
			if got := editor.Text(); got != kept {
				t.Fatalf("survivor = %q, want kept", got)
			}
			at := uintptr(unsafe.Pointer(unsafe.StringData(editor.lines[0]))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
			if at >= sourceStart && at < sourceEnd {
				t.Fatal("surviving half line still shares the removed line allocation")
			}
		})
	}
}
