package headless

import (
	"image"

	"github.com/Tangerg/oolong/core/input"
)

// Pointer owns hover, capture and completed clicks for one interactive control.
// Stage publishes the control's hit region with a complete root frame. Handle
// settles gestures against that region, so a press and release need no intervening
// draw. Over and Pressing are pure appearance queries; Clicked consumes a completed
// click from the event handler.
//
// Each control owns its Pointer. Containers and PointerRegion route between controls;
// rectangles describe geometry and never identify different controls. A Pointer must
// not be copied after first use. Its zero value declines input until Stage commits.
type Pointer struct {
	noCopy noCopy

	presentation Snapshot[pointerFrame]
	at           image.Point
	inside       bool
	held         *byte
	button       input.Button
	clicked      bool
}

type pointerFrame struct {
	area     image.Rectangle
	identity *byte
}

// Stage publishes the control's local hit region, clipped to its frame. A gesture
// follows the same control when its region moves or resizes. An empty committed
// region ends that presentation lifetime, so showing it again cannot revive capture.
func (p *Pointer) Stage(frame Frame, area image.Rectangle) {
	area = area.Intersect(frame.Bounds())
	id := p.presentation.Value().identity
	if area.Empty() {
		id = nil
	} else if id == nil {
		id = new(byte)
	}
	p.presentation.Stage(frame, pointerFrame{area: area, identity: id})
}

// Handle updates position and settles a pointer event against the committed region.
// An accepted press owns dragging and release even outside the region. A new press
// supersedes it. Other events are delivered only while over the visible control.
func (p *Pointer) Handle(event input.Event) bool {
	mouse, ok := event.(input.Mouse)
	if !ok {
		return false
	}
	p.at, p.inside, p.clicked = mouse.Pos, true, false
	presented := p.presentation.Value()
	switch mouse.Action {
	case input.MouseDown:
		p.held, p.button = nil, input.ButtonNone
		if !p.Over() {
			return false
		}
		p.held, p.button = presented.identity, mouse.Button
		return true
	case input.MouseDrag, input.MouseUp:
		owner := p.held
		if mouse.Action == input.MouseUp {
			p.held = nil
		}
		if owner == nil || owner != presented.identity {
			return false
		}
		if mouse.Action == input.MouseUp {
			p.clicked = p.Over()
		}
		return true
	default:
		return p.Over()
	}
}

// Left ends hover, capture and any unconsumed click, such as when focus is lost.
func (p *Pointer) Left() {
	p.inside, p.clicked = false, false
	p.held, p.button = nil, input.ButtonNone
}

// Position reports the last pointer coordinates and whether it is in the interface.
func (p *Pointer) Position() (image.Point, bool) { return p.at, p.inside }

// Over reports whether the pointer is over the control's committed region.
func (p *Pointer) Over() bool {
	return p.inside && p.at.In(p.presentation.Value().area)
}

// Pressing reports a live captured press, even while the pointer is outside.
func (p *Pointer) Pressing() bool {
	return p.held != nil && p.held == p.presentation.Value().identity
}

// Clicked consumes a completed click of button. Call it after Handle in the event
// handler. Drawing observes Over and Pressing without consuming input.
func (p *Pointer) Clicked(button input.Button) bool {
	if !p.clicked || p.button != button {
		return false
	}
	p.clicked = false
	return true
}
