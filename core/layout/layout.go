// Package layout divides a region among the things that go in it.
//
// It is geometry and nothing else: it decides where boxes go and hands back the
// views to draw them in, and it never draws. That is what lets the same rules place
// a widget, a string, or a hole left deliberately empty — and what keeps the sizing
// rules testable by asking for numbers rather than by reading a screen.
//
// # Measuring
//
// A slot whose size follows from its content says so with [Measured] and supplies a
// [Measurer]. The measurer is asked about the axis being divided, given how much
// room there is across the other one: a row of text asked how tall it is at a width,
// a column of labels asked how wide it is at a height. One question, either axis,
// which is why [Measured] means the same thing in [Rows] and in [Columns].
package layout

import (
	"image"

	"github.com/Tangerg/oolong/core/grid"
)

// Size is a width and a height in cells.
type Size struct{ W, H int }

// Align is how content sits in a space wider than itself.
type Align uint8

const (
	Start Align = iota
	Center
	End
)

// Offset is where content of the given width starts inside space columns.
func (a Align) Offset(space, width int) int {
	switch a {
	case Center:
		return max((space-width)/2, 0)
	case End:
		return max(space-width, 0)
	default:
		return 0
	}
}

// Inset is space held clear on each side.
type Inset struct{ Top, Right, Bottom, Left int }

// Uniform is the same inset on every side.
func Uniform(n int) Inset { return Inset{Top: n, Right: n, Bottom: n, Left: n} }

// Symmetric is one inset above and below, another to the left and right — the
// common case, because a terminal cell is about twice as tall as it is wide and
// even padding does not look even.
func Symmetric(vertical, horizontal int) Inset {
	return Inset{Top: vertical, Right: horizontal, Bottom: vertical, Left: horizontal}
}

// Size is how many columns and rows the inset takes.
func (i Inset) Size() Size {
	return Size{W: i.Left + i.Right, H: i.Top + i.Bottom}
}

// Apply is what is left of r after the inset is held clear, and nothing at all when
// the inset is larger than the region.
//
// The rectangle is built by hand rather than with [image.Rect], which puts a
// backwards rectangle the right way round: an inset that overran its region would
// come back as a real region somewhere else instead of as no region at all.
func (i Inset) Apply(r image.Rectangle) image.Rectangle {
	out := image.Rectangle{
		Min: image.Pt(r.Min.X+i.Left, r.Min.Y+i.Top),
		Max: image.Pt(r.Max.X-i.Right, r.Max.Y-i.Bottom),
	}
	if out.Min.X >= out.Max.X || out.Min.Y >= out.Max.Y {
		return image.Rectangle{}
	}
	return out
}

// Measurer reports how much of one axis something wants, given how much room it has
// across the other.
//
// Which axis is which is decided by whoever is asking: [Rows] divides height and
// asks for a height at a width, [Columns] divides width and asks for a width at a
// height. A type that can only answer for one axis is a type that belongs in only
// one of them, and saying so is the caller's business rather than this package's.
type Measurer interface {
	Measure(across int) int
}

// MeasureFunc adapts a function to [Measurer].
type MeasureFunc func(across int) int

func (f MeasureFunc) Measure(across int) int { return f(across) }

// Sizing says how much of an axis a slot wants.
type Sizing struct {
	// Fixed is an exact number of rows or columns. It wins over everything else.
	Fixed int
	// Flex is a share of what is left after the fixed and measured slots have taken
	// theirs. Two slots with flex 1 and 2 split the remainder one third to two
	// thirds.
	Flex int
	// Measured asks the slot's [Measurer] how much it wants.
	Measured bool
	// Min is a floor on a flex or measured slot, so a pane cannot be squeezed into a
	// size where it shows nothing useful. It is honoured while there is room for it:
	// several floors can add up to more than the space there is.
	Min int
	// Max caps a measured slot, so content that grew without bound does not take the
	// whole region.
	Max int
}

