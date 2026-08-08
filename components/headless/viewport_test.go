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
	mouse   image.Point
	told    int
	focused bool
}

func (c *tall) Measure(int) int { return c.rows }

func (c *tall) Draw(v headless.Frame) {
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
	c.focused = has
	if has {
		c.told++
	}
}

func TestAWindowShowsThePartOfItsContentItIsScrolledTo(t *testing.T) {
	// The content is drawn at its whole height into a view that begins above the box,
	// so the rows off the top fall away and nothing had to be told it was scrolled.
	p := headless.NewViewport(&tall{rows: 10})
	paintWidget(6, 3, p)
	p.Scroll().By(3)

	rows := paintWidget(6, 3, p)
	equalRows(t, rows, []string{"row 3.", "row 4.", "row 5."})
}

func TestAWindowCannotBeScrolledPastItsContent(t *testing.T) {
	p := headless.NewViewport(&tall{rows: 4})
	paintWidget(6, 3, p)
	p.Scroll().By(100)
	rows := paintWidget(6, 3, p)
	equalRows(t, rows, []string{"row 1.", "row 2.", "row 3."})
}

func TestAWindowTakesTheWheelAndPassesOnTheRest(t *testing.T) {
	// Content that answered the wheel as well would scroll twice as far as the reader
	// asked. Everything else a pointer does is the content's, in the content's own
	// coordinates — which here means the row it is over and not the row on screen.
	content := &tall{rows: 20}
	p := headless.NewViewport(content)
	paintWidget(6, 4, p)

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
	if want := image.Pt(2, 1); content.mouse != want {
		t.Fatalf("before repaint the content was told the press was at %v, want the visible %v", content.mouse, want)
	}
	paintWidget(6, 4, p)
	p.Handle(input.Mouse{Pos: image.Pt(2, 1), Action: input.MouseDown, Button: input.ButtonLeft})
	if want := image.Pt(2, 4); content.mouse != want {
		t.Fatalf("the content was told the press was at %v, want %v", content.mouse, want)
	}
}

func TestAWindowScrollsOnlyWhatItsContentDeclined(t *testing.T) {
	// Content with arrow keys of its own keeps them. A window that took them first
	// would make a list inside one impossible to move through.
	content := &tall{rows: 20}
	p := headless.NewViewport(content)
	paintWidget(6, 4, p)

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
	p := headless.NewViewport(content)
	p.Focus(true)
	if content.told != 1 {
		t.Fatal("the content was not told it has the keyboard")
	}
}

func TestAWindowTransfersKeyboardOwnershipWithItsContent(t *testing.T) {
	first := &tall{rows: 4}
	second := &tall{rows: 4}
	p := headless.NewViewport(first)
	p.SetContent(second)
	if p.Content() != second || first.focused || !second.focused {
		t.Fatalf("focus after replacement: first=%v second=%v", first.focused, second.focused)
	}

	p.Focus(false)
	p.SetContent(first)
	if first.focused || second.focused {
		t.Fatalf("blurred replacement gained focus: first=%v second=%v", first.focused, second.focused)
	}
}

func TestAnEmptyWindowDrawsNothingAndAnswersNothing(t *testing.T) {
	var p headless.Viewport
	paintWidget(6, 3, &p) // must not panic
	if p.Measure(6) != 0 {
		t.Fatal("a window with nothing in it asked for room")
	}
	if p.Handle(input.Key{Code: input.Down}) {
		t.Fatal("a window with nothing in it scrolled")
	}
}

func TestAWindowAnswersToTheNameOfWhatItDoes(t *testing.T) {
	// Its own actions and its content's, which is what lets one be driven from a menu
	// or from a command typed by name.
	p := headless.NewViewport(&tall{rows: 20})
	paintWidget(6, 4, p)

	if !p.Do(headless.ScrollBottom) || !p.Scroll().AtBottom() {
		t.Fatal("the window did not go to the end")
	}
	if !p.Do(headless.ScrollTop) || p.Scroll().Offset() != 0 {
		t.Fatal("the window did not go back to the start")
	}
	if !p.Do(headless.ScrollPageDown) || p.Scroll().Offset() != 3 {
		t.Fatalf("a page took it to %d, want a window less the row of overlap", p.Scroll().Offset())
	}
	if p.Do("fly") {
		t.Fatal("the window claimed an action nobody taught it")
	}
}
