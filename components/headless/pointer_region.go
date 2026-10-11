package headless

import (
	"image"

	"github.com/Tangerg/oolong/components/internal/identity"
	"github.com/Tangerg/oolong/core/input"
)

// PointerRegion routes one child through its committed presentation. Capture
// follows the child using current coordinates, including outside its hit region;
// a disappeared child cannot transfer its gesture to a replacement. Foreign-button
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
// Handled reports whether the child consumed it. Delivered reports whether it reached
// the child at all, which is a different question and the one a wrapper with a second
// pointer target asks: a strip of tabs above a pane needs to know that an event was
// outside the pane, not merely that the pane declined it.
func (r *PointerRegion) Handle(event input.Mouse) (handled, delivered bool) {
	if r == nil {
		return false, false
	}
	presented := r.presented.Value()
	held, turn := r.capture.begin(event)
	if !r.capture.current(turn) || event.Action == input.MouseCancel {
		return false, false
	}
	if event.Action == input.MouseLeave {
		r.leave(event)
		return false, false
	}

	if held != nil &&
		(event.Action == input.MouseDrag || event.Action == input.MouseUp) {
		// Keep the dead token through release: dropping it early would send the
		// remaining events to a replacement. Tokens never return after an absence.
		if presented.identity != held.identity {
			r.leave(event)
			return false, false
		}
		return r.deliver(presented, event, turn)
	}

	if presented.child == nil || presented.clip.Empty() || !event.Pos.In(presented.clip) {
		r.leave(event)
		return false, false
	}
	if event.Action == input.MouseDown {
		handled = r.capture.offer(turn, presented, func() bool {
			var consumed bool
			consumed, delivered = r.deliver(presented, event, turn)
			return consumed
		})
		return handled, delivered
	}
	return r.deliver(presented, event, turn)
}

func (r *PointerRegion) deliver(presented pointerRegionFrame, event input.Mouse, turn uint64) (bool, bool) {
	r.hoverTo(presented, event)
	if !r.capture.current(turn) || r.hover.identity != presented.identity || r.presented.Value().identity != presented.identity {
		return false, false
	}
	return presented.deliverPointer(event), true
}

func (p pointerRegionFrame) deliverPointer(event input.Mouse) bool {
	event.Pos = event.Pos.Sub(p.area.Min)
	return p.child.Handle(event)
}

func (r *PointerRegion) leave(event input.Mouse) {
	r.hoverTo(pointerRegionFrame{}, event)
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
