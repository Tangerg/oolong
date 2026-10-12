package headless

import (
	"image"

	"github.com/Tangerg/oolong/components/internal/identity"
	"github.com/Tangerg/oolong/core/input"
)

// PointerRegion routes one child through its committed presentation. Capture
// follows the child using current coordinates, including outside its hit region;
// a disappeared child receives cancellation without transferring its gesture. Foreign-button
// releases preserve capture; matching or unspecified releases end it.
// The zero value declines input. Stage during [Root.Draw], read only from Handle.
// A PointerRegion must not be copied after first use.
type PointerRegion struct {
	noCopy noCopy

	presented Snapshot[pointerRegionFrame]
	// capture identifies one continuously presented child, even across moving frames.
	// A removed child returning later starts a different presentation lifetime.
	capture pointerCapture[pointerRegionFrame]
	hover   pointerRegionFrame
}

// Publish child identity and geometry together so routing cannot combine different frames.
type pointerRegionFrame struct {
	area, clip image.Rectangle
	child      Interactive
	identity   *byte
}

// Stage publishes where child was drawn, to take effect with the complete root frame.
//
// A child that does not answer input is staged as an absence: the region is still not
// somewhere a press can land, and saying so here keeps the caller from having to.
func (r *PointerRegion) Stage(frame Frame, area image.Rectangle, child Widget) {
	r.stage(frame, area, area, child)
}

// stage separates the child's coordinate origin from its visible hit region.
// A viewport clips the content without changing the coordinates the content owns.
func (r *PointerRegion) stage(frame Frame, area, clip image.Rectangle, child Widget) {
	if r == nil {
		return
	}
	clip = clip.Intersect(area).Intersect(frame.Visible())
	target, _ := child.(Interactive)
	if target == nil {
		area = image.Rectangle{}
	}
	previous := r.presented.Value()
	id := previous.identity
	if target == nil || clip.Empty() {
		id = nil
	} else if id == nil || !identity.Same(previous.child, target) {
		id = new(byte)
	}
	r.presented.Stage(frame, pointerRegionFrame{area: area, clip: clip, child: target, identity: id})
}

// Handle offers a pointer event to the child, in the child's own coordinates.
//
// Handled reports consumption, including a route superseded by synchronous input or
// redraw. Delivered reports direct delivery, excluding cancellation and hover leave.
// A wrapper may offer another target only when both are false; an expired route must
// not activate a replacement.
func (r *PointerRegion) Handle(event input.Mouse) (handled, delivered bool) {
	if r == nil {
		return false, false
	}
	held, turn := r.capture.begin(event)
	if !r.capture.current(turn) {
		return true, false
	}
	if event.Action == input.MouseCancel {
		return false, false
	}
	if event.Action == input.MouseLeave {
		return r.leave(event, turn), false
	}
	presented := r.presented.Value()

	if held.continues(event) {
		if held.cancelled || presented.identity != held.target.identity {
			if r.capture.cancel(turn, held, event) {
				return r.leave(event, turn), false
			}
			return true, false
		}
		return r.deliver(presented, event, turn)
	}

	if presented.child == nil || presented.clip.Empty() || !event.Pos.In(presented.clip) {
		return r.leave(event, turn), false
	}
	return r.deliver(presented, event, turn)
}

func (r *PointerRegion) deliver(presented pointerRegionFrame, event input.Mouse, turn uint64) (bool, bool) {
	_, revision := r.presented.borrow()
	r.hoverTo(presented, event)
	if !r.capture.current(turn) || !r.presented.unchanged(revision) || r.hover.identity != presented.identity {
		return true, false
	}
	handled := r.capture.deliver(turn, presented, event)
	return handled || !r.capture.current(turn) || !r.presented.unchanged(revision), true
}

func (p pointerRegionFrame) deliverPointer(event input.Mouse) bool {
	event.Pos = event.Pos.Sub(p.area.Min)
	return p.child.Handle(event)
}

func (r *PointerRegion) leave(event input.Mouse, turn uint64) bool {
	_, revision := r.presented.borrow()
	r.hoverTo(pointerRegionFrame{}, event)
	return !r.capture.current(turn) || !r.presented.unchanged(revision)
}

func (r *PointerRegion) hoverTo(to pointerRegionFrame, event input.Mouse) {
	previous := r.hover
	r.hover = to
	if previous.identity == to.identity {
		return
	}
	if previous.child != nil {
		event.Action = input.MouseLeave
		event.Pos = event.Pos.Sub(previous.area.Min)
		previous.child.Handle(event)
	}
}
