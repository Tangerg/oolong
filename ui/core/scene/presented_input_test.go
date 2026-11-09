package scene_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestPresentedInputUsesObservedPrefixWithoutClockOrdering(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		root, value, hit := inputProjection(t, 0)
		instant := time.Now()
		draw(t, root, 1, 1, 1)
		mouse := input.Mouse{Action: input.MouseDown, At: instant}
		captured := root.CapturePresentedInput(mouse, 1)
		root.HandleInput(mouseAt(root, instant, 1))
		if *hit != 0 {
			t.Fatal("synthetic capture weakened physical arrival ordering")
		}
		root.HandleInput(captured)
		if *hit != 1 {
			t.Fatal("completed output at the same instant could not receive pointer intent")
		}
		*value = 2
		draw(t, root, 1, 1, 0)
		root.HandleInput(root.CapturePresentedInput(mouse, 1))
		if *hit != 2 {
			t.Fatal("current pointer intent missed a completed zero-byte geometry update")
		}
		root.HandleInput(captured)
		if *hit != 1 {
			t.Fatal("a zero-byte geometry update retargeted buffered pointer intent")
		}
		*value = 3
		draw(t, root, 1, 1, 3)
		root.HandleInput(root.CapturePresentedInput(mouse, 1))
		if *hit != 2 {
			t.Fatal("an unobserved writer sequence retargeted pointer intent")
		}
		root.HandleInput(root.CapturePresentedInput(mouse, 3))
		if *hit != 3 || !time.Now().Equal(instant) {
			t.Fatalf("latest projection = %d, elapsed = %s", *hit, time.Since(instant))
		}
	})
}

func TestPresentedInputRejectsMissingGeometry(t *testing.T) {
	root, _, _ := inputProjection(t, 1)
	mouse := input.Mouse{Action: input.MouseDown}
	if root.CapturePresentedInput(mouse, 0).Input() != nil {
		t.Fatal("unpainted root accepted pointer intent")
	}
	draw(t, root, 1, 1, 1)
	candidate, err := root.Prepare(grid.NewSurface(1, 1).View())
	if err != nil {
		t.Fatal(err)
	}
	if root.CapturePresentedInput(mouse, 1).Input() != nil {
		t.Fatal("pending frame admission accepted pointer intent")
	}
	candidate.Abort()
	if root.CapturePresentedInput(mouse, 1).Input() == nil {
		t.Fatal("aborted admission retained its input barrier")
	}
	draw(t, root, 1, 1, 2)
	if root.CapturePresentedInput(mouse, 1).Input() != nil {
		t.Fatal("evicted writer evidence acquired a newer projection")
	}
	root.Close()
	if root.CapturePresentedInput(mouse, 2).Input() != nil {
		t.Fatal("closed root accepted pointer intent")
	}
}
