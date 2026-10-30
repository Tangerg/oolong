package base_test

import (
	"image"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestClicksCountAGesture(t *testing.T) {
	// A terminal reports two presses and never a double-click, so whether they are one
	// gesture is a question about when they arrived.
	var c base.Clicks
	base := time.Unix(0, 0)
	at := image.Pt(10, 4)

	if got := c.Press(input.Mouse{Pos: at, At: base}); got != 1 {
		t.Errorf("the first press is %d, want 1", got)
	}
	if got := c.Press(input.Mouse{Pos: at, At: base.Add(100 * time.Millisecond)}); got != 2 {
		t.Errorf("a press soon after is %d, want 2", got)
	}
	if got := c.Press(input.Mouse{Pos: at, At: base.Add(200 * time.Millisecond)}); got != 3 {
		t.Errorf("a third is %d, want 3", got)
	}
	// Long after, the run starts again.
	if got := c.Press(input.Mouse{Pos: at, At: base.Add(2 * time.Second)}); got != 1 {
		t.Errorf("a press long after is %d, want 1", got)
	}
}

func TestClicksStartANewRunWhenTimeMovesBackwards(t *testing.T) {
	var clicks base.Clicks
	at := image.Pt(10, 4)
	later := time.Unix(10, 0)
	clicks.Press(input.Mouse{Pos: at, At: later})
	if got := clicks.Press(input.Mouse{Pos: at, At: later.Add(-time.Second)}); got != 1 {
		t.Fatalf("press in an earlier clock epoch is %d, want a new run", got)
	}
}

func TestClicksBreakWhenThePointerMoves(t *testing.T) {
	var c base.Clicks
	base := time.Unix(0, 0)
	c.Press(input.Mouse{Pos: image.Pt(10, 4), At: base})
	if got := c.Press(input.Mouse{Pos: image.Pt(30, 4), At: base.Add(50 * time.Millisecond)}); got != 1 {
		t.Errorf("a press elsewhere is %d, want 1", got)
	}
}

func TestClicksCannotWrapDistantCoordinatesTogether(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	var clicks base.Clicks
	base := time.Unix(0, 0)
	clicks.Press(input.Mouse{Pos: image.Pt(maxInt, maxInt), At: base})
	if got := clicks.Press(input.Mouse{
		Pos: image.Pt(minInt, minInt), At: base.Add(time.Millisecond),
	}); got != 1 {
		t.Fatalf("opposite coordinate extremes formed click run %d, want a new run", got)
	}
}

// TestClicksAllowAHandThatDrifts. A gesture that broke because the pointer moved one
// column would feel like the double-click had failed.
func TestClicksAllowAHandThatDrifts(t *testing.T) {
	var c base.Clicks
	base := time.Unix(0, 0)
	c.Press(input.Mouse{Pos: image.Pt(10, 4), At: base})
	if got := c.Press(input.Mouse{Pos: image.Pt(11, 5), At: base.Add(50 * time.Millisecond)}); got != 2 {
		t.Errorf("a press one cell away is %d, want 2", got)
	}
}

func TestClicksTakeTheirOwnInterval(t *testing.T) {
	c := base.Clicks{Within: 10 * time.Millisecond}
	base := time.Unix(0, 0)
	c.Press(input.Mouse{Pos: image.Pt(0, 0), At: base})
	if got := c.Press(input.Mouse{Pos: image.Pt(0, 0), At: base.Add(50 * time.Millisecond)}); got != 1 {
		t.Errorf("a press past the interval is %d, want 1", got)
	}
}

func TestClicksReset(t *testing.T) {
	var c base.Clicks
	base := time.Unix(0, 0)
	c.Press(input.Mouse{Pos: image.Pt(0, 0), At: base})
	c.Reset()
	if got := c.Press(input.Mouse{Pos: image.Pt(0, 0), At: base.Add(time.Millisecond)}); got != 1 {
		t.Errorf("the press after a reset is %d, want 1", got)
	}
}

func TestClicksBreakWhenThePointerMovesUpAndLeft(t *testing.T) {
	// The other sign, which is the branch a single direction of drift never reaches.
	var c base.Clicks
	base := time.Unix(0, 0)
	c.Press(input.Mouse{Pos: image.Pt(10, 4), At: base})
	if got := c.Press(input.Mouse{Pos: image.Pt(4, 1), At: base.Add(50 * time.Millisecond)}); got != 1 {
		t.Errorf("a press up and to the left is %d, want 1", got)
	}
}
