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
//
// # The other axis, and the room between
//
// Dividing an axis leaves two questions it cannot ask, and both were being answered
// by hand above this package before they were answered here. [Flow] is an axis with
// a gap between the things it divides. [Slot.Cross] is where a slot's content sits
// when it is narrower than the slot — a centred row of buttons, a hint row against
// the right edge.
//
// This is not the beginning of a flexbox. Deeply nested layout is refused on
// purpose, and a caller who wants it will outgrow this: what is here is the three
// things a terminal interface asks for often enough that everyone writes them again.
package layout

import (
	"image"

	"github.com/Tangerg/oolong/core/grid"
)

// Size is a width and a height in cells.
type Size struct{ W, H int }

// Align is how content sits in a space wider than itself.
type Align uint8

// Where content sits when it is narrower than its space.
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

// Measure calls f.
func (f MeasureFunc) Measure(across int) int { return f(across) }

// Sizing says how much of an axis a slot wants.
type Sizing struct {
	// Fixed is an exact number of rows or columns. It wins over everything else.
	Fixed int
	// Part and Whole are a share of the whole division: Part 1 of Whole 2 is half of
	// it, whatever else is being divided and whatever those others ask for. The whole
	// is what there is to divide, which is the region less the gaps in it.
	//
	// It is not [Flex] and cannot be written as one. A share of what is left changes
	// when anything beside it changes, which is what makes it right for panes that
	// divide the slack and wrong for "this pane is half the screen" — a sentence a
	// caller means literally, and one whose answer must not move when a status bar
	// appears above it.
	Part, Whole int
	// Flex is a share of what is left after the fixed, measured and part slots have
	// taken theirs. Two slots with flex 1 and 2 split the remainder one third to two
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

// Part is a slot taking a fraction of the whole division: Part(1, 2) is half of it,
// whatever else is there. A whole of zero asks for nothing, which is what makes the
// zero [Sizing] mean what it always did.
func Part(part, whole int) Sizing { return Sizing{Part: part, Whole: whole} }

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
	// Cross is where the slot's content sits across the other axis. The zero value
	// fills it, which is what a band across a pane means and what every slot did
	// before there was a way to say otherwise.
	Cross Cross
}

// Cross is how much of the other axis a slot's content takes, and where in the slot
// it sits when that is less than all of it.
//
// It is the answer to the one question dividing an axis cannot ask: a row of buttons
// centred in a dialog, a hint row against the right edge, a title over a pane that is
// wider than the title. Without it a caller has to take the view it was given and
// narrow it by hand, which is arithmetic every caller writes and one of them gets
// wrong.
//
// The size is a number rather than a [Measurer] on purpose. A widget answers about
// one axis — [Rows] asks how tall at a width, [Columns] how wide at a height — and a
// slot that asked the other way round would be asking most widgets a question they
// cannot answer.
type Cross struct {
	// Size is how many cells across the other axis the content takes. Zero, and
	// anything larger than the region, is all of it.
	Size int
	// Align is where it sits when it is less than all of it.
	Align Align
}

// place narrows r to the slot's cross size and puts it where the alignment says.
// across is the whole of the other axis, and a is the axis being divided.
func (c Cross) place(r image.Rectangle, a Axis, across int) image.Rectangle {
	if c.Size <= 0 || c.Size >= across {
		return r
	}
	at := c.Align.Offset(across, c.Size)
	if a == Across {
		return image.Rect(r.Min.X, r.Min.Y+at, r.Max.X, r.Min.Y+at+c.Size)
	}
	return image.Rect(r.Min.X+at, r.Min.Y, r.Min.X+at+c.Size, r.Max.Y)
}

// Axis is which way a region is divided.
//
// It exists so that something arranging its contents can be told which way round it
// goes instead of being written twice. [Rows] and [Columns] are the two values of it
// under the names a caller usually wants.
type Axis uint8

const (
	// Down stacks bands one above another, dividing height.
	Down Axis = iota
	// Across puts panes side by side, dividing width.
	Across
)

// Flow is an axis with room between the things it divides.
//
// The gap is here rather than in [Slot] because it is one answer for the whole
// division: a caller says "these, with a column between them" once, instead of
// padding every slot but the last and getting the last one wrong. [Rows] and
// [Columns] are this with no gap, which is why they are still the ordinary call.
type Flow struct {
	Axis Axis
	// Gap is how many cells go between one slot and the next.
	//
	// It is reserved for every join, including the ones beside a slot that ended up
	// with no room. A gap that appeared and disappeared with its neighbour's contents
	// would move every column after it whenever a value happened to be empty, and a
	// table whose columns shift as its rows change is worse than one with a wider
	// margin than it needed.
	Gap int
}

