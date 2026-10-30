package base

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestScrollCandidateRefinementPublishesOnlyAsACompleteFrame(t *testing.T) {
	var scroll Scroll
	total, window := 100, 10
	refine := func(*ScrollLayout) {}
	element := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { candidate := scroll.Stage(frame, total, window); refine(&candidate) }})
	root, err := scene.NewRoot(scene.RootConfig{Element: element})
	if err != nil {
		t.Fatal(err)
	}
	surface := grid.NewSurface(8, 10)
	frame, err := root.Prepare(surface.View())
	if err != nil {
		t.Fatal(err)
	}
	if scroll.resolved().total != 0 || scroll.resolved().window != 0 {
		t.Fatal("candidate leaked derived extents before commit")
	}
	frame.Commit(1)
	scroll.By(50)
	before := scroll.Offset()
	total, window = 100, 100
	refine = func(candidate *ScrollLayout) {
		if candidate.Offset() != 0 {
			t.Fatal("provisional clamp wrong")
		}
		candidate.Resize(10)
		if candidate.Offset() != before {
			t.Fatalf("refining provisional clamp lost original intention: %d", candidate.Offset())
		}
		candidate.Reveal(80, 80)
		candidate.Resize(20)
		if candidate.Offset() != 61 {
			t.Fatalf("refined reveal=%d", candidate.Offset())
		}
	}
	frame, err = root.Prepare(surface.View())
	if err != nil {
		t.Fatal(err)
	}
	if scroll.Offset() != before || scroll.resolved().window != 10 {
		t.Fatal("refinement leaked before whole-frame admission")
	}
	frame.Abort()
	if scroll.Offset() != before || scroll.resolved().window != 10 {
		t.Fatal("aborted candidate changed scroll intent or bounds")
	}
	frame, err = root.Prepare(surface.View())
	if err != nil {
		t.Fatal(err)
	}
	frame.Commit(2)
	if scroll.Offset() != 61 || scroll.resolved().window != 20 {
		t.Fatalf("committed refinement=%d window=%d", scroll.Offset(), scroll.resolved().window)
	}
}
