package layout_test

import (
	"testing"

	"github.com/Tangerg/oolong/core/layout"

	"github.com/Tangerg/oolong/core/grid"
)

// wants is a measurer that asks for a fixed amount, remembering what it was asked.
type wants struct {
	give   int
	across int
	asked  bool
}

func (w *wants) Measure(across int) int {
	w.across, w.asked = across, true
	return w.give
}

func sizes(views []grid.View, vertical bool) []int {
	out := make([]int, len(views))
	for i, v := range views {
		w, h := v.Size()
		if vertical {
			out[i] = h
		} else {
			out[i] = w
		}
	}
	return out
}

func equal(t *testing.T, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestFixedSlotsAreHonouredBeforeAnythingElse(t *testing.T) {
	views := layout.Rows(grid.NewSurface(10, 10).View(),
		layout.Slot{Size: layout.Fixed(2)},
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Fixed(3)},
	)
	equal(t, sizes(views, true), []int{2, 5, 3})
}

func TestFlexSlotsSplitWhatIsLeftInProportion(t *testing.T) {
	views := layout.Rows(grid.NewSurface(4, 9).View(),
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Flex(2)},
	)
	equal(t, sizes(views, true), []int{3, 6})
}

func TestTheRoundingRemainderIsNotLost(t *testing.T) {
	// A row lost to rounding is a gap the user can see.
	views := layout.Rows(grid.NewSurface(4, 10).View(),
		layout.Slot{Size: layout.Flex(1)},
		layout.Slot{Size: layout.Flex(2)},
	)
	total := 0
	for _, n := range sizes(views, true) {
		total += n
	}
	if total != 10 {
		t.Fatalf("slots add up to %d rows, want all 10", total)
	}
}

func TestAMeasuredSlotIsAskedAcrossTheOtherAxis(t *testing.T) {
	w := &wants{give: 3}
	views := layout.Rows(grid.NewSurface(12, 10).View(),
		layout.Slot{Size: layout.Measured(0, 0), Of: w},
		layout.Slot{Size: layout.Flex(1)},
	)
	if !w.asked {
		t.Fatal("the measurer was never asked")
	}
	if w.across != 12 {
		t.Fatalf("measured across %d, want the width of 12", w.across)
	}
	equal(t, sizes(views, true), []int{3, 7})
}

func TestMeasuringWorksAcrossBothAxes(t *testing.T) {
	// The reason Measure takes the axis it is not deciding: a slot in layout.Columns is
	// asked for a width at a height, and one knob means the same thing in both. A
	// measured column that silently came out zero wide is what this replaced.
	w := &wants{give: 4}
	views := layout.Columns(grid.NewSurface(30, 5).View(),
		layout.Slot{Size: layout.Measured(0, 0), Of: w},
		layout.Slot{Size: layout.Flex(1)},
	)
	if w.across != 5 {
		t.Fatalf("measured across %d, want the height of 5", w.across)
	}
	equal(t, sizes(views, false), []int{4, 26})
}

func TestAMeasuredSlotIsHeldBetweenItsFloorAndItsCap(t *testing.T) {
	for _, tc := range []struct {
		name       string
		give       int
		min, max   int
		wantHeight int
	}{
		{"below the floor", 1, 3, 0, 3},
		{"above the cap", 9, 0, 4, 4},
		{"between the two", 5, 3, 8, 5},
		{"no cap at all", 9, 0, 0, 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			views := layout.Rows(grid.NewSurface(10, 20).View(),
				layout.Slot{Size: layout.Measured(tc.min, tc.max), Of: &wants{give: tc.give}},
				layout.Slot{Size: layout.Flex(1)},
			)
			if got := sizes(views, true)[0]; got != tc.wantHeight {
				t.Fatalf("measured slot = %d rows, want %d", got, tc.wantHeight)
			}
		})
	}
}

func TestAMeasuredSlotWithNothingToAskGetsItsFloor(t *testing.T) {
	views := layout.Rows(grid.NewSurface(10, 20).View(),
		layout.Slot{Size: layout.Measured(2, 0)},
		layout.Slot{Size: layout.Flex(1)},
	)
	equal(t, sizes(views, true), []int{2, 18})
}

func TestNeverHandsOutMoreThanThereIs(t *testing.T) {
	// Two floors that do not both fit. Honouring them both would tell the second
	// slot it had eight rows while the view clipped it to two — and a widget lays
	// out against what it was told, not against what the user can see.
	views := layout.Rows(grid.NewSurface(20, 10).View(),
		layout.Slot{Size: layout.Sizing{Flex: 1, Min: 8}},
		layout.Slot{Size: layout.Sizing{Flex: 1, Min: 8}},
	)
	total := 0
	for i, v := range views {
		_, h := v.Size()
		if visible := v.Visible().Dy(); h != visible {
			t.Errorf("slot %d was laid out into %d rows but only %d are on screen", i, h, visible)
		}
		total += h
	}
	if total != 10 {
		t.Fatalf("slots add up to %d rows, want the 10 there are", total)
	}
}

