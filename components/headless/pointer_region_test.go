package headless_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

type pointerTarget struct {
	takes  bool
	events []input.Mouse
}

func (t *pointerTarget) Draw(headless.Frame) {}

func (t *pointerTarget) Handle(event input.Event) bool {
	mouse, ok := event.(input.Mouse)
	if !ok {
		return false
	}
	t.events = append(t.events, mouse)
	return t.takes
}

func (t *pointerTarget) last() image.Point {
	if len(t.events) == 0 {
		return image.Pt(-1, -1)
	}
	return t.events[len(t.events)-1].Pos
}

// pointerRegionFixture is a wrapper that draws one child somewhere. Changing area or
// child and drawing again is how a test publishes a new frame.
type pointerRegionFixture struct {
	region headless.PointerRegion
	area   image.Rectangle
	child  headless.Widget
}

func (f *pointerRegionFixture) Draw(frame headless.Frame) {
	f.region.Stage(frame, f.area, f.child)
}

func (f *pointerRegionFixture) Handle(event input.Event) bool {
	mouse, ok := event.(input.Mouse)
	if !ok {
		return false
	}
	handled, _ := f.region.Handle(mouse)
	return handled
}

func (f *pointerRegionFixture) draw() {
	headless.NewRoot(f).Draw(grid.NewSurface(14, 8).View())
}

func regionPress(x, y int) input.Mouse {
	return input.Mouse{Pos: image.Pt(x, y), Action: input.MouseDown, Button: input.ButtonLeft}
}

func regionDrag(x, y int) input.Mouse {
	return input.Mouse{Pos: image.Pt(x, y), Action: input.MouseDrag}
}

func regionRelease(x, y int) input.Mouse {
	return input.Mouse{Pos: image.Pt(x, y), Action: input.MouseUp}
}

// TestPointerRegionFollowsItsChildWhenTheLayoutMoves is the reason a gesture is
// remembered by identity and not by geometry.
//
// The user is aiming at what is on the screen. A child that is still presented, one
// row lower than when the press landed, must receive coordinates measured from where
// it is now — a selection translated by where it used to be jumps by exactly the
// distance the layout moved.
func TestPointerRegionFollowsItsChildWhenTheLayoutMoves(t *testing.T) {
	child := &pointerTarget{takes: true}
	fixture := &pointerRegionFixture{area: image.Rect(2, 1, 10, 5), child: child}
	fixture.draw()

	fixture.Handle(regionPress(5, 3))
	if got := child.last(); got != image.Pt(3, 2) {
		t.Fatalf("local press = %v, want (3,2)", got)
	}

	// The same child, drawn one row lower.
	fixture.area = image.Rect(2, 2, 10, 6)
	fixture.draw()
	if !fixture.Handle(regionDrag(5, 3)) {
		t.Fatal("the child lost a drag it had begun")
	}
	if got := child.last(); got != image.Pt(3, 1) {
		t.Fatalf("local drag = %v, want (3,1) measured from where the child is now", got)
	}
}

// TestPointerRegionDropsAGestureWhoseChildIsGone keeps the same answer [Container]
// gives: a child that is no longer presented has nowhere to send the rest of its
// gesture, and that is not the same as the gesture belonging to its replacement.
// There is no rectangle to measure a screen position against, so inventing one would
// hand the child a coordinate it never occupied.
func TestPointerRegionDropsAGestureWhoseChildIsGone(t *testing.T) {
	child := &pointerTarget{takes: true}
	replacement := &pointerTarget{takes: true}
	fixture := &pointerRegionFixture{area: image.Rect(2, 1, 10, 5), child: child}
	fixture.draw()

	if !fixture.Handle(regionPress(5, 3)) {
		t.Fatal("the child declined its press")
	}
	fixture.child = replacement
	fixture.draw()

	if fixture.Handle(regionDrag(5, 3)) || fixture.Handle(regionRelease(5, 3)) {
		t.Fatal("a gesture was delivered after its child stopped being presented")
	}
	if len(child.events) != 1 {
		t.Fatalf("the original child received %d events, want only its press", len(child.events))
	}
	if len(replacement.events) != 0 {
		t.Fatalf("the replacement inherited %d events from another child's gesture", len(replacement.events))
	}
}

// TestPointerRegionRoutesAgainstTheLastCompleteFrame: changing what a wrapper holds
// is not publication. Until another complete frame, a press is about what the user
// can still see.
func TestPointerRegionRoutesAgainstTheLastCompleteFrame(t *testing.T) {
	shown := &pointerTarget{takes: true}
	hidden := &pointerTarget{takes: true}
	fixture := &pointerRegionFixture{area: image.Rect(1, 1, 5, 4), child: shown}
	fixture.draw()

	fixture.child = hidden
	if !fixture.Handle(regionPress(2, 2)) || len(shown.events) != 1 || len(hidden.events) != 0 {
		t.Fatal("an unpublished child received a press")
	}
	fixture.Handle(regionRelease(2, 2))

	fixture.draw()
	if !fixture.Handle(regionPress(2, 2)) || len(hidden.events) != 1 {
		t.Fatal("the replacement did not receive input once its frame was published")
	}
}

// TestPointerRegionCapturesOnlyAnAcceptedPress: holding a press the child refused
// would swallow the release belonging to whatever the pointer is really over.
func TestPointerRegionCapturesOnlyAnAcceptedPress(t *testing.T) {
	child := &pointerTarget{}
	fixture := &pointerRegionFixture{area: image.Rect(1, 1, 5, 4), child: child}
	fixture.draw()

	if fixture.Handle(regionPress(2, 2)) {
		t.Fatal("a declined press was reported handled")
	}
	if fixture.Handle(regionDrag(9, 5)) {
		t.Fatal("a declined press captured the drag")
	}
	if len(child.events) != 1 {
		t.Fatalf("child received %d events, want only the press", len(child.events))
	}
}

// TestPointerRegionEndsOwnershipAtTheRelease: the interior decides again afterwards.
func TestPointerRegionEndsOwnershipAtTheRelease(t *testing.T) {
	child := &pointerTarget{takes: true}
	fixture := &pointerRegionFixture{area: image.Rect(2, 1, 10, 5), child: child}
	fixture.draw()

	fixture.Handle(regionPress(5, 3))
	if !fixture.Handle(regionDrag(0, 0)) {
		t.Fatal("the child lost a drag outside its area")
	}
	if !fixture.Handle(regionRelease(0, 0)) {
		t.Fatal("the child lost the release that ends its gesture")
	}
	if fixture.Handle(regionDrag(0, 0)) {
		t.Fatal("ownership outlived the release")
	}
}

// TestPointerRegionDeclinesAPassiveChild: a child that answers no input is staged as
// an absence, so a press cannot land on a region nothing owns.
func TestPointerRegionDeclinesAPassiveChild(t *testing.T) {
	fixture := &pointerRegionFixture{area: image.Rect(1, 1, 5, 4), child: passiveWidget{}}
	fixture.draw()

	if handled, delivered := fixture.region.Handle(regionPress(2, 2)); handled || delivered {
		t.Fatalf("a passive child took a press: handled=%v delivered=%v", handled, delivered)
	}
}

type passiveWidget struct{}

func (passiveWidget) Draw(headless.Frame) {}
