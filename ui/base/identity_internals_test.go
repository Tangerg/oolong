package base

import (
	"math"
	"testing"
)

func TestIdentitySequenceStopsBeforeReusingAValue(t *testing.T) {
	sequence := identitySequence{last: math.MaxUint64 - 1}
	if got, ok := sequence.next(); !ok || got != math.MaxUint64 {
		t.Fatalf("last identity = %d, %t; want %d, true", got, ok, uint64(math.MaxUint64))
	}
	if got, ok := sequence.next(); ok || got != 0 {
		t.Fatalf("exhausted identity = %d, %t; want 0, false", got, ok)
	}
}

func TestIdentityOwnersFailBeforeChangingTheirState(t *testing.T) {
	t.Run("editor", func(t *testing.T) {
		editor := &Editor{}
		editor.SetText("kept")
		editor.document().elementIDs.last = math.MaxUint64
		got := panicFrom(func() { editor.InsertElement(0, "new") })
		if got != "base: editor exhausted element identities" {
			t.Fatalf("panic = %v, want the editor exhaustion contract", got)
		}
		if text := editor.Text(); text != "kept" {
			t.Fatalf("text = %q after refused insertion, want kept", text)
		}
	})
}

func panicFrom(f func()) (value any) {
	defer func() { value = recover() }()
	f()
	return nil
}