func TestASlotSqueezedToNothingStillGetsAView(t *testing.T) {
	// A caller's draw code runs every frame. Code that only breaks when it has no
	// room breaks in front of the user.
	views := layout.Rows(grid.NewSurface(10, 1).View(),
		layout.Slot{Size: layout.Fixed(1)},
		layout.Slot{Size: layout.Fixed(5)},
	)
	if len(views) != 2 {
		t.Fatalf("got %d views, want one per slot", len(views))
	}
	if _, h := views[1].Size(); h != 0 {
		t.Fatalf("the squeezed slot got %d rows, want none", h)
	}
}

func TestDivideAnswersWithoutViews(t *testing.T) {
	// A caller aligning a header over a table needs the numbers and not the views.
	got := layout.Divide(10, 4, []layout.Slot{{Size: layout.Fixed(3)}, {Size: layout.Flex(1)}})
	equal(t, got, []int{3, 7})
}

func TestAlignPlacesContentInASpaceWiderThanItself(t *testing.T) {
	for _, tc := range []struct {
		align layout.Align
		want  int
	}{
		{layout.Start, 0},
		{layout.Center, 3},
		{layout.End, 6},
	} {
		if got := tc.align.Offset(10, 4); got != tc.want {
			t.Errorf("align %d: offset = %d, want %d", tc.align, got, tc.want)
		}
	}
}

func TestAlignNeverPlacesContentOutsideTheSpace(t *testing.T) {
	for _, align := range []layout.Align{layout.Start, layout.Center, layout.End} {
		if got := align.Offset(4, 10); got != 0 {
			t.Errorf("align %d: content wider than its space starts at %d, want 0", align, got)
		}
	}
}

func TestInsetReportsWhatItTakesAndWhatItLeaves(t *testing.T) {
	in := layout.Symmetric(1, 2)
	if got := in.Size(); got != (layout.Size{W: 4, H: 2}) {
		t.Fatalf("size = %+v, want 4 columns and 2 rows", got)
	}
	left := in.Apply(grid.Rect(0, 0, 10, 6))
	if left != grid.Rect(2, 1, 6, 4) {
		t.Fatalf("inner = %v, want the region less the inset", left)
	}
}

func TestAnInsetBiggerThanItsRegionLeavesNothing(t *testing.T) {
	if left := layout.Uniform(5).Apply(grid.Rect(0, 0, 4, 4)); !left.Empty() {
		t.Fatalf("inner = %v, want nothing left", left)
	}
}

func TestPlacementAnchors(t *testing.T) {
	space := layout.Size{W: 10, H: 6}
	for _, tc := range []struct {
		anchor layout.Anchor
		wantX  int
		wantY  int
	}{
		{layout.TopLeft, 0, 0},
		{layout.Top, 3, 0},
		{layout.TopRight, 6, 0},
		{layout.Left, 0, 2},
		{layout.Middle, 3, 2},
		{layout.Right, 6, 2},
		{layout.BottomLeft, 0, 4},
		{layout.Bottom, 3, 4},
		{layout.BottomRight, 6, 4},
	} {
		got := layout.Placement{Anchor: tc.anchor, Width: 4, Height: 2}.In(space)
		if got.Min.X != tc.wantX || got.Min.Y != tc.wantY {
			t.Errorf("anchor %d: at (%d,%d), want (%d,%d)",
				tc.anchor, got.Min.X, got.Min.Y, tc.wantX, tc.wantY)
		}
	}
}

func TestPlacementIsClampedToTheSpaceItFloatsOver(t *testing.T) {
	// A dialog whose buttons are past the right margin is a dialog nobody can answer.
	got := layout.Placement{Anchor: layout.Middle, Width: 100, Height: 100}.In(layout.Size{W: 10, H: 6})
	if got != grid.Rect(0, 0, 10, 6) {
		t.Fatalf("area = %v, want it clamped to the space", got)
	}
}

func TestPlacementWithNoSizeFillsWhatTheMarginLeaves(t *testing.T) {
	got := layout.Placement{Margin: 1}.In(layout.Size{W: 10, H: 6})
	if got != grid.Rect(1, 1, 8, 4) {
		t.Fatalf("area = %v, want the space less the margin", got)
	}
}

func TestPlacementWithNowhereToGo(t *testing.T) {
	if got := (layout.Placement{Margin: 10}).In(layout.Size{W: 4, H: 4}); !got.Empty() {
		t.Fatalf("area = %v, want nothing", got)
	}
}
