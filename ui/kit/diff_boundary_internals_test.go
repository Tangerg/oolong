package kit

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/Tangerg/oolong/ui/core/diff"
)

func expectInvalidDiffKind(t *testing.T, run func()) {
	t.Helper()
	defer func() {
		err, _ := recover().(error)
		if !errors.Is(err, diff.ErrInvalidKind) {
			t.Errorf("panic = %v, want %v", err, diff.ErrInvalidKind)
		}
	}()
	run()
}

func TestDiffRejectsTheWholeCandidateBeforeReplacingItsSnapshot(t *testing.T) {
	view := NewDiff(DiffConfig{Hunks: []diff.Hunk{{Lines: diff.Script{{Kind: diff.Added, Text: "retained words"}}}}})
	before := view.Hunks()
	measured := view.layout(8)
	wrapped := view.wrapped
	copied := *view
	candidate := []diff.Hunk{
		{Lines: diff.Script{{Kind: diff.Removed, Text: "valid earlier hunk"}}},
		{Lines: diff.Script{{Kind: diff.Kind(math.MaxUint8), Text: "invalid later hunk"}}},
	}
	expectInvalidDiffKind(t, func() { view.SetHunks(candidate) })
	if !reflect.DeepEqual(view.Hunks(), before) {
		t.Fatalf("rejected hunks replaced the previous snapshot: %+v", view.Hunks())
	}
	if !reflect.DeepEqual(view.wrapped, wrapped) || len(view.wrapped.rows) == 0 || &view.wrapped.rows[0] != &measured[0] {
		t.Fatal("rejected hunks cleared or replaced the measured layout")
	}
	if !reflect.DeepEqual(copied.Hunks(), before) || !reflect.DeepEqual(copied.wrapped, wrapped) {
		t.Fatal("rejected hunks changed a retained copy")
	}
	candidate[0].Lines[0].Text = "changed after rejection"
	if !reflect.DeepEqual(view.Hunks(), before) {
		t.Fatal("rejected candidate storage became retained state")
	}
	expectInvalidDiffKind(t, func() { NewDiff(DiffConfig{Hunks: candidate}) })
	var absent *Diff
	expectInvalidDiffKind(t, func() { absent.SetHunks(candidate) })
}
