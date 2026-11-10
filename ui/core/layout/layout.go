// Package layout owns one-dimensional rectangle allocation. [Slot.Size] chooses
// fixed, flexible, fractional or measured extent; [Axis] selects rows or columns.
// [Measurer] receives the cross-axis extent, keeping one sizing rule for both axes.
// Callers compose rectangles for nesting; layout owns no widget tree or wrapping.
// Geometry operations panic for invalid enum values even with empty geometry,
// before measurers run, so resizing cannot hide malformed configuration.
// Validate methods support error-returning boundaries.
package layout

import (
	"errors"
	"fmt"
	"image"
	"math/bits"
	"slices"
)

type Align uint8

const (
	Start Align = iota
	Center
	End
)

var ErrInvalidAlign = errors.New("layout: invalid alignment")

func (a Align) Validate() error {
	if a > End {
		return fmt.Errorf("%w: %d", ErrInvalidAlign, a)
	}
	return nil
}

// Offset panics for invalid alignment even without available space, so collapsing
// geometry cannot hide malformed configuration.
func (a Align) Offset(space, extent int) int {
	if err := a.Validate(); err != nil {
		panic(err)
	}
	switch a {
	case Center:
		return Remaining(space, extent) / 2
	case End:
		return Remaining(space, extent)
	default:
		return 0
	}
}

type Inset struct{ Top, Right, Bottom, Left int }

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

func (i Inset) Size() image.Point {
	i = i.normalized()
	return image.Pt(Sum(i.Left, i.Right), Sum(i.Top, i.Bottom))
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

// Measurer reports the desired extent along axis given the available cross extent.
// Down asks for height at a width; Across asks for width at a height. Implementations
// must interpret the explicit axis, so height-only content cannot satisfy this seam.
type Measurer interface {
	Measure(axis Axis, across int) int
}

type MeasureFunc func(axis Axis, across int) int

// Measure panics for an invalid axis before invoking f, so the callback only
// receives a supported dimension.
func (f MeasureFunc) Measure(axis Axis, across int) int {
	if err := axis.Validate(); err != nil {
		panic(err)
	}
	return f(axis, across)
}

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

// IsZero reports whether s names no sizing policy. It lets a higher-level
// container apply its own useful default without inspecting this value's private
// representation or mistaking an explicit Fixed(0) for omission.
func (s Sizing) IsZero() bool { return s.kind == zeroSizing }

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
// maximum means no cap. Negative bounds are normalized to zero.
//
// A non-zero maximum below the minimum is a programmer error and panics here. The
// two bounds are a contradiction the caller wrote, and clamping either of them would
// hand back a slot sized by neither, in a division whose other slots then absorb the
// difference somewhere the caller never looks.
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
//
// That error panics, as does a floor above a measured slot's non-zero maximum.
// Both are contradictions in one slot's own description, and a Sizing is passed to
// [Flow.Rects] far from where it was built — a silently corrected one would surface
// as a neighbouring slot being the wrong size, which is the wrong place to look.
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

type Slot struct {
	Size Sizing
	// Of is asked how much of the divided axis a measured slot wants, and how much
	// of the cross axis any slot wants when the flow measures its own extent. A
	// measured slot with nothing to ask gets its floor, which is zero unless one was
	// set.
	Of Measurer
	// Cross is where the slot's item sits across the other axis. The zero value
	// fills it.
	Cross Cross
}

// Cross supplies the extent the divided axis cannot determine. Size is explicit
// because an axis-specific Measurer may not support the perpendicular question.
type Cross struct {
	// Size is how many units across the other axis the item takes. Zero, and
	// anything larger than the region, is all of it.
	Size  int
	Align Align
}

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

// Axis lets the same sizing rules serve rows and columns.
type Axis uint8

const (
	Down Axis = iota
	Across
)

var ErrInvalidAxis = errors.New("layout: invalid axis")

func (a Axis) Validate() error {
	if a > Across {
		return fmt.Errorf("%w: %d", ErrInvalidAxis, a)
	}
	return nil
}

// Flow is an axis with room between the things it divides.
//
// The gap is here rather than in [Slot] because it is one answer for the whole
// division: a caller specifies the spacing once, instead of padding every slot but
// the last and getting the last one wrong. A zero gap is the ordinary contiguous
// arrangement; it uses this same value and the same operations.
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
// An invalid Axis or any slot's Cross.Align panics before any measurer runs,
// including for empty geometry, so a rejected candidate has no measurement effects.
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
		at = min(Sum(at, size), total)
		at += min(gap, total-at)
	}
	return rects
}

