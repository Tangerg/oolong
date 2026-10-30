package base

import (
	"image"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/core/input"
)

// Pointer owns hover, capture and completed clicks for one control. Committed
// presentation identities keep moved controls capturable without reviving controls
// that disappeared. The zero value declines input until Stage commits. A Pointer
// must not be copied after first use.
type Pointer struct {
	noCopy   noCopy
	geometry eventGeometry

	presentation scene.Snapshot[pointerFrame]
	at           image.Point
	inside       bool
	held         *byte
	button       input.Button
	clicked      bool
}

type pointerFrame struct {
	area     image.Rectangle
	track    image.Rectangle
	identity *byte
}

// Stage publishes the control's local hit region, clipped to its frame. A gesture
// follows the same control when its region moves or resizes. An empty committed
// region ends that presentation lifetime, so showing it again cannot revive capture.
func (p *Pointer) Stage(frame scene.Frame, area image.Rectangle) {
	track := area
	area = area.Intersect(frame.Visible())
	id := p.geometry.read(&p.presentation).identity
	if area.Empty() {
		id = nil
	} else if id == nil {
		id = new(byte)
	}
	p.presentation.Stage(frame, pointerFrame{area: area, track: track, identity: id})
}

// Handle updates position and settles a pointer event against the committed region.
// An accepted press owns dragging and release even outside the region. A new press
// supersedes it. Other events are delivered only while over the visible control.
// A release naming another button is declined without ending capture. ButtonNone
// on release means the button was unspecified and releases the captured press.
// MouseLeave invalidates hover while preserving capture and a completed click.
// MouseCancel ends capture without creating a click.
func (p *Pointer) Handle(event input.Event) bool {
	mouse, ok := event.(input.Mouse)
	if !ok {
		return false
	}
	if mouse.Action == input.MouseLeave {
		p.inside = false
		return false
	}
	if mouse.Action == input.MouseCancel {
		p.Left()
		return false
	}
	p.at, p.inside, p.clicked = mouse.Pos, true, false
	switch mouse.Action {
	case input.MouseDown:
		p.held, p.button = nil, input.ButtonNone
		if !p.Over() {
			return false
		}
		p.held, p.button = p.geometry.read(&p.presentation).identity, mouse.Button
		return true
	case input.MouseDrag:
		return p.Pressing() && mouse.Drags(p.button)
	case input.MouseUp:
		if !mouse.Releases(p.button) {
			return false
		}
		pressing := p.Pressing()
		p.held = nil
		p.clicked = pressing && p.Over()
		return pressing
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
	return p.inside && p.at.In(p.geometry.read(&p.presentation).area)
}

// Pressing reports a live captured press, even while the pointer is outside.
func (p *Pointer) Pressing() bool {
	return p.held != nil && p.held == p.geometry.read(&p.presentation).identity
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
