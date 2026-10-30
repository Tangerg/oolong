package base_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestReadOnlyRefinementCannotPublishASecondScrollCandidate(t *testing.T) {
	for _, operation := range []struct {
		name string
		run  func(*base.ScrollLayout)
	}{
		{name: "resize", run: func(candidate *base.ScrollLayout) { candidate.Resize(100) }},
		{name: "reveal", run: func(candidate *base.ScrollLayout) { candidate.Reveal(80, 80) }},
		{name: "anchor", run: func(candidate *base.ScrollLayout) { candidate.Anchor(80) }},
	} {
		t.Run(operation.name, func(t *testing.T) {
			var scroll base.Scroll
			armed := false
			var borrowed *base.ScrollLayout
			var element scene.Element
			element = scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
				candidate := scroll.Stage(frame, 100, 10)
				borrowed = &candidate
				if !armed {
					return
				}
				before := candidate.Offset()
				element.ReadOnly(func() {
					defer func() {
						got, _ := recover().(error)
						if !errors.Is(got, scene.ErrBusy) {
							t.Errorf("read-only refinement=%v", got)
						}
					}()
					operation.run(&candidate)
				})
				if candidate.Offset() != before {
					t.Fatal("recovered refinement exposed an uncommitted second candidate")
				}
				candidate.Resize(20)
				if candidate.Offset() != before {
					t.Fatal("failed refinement leaked a reveal or anchor into later preparation")
				}
			}})
			root, err := scene.NewRoot(scene.RootConfig{Element: element})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(root.Close)
			for sequence := uint64(1); sequence <= 2; sequence++ {
				candidate, err := root.Prepare(grid.NewSurface(8, 10).View())
				if err != nil {
					t.Fatal(err)
				}
				func() {
					defer func() {
						if recover() == nil {
							t.Error("expired ScrollLayout still exposed candidate state")
						}
					}()
					borrowed.Offset()
				}()
				candidate.Commit(sequence)
				if sequence == 1 {
					scroll.By(30)
					armed = true
				}
			}
			if scroll.Offset() != 30 {
				t.Fatal("frame committed geometry inconsistent with its recovered drawing")
			}
			scroll.Pages(1)
			if scroll.Offset() != 49 {
				t.Fatal("paging did not use the complete committed twenty-row window")
			}
		})
	}
}