// Divide panics for an invalid Axis or any slot's Cross.Align before any measurer
// runs, including for empty geometry, so a rejected candidate has no measurement effects.
func (f Flow) Divide(total, across int, slots []Slot) []int {
	if err := f.validate(slots); err != nil {
		panic(err)
	}
	available := max(total, 0)
	return divide(f.Axis, Remaining(available, f.gaps(len(slots))), across, slots)
}

// Wanted is how much of the divided axis the slots ask for altogether, the gaps
// between them included.
// An invalid Axis or any slot's Cross.Align panics before any measurer runs,
// including for empty geometry, so a rejected candidate has no measurement effects.
func (f Flow) Wanted(across int, slots []Slot) int {
	if err := f.validate(slots); err != nil {
		panic(err)
	}
	wanted, gaps := wanted(f.Axis, across, slots), f.gaps(len(slots))
	return Sum(wanted, gaps)
}

// CrossWanted measures the perpendicular extent of a flow with a known
// main-axis total. Fixed and proportional requests can be divided immediately.
// A measured request that itself depends on the unknown cross extent gets the
// largest main extent it can receive, respecting its cap and preceding floors.
// An application with mutually dependent constraints can supply its own measure.
// An invalid Axis or any slot's Cross.Align panics before any measurer runs,
// including for empty geometry, so a rejected candidate has no measurement effects.
func (f Flow) CrossWanted(total int, slots []Slot) int {
	requests := slices.Clone(slots)
	for i := range requests {
		requests[i].Of = nil
	}
	sizes := f.Divide(total, 0, requests)
	axis := Across
	if f.Axis == Across {
		axis = Down
	}
	remaining := Remaining(total, f.gaps(len(slots)))
	extent := 0
	for i, slot := range slots {
		available := sizes[i]
		if slot.Size.kind == measuredSizing {
			available = remaining
			if slot.Size.maximum > 0 {
				available = min(available, slot.Size.maximum)
			}
		}
		if slot.Size.kind != flexSizing {
			remaining = Remaining(remaining, sizes[i])
		}
		if available > 0 {
			if slot.Cross.Size > 0 {
				extent = max(extent, slot.Cross.Size)
			} else if slot.Of != nil {
				extent = max(extent, slot.Of.Measure(axis, available))
			}
		}
	}
	return extent
}

func (f Flow) validate(slots []Slot) error {
	if err := f.Axis.Validate(); err != nil {
		return err
	}
	for i, slot := range slots {
		if err := slot.Cross.Align.Validate(); err != nil {
			return fmt.Errorf("layout: slot %d: %w", i, err)
		}
	}
	return nil
}

func (f Flow) gaps(slots int) int {
	if f.Gap <= 0 || slots < 2 {
		return 0
	}
	if f.Gap > maxInt/(slots-1) {
		return maxInt
	}
	return f.Gap * (slots - 1)
}

// wanted is how much of the divided axis a set of slots asks for altogether,
// measured against across.
//
// It is what a group made of slots answers when it is itself measured. A flexible slot has
// nothing to ask for — a share is a share of a total, and there is no total yet — so
// it counts as its floor.
func wanted(axis Axis, across int, slots []Slot) int {
	across = max(across, 0)
	total := 0
	for _, slot := range slots {
		total = Sum(total, slot.wanted(axis, across))
	}
	return total
}

// wanted is what a slot can ask for before the divided extent exists. A fraction
// and a flexible share therefore ask only for their floor; both need a named whole
// before their proportional part has meaning.
func (s Slot) wanted(axis Axis, across int) int {
	switch s.Size.kind {
	case fixedSizing:
		return s.Size.amount
	case partSizing, flexSizing:
		return s.Size.minimum
	case measuredSizing:
		want := s.Size.minimum
		if s.Of != nil {
			if s.Cross.Size > 0 {
				across = min(across, s.Cross.Size)
			}
			want = max(s.Of.Measure(axis, across), s.Size.minimum)
		}
		if s.Size.maximum > 0 {
			want = min(want, s.Size.maximum)
		}
		return want
	default:
		return 0
	}
}

// divide splits total among slots, measuring against across, and returns each slot's
// size. The sizes always add up to at most total.
func divide(axis Axis, total, across int, slots []Slot) []int {
	d := division{
		axis:    axis,
		total:   max(total, 0),
		across:  max(across, 0),
		slots:   slots,
		sizes:   make([]int, len(slots)),
		maxFlex: maxInt / max(len(slots), 1),
	}
	d.left = d.total
	d.reserve()
	d.distribute()
	return d.sizes
}

// division owns one allocation in progress. Keeping its remaining room and weight
// sum together makes it impossible for the rigid and flexible passes to update only
// half of the same state.
type division struct {
	axis          Axis
	total, across int
	left, flex    int
	maxFlex       int
	slots         []Slot
	sizes         []int
}

