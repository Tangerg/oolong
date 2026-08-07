package kit

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

type pointerRegionTarget struct {
	takes  bool
	events []input.Mouse
}

func (t *pointerRegionTarget) handlePointer(event input.Mouse) bool {
	t.events = append(t.events, event)
	return t.takes
}

type pointerRegionFixture struct {
	region pointerRegion
	area   image.Rectangle
	target pointerHandler
}

func (f *pointerRegionFixture) Draw(frame headless.Frame) {
	f.region.stage(frame, f.area, f.target)
}

func (f *pointerRegionFixture) Handle(event input.Event) bool {
	mouse, ok := event.(input.Mouse)
	if !ok {
		return false
	}
	handled, _ := f.region.handle(mouse)
	return handled
}

func drawPointerRegion(fixture *pointerRegionFixture) {
	headless.NewRoot(fixture).Draw(grid.NewSurface(12, 6).View())
}

// The region is the common ownership boundary for every kit wrapper. This one test
// deliberately changes both halves of the presentation after a press: the old target
// still owns the gesture, while an unrelated target must not inherit its tail.
func TestPointerRegionKeepsTheAcceptedGestureWithItsOriginalTarget(t *testing.T) {
	first := &pointerRegionTarget{takes: true}
	second := &pointerRegionTarget{takes: true}
	fixture := &pointerRegionFixture{
		area: image.Rect(2, 1, 10, 5), target: first,
	}
	drawPointerRegion(fixture)

	if !fixture.Handle(input.Mouse{
		Pos: image.Pt(5, 3), Action: input.MouseDown, Button: input.ButtonLeft,
	}) {
		t.Fatal("the target declined its press")
	}
	if got := first.events[0].Pos; got != image.Pt(3, 2) {
		t.Fatalf("local press = %v, want (3,2)", got)
	}

	// A complete frame replaces the visible target and collapses the region. Neither
	// change may strand the gesture or transfer it to the replacement.
	fixture.area = image.Rectangle{}
	fixture.target = second
	drawPointerRegion(fixture)
	for _, event := range []input.Mouse{
		{Pos: image.Pt(11, 5), Action: input.MouseDrag},
		{Pos: image.Pt(11, 5), Action: input.MouseUp},
	} {
		if !fixture.Handle(event) {
			t.Fatalf("the original target declined %v", event.Action)
		}
	}
	if got := len(first.events); got != 3 {
		t.Fatalf("original target received %d events, want press, drag and release", got)
	}
	if got := first.events[1].Pos; got != image.Pt(9, 4) {
		t.Fatalf("captured drag used mixed-frame coordinates %v, want (9,4)", got)
	}
	if got := len(second.events); got != 0 {
		t.Fatalf("replacement inherited %d events from another target's gesture", got)
	}
	if fixture.Handle(input.Mouse{Pos: image.Pt(11, 5), Action: input.MouseDrag}) {
		t.Fatal("the region retained capture after release")
	}
}

func TestPointerRegionRoutesAgainstTheLastCompleteFrame(t *testing.T) {
	first := &pointerRegionTarget{takes: true}
	second := &pointerRegionTarget{takes: true}
	fixture := &pointerRegionFixture{
		area: image.Rect(1, 1, 5, 4), target: first,
	}
	drawPointerRegion(fixture)

	// Mutating the semantic child is not publication. Until another complete frame,
	// a press is about the target the user can still see.
	fixture.target = second
	press := input.Mouse{Pos: image.Pt(2, 2), Action: input.MouseDown, Button: input.ButtonLeft}
	if !fixture.Handle(press) || len(first.events) != 1 || len(second.events) != 0 {
		t.Fatal("an unpublished target received a press")
	}
	fixture.Handle(input.Mouse{Pos: press.Pos, Action: input.MouseUp})

	drawPointerRegion(fixture)
	if !fixture.Handle(press) || len(second.events) != 1 {
		t.Fatal("the replacement did not receive input after its frame was published")
	}
}

func TestPointerRegionCapturesOnlyAnAcceptedPress(t *testing.T) {
	target := &pointerRegionTarget{}
	fixture := &pointerRegionFixture{
		area: image.Rect(1, 1, 5, 4), target: target,
	}
	drawPointerRegion(fixture)

	if fixture.Handle(input.Mouse{
		Pos: image.Pt(2, 2), Action: input.MouseDown, Button: input.ButtonLeft,
	}) {
		t.Fatal("a declined press was reported handled")
	}
	if fixture.Handle(input.Mouse{Pos: image.Pt(9, 5), Action: input.MouseDrag}) {
		t.Fatal("a declined press captured the drag")
	}
	if got := len(target.events); got != 1 {
		t.Fatalf("target received %d events, want only the press", got)
	}
}
