// Package layout divides a region among rectangular items.
//
// It is geometry and nothing else: it allocates rectangles without knowing how
// callers will use them. That keeps the same rules useful in any coordinate model
// and makes sizing testable as arithmetic.
//
// # Measuring
//
// A slot whose size follows from its item says so with [Measured] and supplies a
// [Measurer]. The measurer is asked about the axis being divided, given how much
// room there is across the other one: an item asked for one dimension given the
// available other dimension. One question, either axis,
// which is why [Measured] means the same thing for [Down] and [Across].
//
// # The other axis, and the room between
//
// Dividing an axis leaves two questions it cannot ask, and both were being answered
// by hand above this package before they were answered here. [Flow] is an axis with
// a gap between the things it divides. [Slot.Cross] says where an item sits when it
// takes less than the available cross-axis extent.
//
// This is intentionally a small one-dimensional allocator, not the beginning of a
// flexbox implementation. It does not own a tree, wrapping or reflow: callers
// compose its rectangles when they need nesting, and callers needing a layout engine
// should use one above this package rather than making this allocator know their
// item lifecycle.
package layout

import (
	"image"
	"math/bits"
)

// Align is how an extent sits in a larger space.
type Align uint8

// Where an extent sits within its space.
const (
	Start Align = iota
	Center
	End
)

// Offset is where an extent starts inside a space of the given size.
func (a Align) Offset(space, extent int) int {
	space, extent = max(space, 0), max(extent, 0)
	switch a {
	case Center:
		return max((space-extent)/2, 0)
	case End:
		return max(space-extent, 0)
	default:
		return 0
	}
}

// Inset is space held clear on each side.
type Inset struct{ Top, Right, Bottom, Left int }

// Uniform is the same inset on every side.
func Uniform(n int) Inset {
	n = max(n, 0)
	return Inset{Top: n, Right: n, Bottom: n, Left: n}
}

// Symmetric is one inset above and below, and another to the left and right. The
// two axes are separate because equal coordinate extents need not occupy equal
// physical space; naming both pairs also avoids repeating four values at every
// call site.
func Symmetric(vertical, horizontal int) Inset {
	vertical, horizontal = max(vertical, 0), max(horizontal, 0)
	return Inset{Top: vertical, Right: horizontal, Bottom: vertical, Left: horizontal}
}

// Size is the horizontal and vertical extent the inset takes.
func (i Inset) Size() image.Point {
	i = i.normalized()
	return image.Pt(saturatingAdd(i.Left, i.Right), saturatingAdd(i.Top, i.Bottom))
}

// Apply is what is left of r after the inset is held clear, and nothing at all when
// the inset is larger than the region.
//
// The rectangle is built by hand rather than with [image.Rect], which puts a
// backwards rectangle the right way round: an inset that overran its region would
// come back as a real region somewhere else instead of as no region at all.
func (i Inset) Apply(r image.Rectangle) image.Rectangle {
	i = i.normalized()
	if r.Empty() || uint(i.Left)+uint(i.Right) >= uint(r.Max.X)-uint(r.Min.X) ||
		uint(i.Top)+uint(i.Bottom) >= uint(r.Max.Y)-uint(r.Min.Y) {
		return image.Rectangle{}
	}
	out := image.Rectangle{
		Min: image.Pt(r.Min.X+i.Left, r.Min.Y+i.Top),
		Max: image.Pt(r.Max.X-i.Right, r.Max.Y-i.Bottom),
	}
	return out
}

func (i Inset) normalized() Inset {
	i.Top = max(i.Top, 0)
	i.Right = max(i.Right, 0)
	i.Bottom = max(i.Bottom, 0)
	i.Left = max(i.Left, 0)
	return i
}

// Measurer reports how much of one axis something wants, given how much room it has
// across the other.
//
// Which axis is which is decided by whoever is asking: [Down] divides height and
// asks for a height at a width, [Across] divides width and asks for a width at a
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
//
// Its representation is private because fixed, fractional, flexible and measured
// are alternatives, not fields a caller should combine by priority. Construct one
// with [Fixed], [Part], [Flex] or [Measured]; use [Sizing.AtLeast] when a fractional,
// flexible or measured slot also has a floor. The zero value asks for no space.
type Sizing struct {
	kind             sizingKind
	amount, whole    int
	minimum, maximum int
}

type sizingKind uint8

const (
	zeroSizing sizingKind = iota
	fixedSizing
	partSizing
	flexSizing
	measuredSizing
)

// Fixed is a slot of an exact size. A negative size is normalized to zero.
func Fixed(n int) Sizing { return Sizing{kind: fixedSizing, amount: max(n, 0)} }

// Part is a slot taking a fraction of the whole division: Part(1, 2) is half of it,
// whatever else is there. A whole of zero asks for nothing, which is what makes the
// zero [Sizing] mean what it always did.
func Part(part, whole int) Sizing {
	return Sizing{kind: partSizing, amount: max(part, 0), whole: max(whole, 0)}
}