func (d *division) reserve() {
	for i, slot := range d.slots {
		switch slot.Size.kind {
		case flexSizing:
			d.flex += d.share(slot)
		case partSizing:
			// A part is of the whole region rather than of what is left, which is the
			// difference between it and a flexible share.
			want := Scale(d.total, slot.Size.amount, slot.Size.whole)
			d.allocate(i, max(want, slot.Size.minimum))
		default:
			d.allocate(i, slot.wanted(d.axis, d.across))
		}
	}
}

// distribute gives flexible slots shares of the same remainder.
//
// A floor is a minimum, not a share. Slots whose weight cannot reach their own
// minimum are settled at it first and leave the rest to share what remains; sharing
// first and clamping to whatever happened to be left made an explicit minimum depend
// on where its slot sat in the ring, so a slot earlier in it could spend room a later
// one had already asked for by name. Flex(0).AtLeast(n) asks for no proportional room
// and still gets its n.
func (d *division) distribute() {
	settled, room, weight := d.pinFloors()
	// Integer division may leave a remainder. The last weighted slot owns it; a
	// floor-only slot must not absorb room it never asked to share.
	if last := d.allocateFlex(settled, room, weight); last >= 0 && d.left > 0 {
		d.sizes[last] += d.left
		d.left = 0
	}
}

// pinFloors settles the flexible slots whose share would fall below the floor they
// stated, taking that floor out of the room the rest divide. Pinning one can push
// another below its own floor, so it repeats until nothing moves.
func (d *division) pinFloors() (settled []bool, room, weight int) {
	settled = make([]bool, len(d.slots))
	room, weight = d.left, d.flex
	for pinning := true; pinning; {
		pinning = false
		for i, slot := range d.slots {
			if slot.Size.kind != flexSizing || settled[i] {
				continue
			}
			share := d.share(slot)
			want := 0
			if weight > 0 {
				want = Scale(room, share, weight)
			}
			if want >= slot.Size.minimum {
				continue
			}
			settled[i], pinning = true, true
			room, weight = Remaining(room, slot.Size.minimum), weight-share
		}
	}
	return settled, room, weight
}

func (d *division) allocateFlex(settled []bool, room, weight int) int {
	lastWeighted := -1
	for i, slot := range d.slots {
		if slot.Size.kind != flexSizing {
			continue
		}
		share := d.share(slot)
		want := slot.Size.minimum
		if !settled[i] && weight > 0 {
			want = Scale(room, share, weight)
		}
		d.allocate(i, want)
		if share > 0 && !settled[i] {
			lastWeighted = i
		}
	}
	return lastWeighted
}

func (d *division) share(slot Slot) int { return min(slot.Size.amount, d.maxFlex) }

func (d *division) allocate(index, want int) {
	d.sizes[index] = min(max(want, 0), d.left)
	d.left -= d.sizes[index]
}

const (
	maxInt = int(^uint(0) >> 1)
	minInt = -maxInt - 1
)

// Translate moves a signed coordinate by delta, saturating at the integer limits.
// It is distinct from [Sum]: coordinates may legitimately be negative, while Sum
// combines non-negative extents and treats a negative input as no extent.
func Translate(at, delta int) int {
	switch {
	case delta > 0 && at > maxInt-delta:
		return maxInt
	case delta < 0 && at < minInt-delta:
		return minInt
	default:
		return at + delta
	}
}

// Relative projects an absolute coordinate into a space whose origin is origin,
// saturating when the mathematical difference is outside the integer range.
func Relative(at, origin int) int {
	switch {
	case origin > 0 && at < minInt+origin:
		return minInt
	case origin < 0 && at > maxInt+origin:
		return maxInt
	default:
		return at - origin
	}
}

// Sum adds non-negative extents, saturating at the largest int instead of letting
// overflow turn geometry negative. Negative inputs describe no extent and contribute
// zero. It is the one addition rule for both layout internals and composed measurers.
func Sum(extents ...int) int {
	total := 0
	for _, extent := range extents {
		extent = max(extent, 0)
		if extent > maxInt-total {
			return maxInt
		}
		total += extent
	}
	return total
}

// Remaining subtracts non-negative extents from total without underflow. Negative
// inputs describe no room or no use. It is the subtraction counterpart to [Sum] and
// the one rule for asking how much of a measured axis is left.
func Remaining(total int, used ...int) int {
	total = max(total, 0)
	for _, extent := range used {
		extent = max(extent, 0)
		if extent >= total {
			return 0
		}
		total -= extent
	}
	return total
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
