package scene_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestOnlyTheActivePainterCanWriteCellsAndRefineGeometry(t *testing.T) {
	var first scene.Element
	var held grid.View
	var candidate *scene.Staged[int]
	var snap scene.Snapshot[int]
	deny := func(label string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s wrote without its paint borrow", label)
			}
		}()
		fn()
	}
	write := func() { held.Text(0, 0, "X", grid.Style{}) }
	first = scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		held = frame.Sub(frame.Bounds()).View
		candidate = snap.Stage(frame, 1)
		frame.Text(0, 0, "A", grid.Style{})
		first.ReadOnly(func() { deny("read-only cell", write) })
	}})
	second := scene.NewElement(scene.ElementConfig{Paint: func(f scene.Frame) {
		deny("sibling cell", write)
		deny("sibling projection", func() { candidate.Set(2) })
		f.Text(0, 0, "B", grid.Style{})
	}})
	r := root(t, scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("a", first), item("b", second)}}).Element())
	src := grid.NewSurface(2, 2)
	p, err := r.Prepare(src.View())
	if err != nil {
		t.Fatal(err)
	}
	deny("pending frame", write)
	p.Commit(0)
	deny("settled frame", write)
	if snap.Value() != 1 || src.Rows()[0] != "A" {
		t.Errorf("changed committed geometry/cells: %d, %q", snap.Value(), src.Rows())
	}
	src.View().Text(0, 0, "C", grid.Style{}) // The caller still owns the underlying destination.
}

func TestNestedLayoutCannotOverwriteItsParentsGeometry(t *testing.T) {
	// Two legitimate layout calls reuse a scratch slice. The parent result must
	// be consumed as one value before visiting the child's callback.
	reused := make([]image.Rectangle, 2)
	leaf := scene.NewElement(scene.ElementConfig{Paint: func(f scene.Frame) { f.Text(0, 0, "B", grid.Style{}) }})
	inner := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("spacer", scene.NewElement(scene.ElementConfig{})), item("leaf", scene.NewElement(scene.ElementConfig{}))}, Layout: func(image.Point, []scene.Item) []image.Rectangle {
		reused[0], reused[1] = image.Rect(0, 0, 1, 1), image.Rect(0, 0, 1, 1)
		return reused
	}})
	parent := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{item("inner", inner.Element()), item("right", leaf)}, Layout: func(image.Point, []scene.Item) []image.Rectangle {
		reused[0], reused[1] = image.Rect(0, 0, 1, 1), image.Rect(1, 0, 2, 1)
		return reused
	}})
	r := root(t, parent.Element())
	s := grid.NewSurface(2, 1)
	p, err := r.Prepare(s.View())
	if err != nil {
		t.Fatal(err)
	}
	p.Commit(0)
	if got := s.Rows()[0]; got != " B" {
		t.Fatalf("nested layout rewrote parent: %q", got)
	}
}