// Flex is a slot taking a share of what is left.
//
// A weight is a ratio and nothing else: doubling every weight in a division changes
// no result. Each one is therefore capped at an equal part of the largest
// representable total, which is what lets the weights be added in ordinary
// arithmetic with no case for the sum running past it. Weights above the cap
// saturate to it and become indistinguishable — which costs a caller nothing that a
// smaller pair of weights could not have said.
func Flex(share int) Sizing { return Sizing{kind: flexSizing, amount: max(share, 0)} }

// Measured is a slot as big as its [Measurer] asks to be, within bounds. A zero
// maximum means no cap.
func Measured(minimum, maximum int) Sizing {
	minimum, maximum = max(minimum, 0), max(maximum, 0)
	if maximum > 0 && maximum < minimum {
		panic("layout: measured maximum is below minimum")
	}
	return Sizing{
		kind: measuredSizing, minimum: minimum, maximum: maximum,
	}
}

// AtLeast returns s with a non-negative floor. Floors compose with fractional,
// flexible and measured sizing. Applying one to a fixed or zero sizing is a
// programmer error: the former is already exact, and the latter names no sizing
// policy to constrain.
func (s Sizing) AtLeast(minimum int) Sizing {
	minimum = max(minimum, 0)
	switch s.kind {
	case partSizing, flexSizing:
		s.minimum = minimum
	case measuredSizing:
		if s.maximum > 0 && s.maximum < minimum {
			panic("layout: minimum is above measured maximum")
		}
		s.minimum = minimum
	default:
		panic("layout: minimum requires part, flex, or measured sizing")
	}
	return s
}

// Slot is one division of a region: how much room it gets, and what to ask when
// that follows from its item.
type Slot struct {
	Size Sizing
	// Of is asked how much of the divided axis this slot wants, and is only
	// consulted when Size says the slot is measured. A measured slot with nothing to
	// ask gets its floor, which is zero unless one was set.
	Of Measurer
	// Cross is where the slot's item sits across the other axis. The zero value
	// fills it.
	Cross Cross
}

