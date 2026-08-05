package headless

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/primitives/grid"
	"github.com/Tangerg/oolong/primitives/input"
	"github.com/Tangerg/oolong/primitives/layout"
)

// panel is a test modal that records what it was given.
type panel struct {
	name      string
	place     layout.Placement
	takes     input.Code
	seen      []input.Event
	drawnInto []int
	closed    int
	stick     bool
}

func (p *panel) Draw(v grid.View) {
	w, h := v.Size()
	p.drawnInto = []int{w, h}
	v.Text(0, 0, p.name, grid.Style{})
}

func (p *panel) Handle(ev input.Event) bool {
	p.seen = append(p.seen, ev)
	key, ok := ev.(input.Key)
	return ok && p.takes != 0 && key.Code == p.takes
}

func (p *panel) Place(layout.Size) layout.Placement { return p.place }

func (p *panel) Closed() { p.closed++ }

type stickyPanel struct{ panel }

func (s *stickyPanel) Sticky() bool { return s.stick }

func middle(w, h int) layout.Placement {
	return layout.Placement{Anchor: layout.Middle, Width: w, Height: h}
}

func draw(s *Stack, w, h int) {
	s.Draw(grid.NewSurface(w, h).View())
}

func esc() input.Event { return input.Key{Code: input.Esc} }

func TestAnEmptyStackConsumesNothing(t *testing.T) {
	// So an interface can offer it every event and carry on when it is not
	// interested.
	var s Stack
	if s.Handle(esc()) {
		t.Fatal("an empty stack consumed a key")
	}
	if !s.Empty() || s.Top() != nil || s.Depth() != 0 {
		t.Fatal("the zero stack is not empty")
	}
	draw(&s, 10, 10) // must not panic
}

func TestOnlyTheTopLayerSeesInput(t *testing.T) {
	under := &panel{name: "under", place: middle(6, 2)}
	over := &panel{name: "over", place: middle(4, 2)}
	var s Stack
	s.Push(under)
	s.Push(over)
	draw(&s, 20, 10)

	s.Handle(input.Key{Code: input.Down})
	if len(under.seen) != 0 {
		t.Fatalf("the covered layer saw %d events, want none", len(under.seen))
	}
	if len(over.seen) != 1 {
		t.Fatalf("the top layer saw %d events, want the one", len(over.seen))
	}
}

func TestAStackWithAnythingInItSwallowsEveryKey(t *testing.T) {
	// A key reaching what a modal is covering acts somewhere the user is not
	// looking.
	p := &panel{name: "p", place: middle(4, 2)}
	var s Stack
	s.Push(p)
	draw(&s, 20, 10)
	if !s.Handle(input.Key{Code: input.Character, Rune: 'x'}) {
		t.Fatal("a key fell through to what the modal is covering")
	}
}

func TestEscapePopsTheTopLayer(t *testing.T) {
	var s Stack
	s.Push(&panel{name: "a", place: middle(4, 2)})
	s.Push(&panel{name: "b", place: middle(4, 2)})
	draw(&s, 20, 10)

	s.Handle(esc())
	if s.Depth() != 1 {
		t.Fatalf("depth = %d, want the top layer gone", s.Depth())
	}
	s.Handle(esc())
	if !s.Empty() {
		t.Fatal("the second escape did not empty the stack")
	}
}

func TestALayerThatConsumesEscapeKeepsIt(t *testing.T) {
	p := &panel{name: "p", place: middle(4, 2), takes: input.Esc}
	var s Stack
	s.Push(p)
	draw(&s, 20, 10)
	s.Handle(esc())
	if s.Depth() != 1 {
		t.Fatal("the layer consumed escape and was popped anyway")
	}
}

func TestAStickyLayerRefusesToBeDismissed(t *testing.T) {
	// The layer that has to be answered rather than dismissed. Consuming escape
	// and doing nothing would read as a bug at the call site.
	p := &stickyPanel{panel: panel{name: "p", place: middle(4, 2)}}
	p.stick = true
	var s Stack
	s.Push(p)
	draw(&s, 20, 10)

	s.Handle(esc())
	if s.Depth() != 1 {
		t.Fatal("a sticky layer was dismissed")
	}
	// And stops insisting once it has what it needs.
	p.stick = false
	s.Handle(esc())
	if !s.Empty() {
		t.Fatal("the layer stopped being sticky and was still not dismissed")
	}
}

func TestAPressOutsideTheLayerPopsIt(t *testing.T) {
	p := &panel{name: "p", place: middle(4, 2)}
	var s Stack
	s.Push(p)
	draw(&s, 20, 10)

	area, ok := s.Area()
	if !ok || area.Empty() {
		t.Fatal("the layer was not placed")
	}
	outside := image.Pt(area.Min.X-2, area.Min.Y)
	s.Handle(input.Mouse{Pos: outside, Action: input.MouseDown, Button: input.ButtonLeft})
	if !s.Empty() {
		t.Fatal("a press outside the layer did not dismiss it")
	}
}

func TestAPressOutsideCanBeKept(t *testing.T) {
	p := &panel{name: "p", place: middle(4, 2)}
	s := Stack{KeepOnClickOutside: true}
	s.Push(p)
	draw(&s, 20, 10)

	area, _ := s.Area()
	s.Handle(input.Mouse{
		Pos: image.Pt(area.Min.X-2, area.Min.Y), Action: input.MouseDown, Button: input.ButtonLeft,
	})
	if s.Empty() {
		t.Fatal("the layer was dismissed by a press it was told to ignore")
	}
}