// Fixed is a slot of an exact size.
func Fixed(n int) Sizing { return Sizing{Fixed: n} }

// Flex is a slot taking a share of what is left.
func Flex(share int) Sizing { return Sizing{Flex: share} }

// Measured is a slot as big as its [Measurer] asks to be, within bounds. A zero
// maximum means no cap.
func Measured(minimum, maximum int) Sizing {
	return Sizing{Measured: true, Min: minimum, Max: maximum}
}

// Slot is one division of a region: how much room it gets, and what to ask when
// that follows from its content.
type Slot struct {
	Size Sizing
	// Of is asked how much of the divided axis this slot wants, and is only
	// consulted when Size says the slot is measured. A measured slot with nothing to
	// ask gets its floor, which is zero unless one was set.
	Of Measurer
}

// Rows divides v into horizontal bands down the region and returns the view for
// each, in order.
//
// Nothing is drawn. The caller draws into the views it is given, which is what lets
// a slot be left empty, be drawn conditionally, or be measured now and drawn later.
//
// The order of business is measure, then arrange: the only order that works when
// one slot's size depends on its content and another's depends on what is left.
// Slots that end up with no height still get a view — an empty one — because a
// caller's draw code runs every frame, and code that only breaks when it is squeezed
// to nothing breaks in front of the user.
func Rows(v grid.View, slots ...Slot) []grid.View {
	width, height := v.Size()
	sizes := Divide(height, width, slots)

	views := make([]grid.View, len(slots))
	y := 0
	for i, size := range sizes {
		views[i] = v.Sub(grid.Rect(0, y, width, size))
		y += size
	}
	return views
}

// Columns is [Rows] across, for panes side by side.
func Columns(v grid.View, slots ...Slot) []grid.View {
	width, height := v.Size()
	sizes := Divide(width, height, slots)

	views := make([]grid.View, len(slots))
	x := 0
	for i, size := range sizes {
		views[i] = v.Sub(grid.Rect(x, 0, size, height))
		x += size
	}
	return views
}

// Divide splits total among slots, measuring against across, and returns each
// slot's size. The sizes always add up to at most total.
//
// It is exported because a caller aligning something to the same grid — a header
// over a table, a ruler beside a pane — needs the numbers without the views.
func Divide(total, across int, slots []Slot) []int {
	sizes := make([]int, len(slots))
	left := max(total, 0)
	flex := 0

	// Fixed and measured slots take theirs first: both are stating a need, and the
	// flexible ones exist to absorb whatever is left over.
	for i, slot := range slots {
		switch {
		case slot.Size.Fixed > 0:
			sizes[i] = min(slot.Size.Fixed, left)
		case slot.Size.Measured:
			want := slot.Size.Min
			if slot.Of != nil {
				want = max(slot.Of.Measure(across), slot.Size.Min)
			}
			if slot.Size.Max > 0 {
				want = min(want, slot.Size.Max)
			}
			sizes[i] = min(max(want, 0), left)
		default:
			flex += max(slot.Size.Flex, 0)
			continue
		}
		left -= sizes[i]
	}
	if flex == 0 {
		return sizes
	}

	// Shares of what is left, with each slot's floor honoured only while there is
	// room for it. Several floors can add up to more than the space there is, and a
	// layout that handed them out anyway would tell a widget it had a size the view
	// then clipped — which the widget cannot see, and lays out against.
	remainder := left
	lastFlex := -1
	for i, slot := range slots {
		share := max(slot.Size.Flex, 0)
		if share == 0 {
			continue
		}
		want := max(remainder*share/flex, slot.Size.Min)
		sizes[i] = min(want, left)
		left -= sizes[i]
		lastFlex = i
	}
	// The rounding remainder goes to the last flexible slot rather than being lost:
	// a row that vanished would leave a gap the user can see.
	if lastFlex >= 0 && left > 0 {
		sizes[lastFlex] += left
	}
	return sizes
}
