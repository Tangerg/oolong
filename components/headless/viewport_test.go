package headless_test

import (
	"image"
	"strconv"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

// tall is content of a fixed height that writes its own row number, so a test can see
// which part of it reached the screen.
type tall struct {
	rows int
	// mouse is the last position it was handed, in its own coordinates.
	mouse image.Point
	told  int
}

func (c *tall) Measure(int) int { return c.rows }

func (c *tall) Draw(v grid.View) {
	for y := range c.rows {
		v.Text(0, y, "row "+strconv.Itoa(y), grid.Style{})
	}
}

func (c *tall) Handle(ev input.Event) bool {
	if mouse, ok := ev.(input.Mouse); ok {
		c.mouse = mouse.Pos
		return true
	}
	return false
}

func (c *tall) Focus(has bool) {
	if has {
		c.told++
	}
}

func TestAWindowShowsThePartOfItsContentItIsScrolledTo(t *testing.T) {
	// The content is drawn at its whole height into a view that begins above the box,
	// so the rows off the top fall away and nothing had to be told it was scrolled.
	p := &headless.Viewport{Content: &tall{rows: 10}}
	paint(6, 3, p.Draw)
	p.Scroll().By(3)

	rows := paint(6, 3, p.Draw)
	equalRows(t, rows, []string{"row 3.", "row 4.", "row 5."})
}

func TestAWindowCannotBeScrolledPastItsContent(t *testing.T) {
	p := &headless.Viewport{Content: &tall{rows: 4}}
	paint(6, 3, p.Draw)
	p.Scroll().By(100)
	rows := paint(6, 3, p.Draw)
	equalRows(t, rows, []string{"row 1.", "row 2.", "row 3."})
}

func TestAWindowTakesTheWheelAndPassesOnTheRest(t *testing.T) {
	// Content that answered the wheel as well would scroll twice as far as the reader
	// asked. Everything else a pointer does is the content's, in the content's own
	// coordinates — which here means the row it is over and not the row on screen.
	content := &tall{rows: 20}
	p := &headless.Viewport{Content: content}
	paint(6, 4, p.Draw)

	for range 3 {
		p.Handle(input.Mouse{Action: input.WheelDown})
	}
	if got := p.Scroll().Offset(); got != 3 {
		t.Fatalf("offset after three wheel reports = %d", got)
	}
	if content.mouse != (image.Point{}) {
		t.Fatal("the wheel reached the content as well")
	}

	p.Handle(input.Mouse{Pos: image.Pt(2, 1), Action: input.MouseDown, Button: input.ButtonLeft})
	if want := image.Pt(2, 4); content.mouse != want {
		t.Fatalf("the content was told the press was at %v, want %v", content.mouse, want)
	}
}

func TestAWindowScrollsOnlyWhatItsContentDeclined(t *testing.T) {
	// Content with arrow keys of its own keeps them. A window that took them first
	// would make a list inside one impossible to move through.
	content := &tall{rows: 20}
	p := &headless.Viewport{Content: content}
	paint(6, 4, p.Draw)

	if !p.Handle(input.Key{Code: input.Down}) || p.Scroll().Offset() != 1 {
		t.Fatalf("the window did not scroll a key its content had no use for")
	}
	if !p.Handle(input.Key{Code: input.End}) || !p.Scroll().AtBottom() {
		t.Fatal("the window did not go to the end")
	}
	if p.Handle(input.Key{Code: input.Enter}) {
		t.Fatal("the window swallowed a key nobody wanted")
	}
}

func TestAWindowPassesTheKeyboardToWhatIsInIt(t *testing.T) {
	content := &tall{rows: 4}
	p := &headless.Viewport{Content: content}
	p.Focus(true)
	if content.told != 1 {
		t.Fatal("the content was not told it has the keyboard")
	}
}

func TestAnEmptyWindowDrawsNothingAndAnswersNothing(t *testing.T) {
	var p headless.Viewport
	paint(6, 3, p.Draw) // must not panic
	if p.Measure(6) != 0 {
		t.Fatal("a window with nothing in it asked for room")
	}
	if p.Handle(input.Key{Code: input.Down}) {
		t.Fatal("a window with nothing in it scrolled")
	}
}