func TestTheWheelOutsideTheLayerBelongsToWhatIsUnderIt(t *testing.T) {
	// A modal does not stop the transcript behind it from scrolling.
	p := &panel{name: "p", place: middle(4, 2)}
	var s Stack
	s.Push(p)
	draw(&s, 20, 10)

	area, _ := s.Area()
	outside := image.Pt(area.Min.X-2, area.Min.Y)
	if s.Handle(input.Mouse{Pos: outside, Action: input.WheelDown}) {
		t.Fatal("the stack swallowed a wheel event outside the layer")
	}
	if s.Empty() {
		t.Fatal("scrolling outside the layer dismissed it")
	}
}

func TestAMouseEventArrivesInTheLayersOwnCoordinates(t *testing.T) {
	// The layer draws into a view whose origin is its own, so it reasons in its own
	// coordinates and a position has to arrive in them.
	p := &panel{name: "p", place: middle(4, 2)}
	var s Stack
	s.Push(p)
	draw(&s, 20, 10)

	area, _ := s.Area()
	if area.Min.X == 0 && area.Min.Y == 0 {
		t.Fatal("the layer landed at the origin, so this test would prove nothing")
	}
	s.Handle(input.Mouse{Pos: area.Min, Action: input.MouseDown, Button: input.ButtonLeft})
	if len(p.seen) != 1 {
		t.Fatalf("the layer saw %d events, want the one", len(p.seen))
	}
	got := p.seen[0].(input.Mouse).Pos
	if got != (image.Point{}) {
		t.Fatalf("the layer was told the press was at %v, want its own origin", got)
	}
}

func TestALayerIsDrawnIntoTheSpaceItAskedFor(t *testing.T) {
	p := &panel{name: "p", place: middle(6, 3)}
	var s Stack
	s.Push(p)
	draw(&s, 20, 10)
	if len(p.drawnInto) != 2 || p.drawnInto[0] != 6 || p.drawnInto[1] != 3 {
		t.Fatalf("drawn into %v, want the 6x3 it asked for", p.drawnInto)
	}
}

func TestPoppingTellsALayerItIsClosed(t *testing.T) {
	p := &panel{name: "p", place: middle(4, 2)}
	var s Stack
	s.Push(p)
	s.Pop()
	if p.closed != 1 {
		t.Fatalf("closed %d times, want once", p.closed)
	}
}

func TestClearPopsFromTheTopDown(t *testing.T) {
	var order []string
	mark := func(name string) *closingPanel {
		return &closingPanel{
			panel: panel{name: name, place: middle(4, 2)},
			note:  func() { order = append(order, name) },
		}
	}
	var s Stack
	s.Push(mark("a"))
	s.Push(mark("b"))
	s.Push(mark("c"))
	s.Clear()

	if !s.Empty() {
		t.Fatal("the stack was not emptied")
	}
	want := []string{"c", "b", "a"}
	for i := range want {
		if i >= len(order) || order[i] != want[i] {
			t.Fatalf("closed in the order %v, want %v", order, want)
		}
	}
}

type closingPanel struct {
	panel
	note func()
}

func (c *closingPanel) Closed() { c.note() }

func TestPoppingAnEmptyStackIsNotAPanic(t *testing.T) {
	var s Stack
	if s.Pop() {
		t.Fatal("popped something from an empty stack")
	}
}

func TestPushingNothingIsIgnored(t *testing.T) {
	var s Stack
	s.Push(nil)
	if !s.Empty() {
		t.Fatal("a nil layer went onto the stack, where it would panic on the next frame")
	}
}

func TestTheEscapeBindingCanBeRebound(t *testing.T) {
	p := &panel{name: "p", place: middle(4, 2)}
	s := Stack{Escape: Binding{Key: input.Key{Code: input.Character, Rune: 'q'}, Does: "close"}}
	s.Push(p)
	draw(&s, 20, 10)

	s.Handle(esc())
	if s.Empty() {
		t.Fatal("escape closed a stack that was rebound away from it")
	}
	s.Handle(input.Key{Code: input.Character, Rune: 'q'})
	if !s.Empty() {
		t.Fatal("the rebound key did not close the stack")
	}
}

// backdropPanel reaches past its own area to dim what it covers.
type backdropPanel struct {
	panel
	backdrops []int
}

func (b *backdropPanel) Backdrop(v grid.View) {
	w, h := v.Size()
	b.backdrops = append(b.backdrops, w, h)
}

func TestABackdropIsGivenTheWholeSpaceAndTheLayerIsNot(t *testing.T) {
	// A layer is handed its own area and nothing else, which is what stops it
	// drawing outside its box. Dimming what is behind needs the rest, so it is a
	// separate question with a separate answer.
	b := &backdropPanel{panel: panel{name: "b", place: middle(4, 2)}}
	var s Stack
	s.Push(b)
	draw(&s, 20, 10)

	if len(b.backdrops) != 2 || b.backdrops[0] != 20 || b.backdrops[1] != 10 {
		t.Fatalf("backdrop given %v, want the whole 20x10 space", b.backdrops)
	}
	if len(b.drawnInto) != 2 || b.drawnInto[0] != 4 || b.drawnInto[1] != 2 {
		t.Fatalf("drawn into %v, want only the 4x2 it asked for", b.drawnInto)
	}
}

func TestALayerWithNoBackdropIsNotAskedForOne(t *testing.T) {
	p := &panel{name: "p", place: middle(4, 2)}
	var s Stack
	s.Push(p)
	draw(&s, 20, 10) // must not panic looking for a method that is not there
	if p.drawnInto[0] != 4 {
		t.Fatal("the layer was drawn into the wrong space")
	}
}