// Rects is where each slot goes when a space is divided, in the space's own
// coordinates.
func (f Flow) Rects(space Size, slots []Slot) []image.Rectangle {
	total, across := space.H, space.W
	if f.Axis == Across {
		total, across = space.W, space.H
	}
	sizes := f.Divide(total, across, slots)

	rects := make([]image.Rectangle, len(slots))
	at := 0
	for i, size := range sizes {
		var r image.Rectangle
		if f.Axis == Across {
			r = grid.Rect(at, 0, size, space.H)
		} else {
			r = grid.Rect(0, at, space.W, size)
		}
		rects[i] = slots[i].Cross.place(r, f.Axis, across)
		at += size + f.Gap
	}
	return rects
}

// Views divides v and returns the view for each slot, in order.
func (f Flow) Views(v grid.View, slots ...Slot) []grid.View {
	width, height := v.Size()
	rects := f.Rects(Size{W: width, H: height}, slots)

	views := make([]grid.View, len(rects))
	for i, r := range rects {
		views[i] = v.Sub(r)
	}
	return views
}

// Divide splits total among the slots, holding back the gaps between them first.
func (f Flow) Divide(total, across int, slots []Slot) []int {
	return Divide(total-f.gaps(len(slots)), across, slots)
}

// Wanted is how much of the divided axis the slots ask for altogether, the gaps
// between them included.
func (f Flow) Wanted(across int, slots []Slot) int {
	return Wanted(across, slots) + f.gaps(len(slots))
}

// gaps is how much of the axis the joins take.
func (f Flow) gaps(slots int) int {
	if f.Gap <= 0 || slots < 2 {
		return 0
	}
	return f.Gap * (slots - 1)
}

// Rects is where each slot goes when a space is divided along the axis, in the
// space's own coordinates.
//
// It is the geometry on its own, without a view to draw into, because working out
// where something went and drawing it there happen at different times: a click
// arrives between two frames and has to be answered against the frame that is on
// screen. Anything routing input by position asks this and keeps the answer.
//
// The order of business is measure, then arrange: the only order that works when one
// slot's size depends on its content and another's depends on what is left. Slots
// that end up with no room still get a rectangle — an empty one — because a caller's
// code runs every frame, and code that only breaks when it is squeezed to nothing
// breaks in front of the user.
func (a Axis) Rects(space Size, slots []Slot) []image.Rectangle {
	return Flow{Axis: a}.Rects(space, slots)
}

// Views divides v along the axis and returns the view for each slot, in order.
//
// Nothing is drawn. The caller draws into the views it is given, which is what lets
// a slot be left empty, be drawn conditionally, or be measured now and drawn later.
func (a Axis) Views(v grid.View, slots ...Slot) []grid.View {
	return Flow{Axis: a}.Views(v, slots...)
}

// Rows divides v into horizontal bands down the region and returns the view for
// each, in order.
func Rows(v grid.View, slots ...Slot) []grid.View { return Down.Views(v, slots...) }

// Columns is [Rows] across, for panes side by side.
func Columns(v grid.View, slots ...Slot) []grid.View { return Across.Views(v, slots...) }

// Wanted is how much of the divided axis a set of slots asks for altogether,
// measured against across.
//
// It is what something made of slots answers when it is itself in a measured slot: a
// column of widgets inside a pane that grows to fit its contents. A flexible slot has
// nothing to ask for — a share is a share of a total, and there is no total yet — so
// it counts as its floor.
func Wanted(across int, slots []Slot) int {
	total := 0
	for _, slot := range slots {
		switch {
		case slot.Size.Fixed > 0:
			total += slot.Size.Fixed
		case slot.Size.Whole > 0:
			// A fraction of a region nobody has named yet. There is no total to take a
			// part of, so it counts as its floor — the same answer a flexible slot
			// gives, and for the same reason.
			total += max(slot.Size.Min, 0)
		case slot.Size.Measured:
			want := slot.Size.Min
			if slot.Of != nil {
				want = max(slot.Of.Measure(across), slot.Size.Min)
			}
			if slot.Size.Max > 0 {
				want = min(want, slot.Size.Max)
			}
			total += max(want, 0)
		default:
			total += max(slot.Size.Min, 0)
		}
	}
	return total
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
		case slot.Size.Whole > 0:
			// Of the whole region rather than of what is left, which is the whole
			// difference between this and a share: it is worked out from the total the
			// division began with, so nothing else in the region can move it.
			want := max(total, 0) * max(slot.Size.Part, 0) / slot.Size.Whole
			sizes[i] = min(max(want, slot.Size.Min), left)
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