// Cross is how much of the other axis a slot's item takes, and where in the slot
// it sits when that is less than all of it.
//
// It is the answer to the one question dividing an axis cannot ask: how a smaller
// cross-axis extent is aligned within the rectangle allocated to its slot.
//
// The size is a number rather than a [Measurer] on purpose. An item answers about
// one axis — [Down] asks how tall at a width, [Across] how wide at a height — and a
// slot that asked the other way round would be asking most values a question they
// cannot answer.
type Cross struct {
	// Size is how many units across the other axis the item takes. Zero, and
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
// goes instead of being written twice. [Down] and [Across] are its two values.
type Axis uint8

const (
	// Down stacks bands one above another, dividing height.
	Down Axis = iota
	// Across puts regions side by side, dividing width.
	Across
)

// Flow is an axis with room between the things it divides.
//
// The gap is here rather than in [Slot] because it is one answer for the whole
// division: a caller specifies the spacing once, instead of padding every slot but
// the last and getting the last one wrong. Calling
// [Axis.Rects] is the same arrangement with no gap.
type Flow struct {
	Axis Axis
	// Gap is how many units go between one slot and the next.
	//
	// It is reserved for every join, including the ones beside a slot that ended up
	// with no room. A gap that appeared and disappeared with its neighbour's contents
	// would move every following slot whenever a value happened to be empty.
	Gap int
}

// Rects is where each slot goes when a space is divided, in the space's own
// coordinates.
func (f Flow) Rects(space image.Point, slots []Slot) []image.Rectangle {
	space.X, space.Y = max(space.X, 0), max(space.Y, 0)
	total, across := space.Y, space.X
	if f.Axis == Across {
		total, across = space.X, space.Y
	}
	sizes := f.Divide(total, across, slots)

	rects := make([]image.Rectangle, len(slots))
	at := 0
	gap := max(f.Gap, 0)
	for i, size := range sizes {
		var r image.Rectangle
		if f.Axis == Across {
			r = image.Rect(at, 0, at+size, space.Y)
		} else {
			r = image.Rect(0, at, space.X, at+size)
		}
		rects[i] = slots[i].Cross.place(r, f.Axis, across)
		at = min(saturatingAdd(at, size), total)
		at += min(gap, total-at)
	}
	return rects
}

// Divide splits total among the slots, holding back the gaps between them first.
func (f Flow) Divide(total, across int, slots []Slot) []int {
	available := max(total, 0)
	return Divide(max(available-f.gaps(len(slots)), 0), across, slots)
}

// Wanted is how much of the divided axis the slots ask for altogether, the gaps
// between them included.
func (f Flow) Wanted(across int, slots []Slot) int {
	wanted, gaps := Wanted(across, slots), f.gaps(len(slots))
	return saturatingAdd(wanted, gaps)
}

// gaps is how much of the axis the joins take.
func (f Flow) gaps(slots int) int {
	if f.Gap <= 0 || slots < 2 {
		return 0
	}
	if f.Gap > maxInt/(slots-1) {
		return maxInt
	}
	return f.Gap * (slots - 1)
}

// Rects is where each slot goes when a space is divided along the axis, in the
// space's own coordinates.
//
// The result is geometry only and can be projected into any coordinate model. Keeping
// allocation separate lets independent consumers share one answer instead of
// reconstructing it according to their own rules.
//
// The order of business is measure, then arrange: the only order that works when one
// slot's size depends on its item and another's depends on what is left. Slots
// that end up with no room still get a rectangle — an empty one — so result indexes
// always correspond to input slot indexes.
func (a Axis) Rects(space image.Point, slots ...Slot) []image.Rectangle {
	return Flow{Axis: a}.Rects(space, slots)
}

// Wanted is how much of the divided axis a set of slots asks for altogether,
// measured against across.
//
// It is what a group made of slots answers when it is itself measured. A flexible slot has
// nothing to ask for — a share is a share of a total, and there is no total yet — so
// it counts as its floor.
func Wanted(across int, slots []Slot) int {
	across = max(across, 0)
	total := 0
	for _, slot := range slots {
		switch slot.Size.kind {
		case fixedSizing:
			total = saturatingAdd(total, slot.Size.amount)
		case partSizing:
			// A fraction of a region nobody has named yet. There is no total to take a
			// part of, so it counts as its floor — the same answer a flexible slot
			// gives, and for the same reason.
			total = saturatingAdd(total, slot.Size.minimum)
		case measuredSizing:
			want := slot.Size.minimum
			if slot.Of != nil {
				want = max(slot.Of.Measure(across), slot.Size.minimum)
			}
			if slot.Size.maximum > 0 {
				want = min(want, slot.Size.maximum)
			}
			total = saturatingAdd(total, want)
		case flexSizing:
			total = saturatingAdd(total, slot.Size.minimum)
		case zeroSizing:
			continue
		}
	}
	return total
}

// Divide splits total among slots, measuring against across, and returns each
// slot's size. The sizes always add up to at most total.
//
// It is exported because related geometry may need the same allocation without
// constructing rectangles.
func Divide(total, across int, slots []Slot) []int {
	sizes := make([]int, len(slots))
	left := max(total, 0)
	across = max(across, 0)
	flex := 0
	// Every slot contributes at most this much, so the sum cannot overflow an int,
	// including on 32-bit architectures. The cap changes only ratios made from
	// weights too large to carry additional useful precision.
	maxFlex := maxInt / max(len(slots), 1)

	// Fixed and measured slots take theirs first: both are stating a need, and the
	// flexible ones exist to absorb whatever is left over.
	for i, slot := range slots {
		switch slot.Size.kind {
		case fixedSizing:
			sizes[i] = min(slot.Size.amount, left)
		case partSizing:
			// Of the whole region rather than of what is left, which is the whole
			// difference between this and a share: it is worked out from the total the
			// division began with, so nothing else in the region can move it.
			want := Scale(max(total, 0), slot.Size.amount, slot.Size.whole)
			sizes[i] = min(max(want, slot.Size.minimum), left)
		case measuredSizing:
			want := slot.Size.minimum
			if slot.Of != nil {
				want = max(slot.Of.Measure(across), slot.Size.minimum)
			}
			if slot.Size.maximum > 0 {
				want = min(want, slot.Size.maximum)
			}
			sizes[i] = min(want, left)
		case flexSizing:
			share := min(slot.Size.amount, maxFlex)
			flex += share
			continue
		case zeroSizing:
			continue
		}
		left -= sizes[i]
	}
	if flex == 0 {
		return sizes
	}

	// Shares of what is left, with each slot's floor honoured only while there is
	// room for it. Several floors can add up to more than the space there is, and a
	// layout that handed them out anyway would report more than it allocated, making
	// measurement and placement disagree.
	remainder := left
	lastFlex := -1
	for i, slot := range slots {
		if slot.Size.kind != flexSizing {
			continue
		}
		share := min(slot.Size.amount, maxFlex)
		if share == 0 {
			continue
		}
		want := Scale(remainder, share, flex)
		want = max(want, slot.Size.minimum)
		sizes[i] = min(want, left)
		left -= sizes[i]
		lastFlex = i
	}
	// The rounding remainder goes to the last flexible slot rather than being lost.
	if lastFlex >= 0 && left > 0 {
		sizes[lastFlex] += left
	}
	return sizes
}

const maxInt = int(^uint(0) >> 1)

// saturatingAdd adds non-negative extents without turning an overflow into a
// negative size.
func saturatingAdd(a, b int) int {
	a, b = max(a, 0), max(b, 0)
	if b > maxInt-a {
		return maxInt
	}
	return a + b
}

// Scale returns total*part/whole, capped to [0, total], without overflowing the
// intermediate product.
//
// It is the one proportional-coordinate operation shared by layout allocation and
// controls that map a bounded value onto an extent. Keeping it here gives both the
// same endpoint, saturation, and architecture-width semantics instead of two subtly
// different overflow workarounds.
func Scale(total, part, whole int) int {
	if total <= 0 || part <= 0 || whole <= 0 {
		return 0
	}
	if part >= whole {
		return total
	}
	hi, lo := bits.Mul64(uint64(total), uint64(part))
	quotient, _ := bits.Div64(hi, lo, uint64(whole))
	if quotient > uint64(maxInt) {
		return maxInt
	}
	return int(quotient)
}
