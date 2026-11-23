package scene

import (
	"errors"
	"slices"

	"github.com/Tangerg/oolong/ui/core/input"
)

type capture struct {
	target  Target
	button  input.Button
	content contentVersion
	press   *byte
	phase   capturePhase
}

type capturePhase uint8

const (
	captureActive capturePhase = iota
	captureReleasing
	captureRevoked
)

func (c capture) endsOn(event input.Event) bool {
	mouse, ok := event.(input.Mouse)
	if !ok || c.target.IsZero() {
		return false
	}
	return mouse.Action == input.MouseDown || mouse.Action == input.MouseCancel || mouse.Releases(c.button)
}

func (c capture) cancel(n *node) {
	if n != nil && n.config.Handle != nil {
		n.config.Handle(Event{input: input.Mouse{Action: input.MouseCancel, Button: c.button}, target: c.target})
	}
}

func (r *Root) mouse(mouse input.Mouse, snapshot *layoutSnapshot) bool {
	turn := r.inputTurn
	if mouse.Action == input.MouseCancel || mouse.Action == input.MouseDown {
		previous := r.capture
		r.capture = capture{}
		if n := r.resolve(previous.target); n != nil && previous.phase != captureRevoked {
			previous.cancel(n)
		}
		if r.inputTurn != turn || r.mutable() != nil {
			return true
		}
		if mouse.Action == input.MouseCancel {
			return false
		}
	}
	if mouse.Action == input.MouseLeave {
		r.leave(mouse)
		return false
	}
	if consumed, routed := r.capturedMouse(mouse, snapshot); routed {
		return consumed
	}
	if mouse.Action == input.MouseDrag || mouse.Action == input.MouseUp {
		return false
	}
	if snapshot == nil {
		return false
	}
	return r.hitMouse(mouse, snapshot)
}

func (r *Root) capturedMouse(mouse input.Mouse, snapshot *layoutSnapshot) (consumed, routed bool) {
	held := r.capture
	if held.target.IsZero() || !mouse.Drags(held.button) && mouse.Action != input.MouseUp {
		return false, false
	}
	n := r.resolve(held.target)
	if held.phase == captureRevoked || !r.allowed(n) {
		r.revokeCapture(mouse, held, n)
		return true, true
	}
	if mouse.Action == input.MouseUp && !mouse.Releases(held.button) {
		return false, true
	}
	placement, ok := snapshot.lookup(held.target)
	q := r.route(n, mouse, snapshot)
	if !ok || placement.content != held.content || q.check(held.target) != nil {
		r.revokeCapture(mouse, held, n)
		return true, true
	}
	if held.phase == captureReleasing {
		return true, true
	}
	handle := n.config.Handle
	if mouse.Releases(held.button) {
		r.capture.phase = captureReleasing
		if handle != nil {
			handle = func(e Event) EventResult {
				r.capture = capture{}
				return n.config.Handle(e)
			}
		}
	}
	if q.preview(n, mouse) {
		return true, true
	}
	return q.dispatch(n, handle, mouse, placement.area).Consumed, true
}

// Routing can exit before the release reaches its handler. Only the original
// gesture still awaiting an end needs cancellation; a new press owns itself. A
// notification may have prepared a frame while the release was routed; the model
// must then stay unchanged until it settles, so the cancellation waits in the
// notification queue like every other callback in that phase.
func (r *Root) finishCapture(held capture, n *node) {
	if r.capture.press != held.press {
		return
	}
	cancel := r.capture.phase != captureRevoked
	r.capture = capture{}
	if !cancel {
		return
	}
	if errors.Is(r.mutable(), ErrBusy) && n != nil {
		r.notify(n, func() { held.cancel(n) })
		return
	}
	held.cancel(n)
}

func (r *Root) revokeCapture(mouse input.Mouse, held capture, n *node) {
	turn := r.inputTurn
	r.capture.phase = captureRevoked
	if held.phase != captureRevoked {
		held.cancel(n)
	}
	if r.inputTurn == turn && mouse.Releases(held.button) {
		r.capture = capture{}
	}
}

func (r *Root) currentContent(n *node, placement geometry) bool {
	return r.contentVersion(n) == placement.content
}

func (r *Root) hitMouse(mouse input.Mouse, snapshot *layoutSnapshot) bool {
	turn := r.inputTurn
	for _, placement := range slices.Backward(snapshot.geometry) {
		if !placement.hit || !mouse.Pos.In(placement.clip) {
			continue
		}
		n := r.resolve(placement.target)
		if n == nil {
			return false
		}
		if !r.allowed(n) {
			return r.modalPointer(mouse, snapshot)
		}
		q := r.route(n, mouse, snapshot)
		if q.check(placement.target) != nil {
			return false
		}
		r.hoverTo(n.target(), mouse, snapshot)
		if r.inputTurn != turn || r.resolve(placement.target) == nil || r.hover != placement.target {
			return true
		}
		return q.bubble(mouse)
	}
	r.leave(mouse)
	return false
}

// modalPointer gives a pointer blocked by a modal layer to that layer through the
// ordinary route: it becomes the hovered target, so what it covers is left, and
// its Preview sees the event before its Handle, as for any other delivery.
func (r *Root) modalPointer(mouse input.Mouse, snapshot *layoutSnapshot) bool {
	modal := r.modal()
	if modal == nil {
		return false
	}
	if _, ok := snapshot.lookup(modal.target()); !ok {
		return false
	}
	turn := r.inputTurn
	q := r.route(modal, mouse, snapshot)
	r.hoverTo(modal.target(), mouse, snapshot)
	if r.inputTurn != turn || r.hover != modal.target() {
		return true
	}
	return q.bubble(mouse)
}

func (r *Root) currentGeometry(n *node, snapshot *layoutSnapshot) bool {
	if !r.current(snapshot) {
		return false
	}
	p, ok := snapshot.lookup(n.target())
	return ok && r.currentContent(n, p)
}

func (r *Root) hoverTo(target Target, mouse input.Mouse, snapshot *layoutSnapshot) {
	if r.hover == target {
		return
	}
	previous := r.hover
	r.hover = target
	if n := r.resolve(previous); n != nil && n.config.Handle != nil {
		placement, _ := snapshot.lookup(previous)
		mouse.Action = input.MouseLeave
		mouse.Pos = mouse.Pos.Sub(placement.area.Min)
		n.config.Handle(Event{input: mouse, target: previous, route: inputRoute{layout: snapshot, pointer: true}})
	}
}

func (r *Root) leave(mouse input.Mouse) { r.hoverTo(Target{}, mouse, r.frames.written) }

// Geometry loss and detachment revoke the hit before user code can reenter.
// A newer hit on the same attachment supersedes its queued leave.
func (r *Root) revokeHover(n *node) {
	previous := r.hover
	r.hover = Target{}
	if n.config.Handle != nil {
		r.notify(n, func() {
			if r.hover != previous {
				n.config.Handle(Event{input: input.Mouse{Action: input.MouseLeave}, target: previous})
			}
		})
	}
}
