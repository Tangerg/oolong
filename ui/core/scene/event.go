package scene

import (
	"image"
	"slices"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
)

// EventResult controls propagation, focus and pointer capture. Consumption does not imply model mutation.
type EventResult struct {
	Consumed bool
	Capture  bool
	Focus    bool
}

// Event coordinates belong to Target, not to the terminal or its parent. The
// scene has already checked attachment, scope and current interaction eligibility.
type Event struct {
	Input  input.Event
	Target Target
	root   *Root
	area   image.Rectangle
	layout *layoutSnapshot
	turn   uint64
}

// Read returns the geometry projection belonging to this input, not the latest
// admitted frame. Keyboard events read the current projection. Missing geometry
// is explicit; a caller must not substitute a newer text or scrolling layout.
func (e Event) Read[T any](snapshot *Snapshot[T]) (T, bool) {
	var zero T
	if snapshot == nil {
		return zero, false
	}
	if _, pointer := e.Input.(input.Mouse); !pointer {
		return snapshot.Value(), true
	}
	if e.layout == nil {
		return zero, false
	}
	value, ok := e.layout.projections[snapshot]
	if !ok {
		return zero, false
	}
	projection, ok := value.(T)
	return projection, ok
}

// CurrentLayout reports whether this input belongs to the latest admitted geometry.
func (e Event) CurrentLayout() bool {
	return e.root != nil && (e.layout == nil || e.root.layout == e.layout)
}

// RequestFocus asks the scene to focus the current event target.
func (e Event) RequestFocus() error {
	if e.root == nil {
		return ErrDetached
	}
	return e.root.RequestFocus(e.Target)
}

// FocusNext traverses eligible targets in scene order, optionally backward.
func (e Event) FocusNext(backward bool) bool {
	return e.root != nil && e.root.FocusNext(backward)
}

// Capture claims this pointer press for the event target until its release or revocation.
// The input turn and attachment must still be current; pure phases reject the request.
func (e Event) Capture() error {
	if e.root == nil {
		return ErrDetached
	}
	if err := e.root.mutable(); err != nil {
		return err
	}
	if e.turn != e.root.inputTurn || e.layout == nil || e.layout.epoch != e.root.geometryEpoch {
		return ErrStale
	}
	mouse, ok := e.Input.(input.Mouse)
	if !ok || mouse.Action != input.MouseDown {
		return ErrInvalid
	}
	n := e.root.resolve(e.Target)
	if n == nil {
		return ErrStale
	}
	if !e.root.allowed(n) {
		return ErrUnavailable
	}
	e.root.capture = capture{target: e.Target, button: mouse.Button, area: e.area}
	return nil
}

type capture struct {
	target  Target
	button  input.Button
	area    image.Rectangle
	revoked bool
}

func (c capture) cancel(n *node) {
	if n != nil && n.config.Handle != nil {
		n.config.Handle(Event{Input: input.Mouse{Action: input.MouseCancel, Button: c.button}, Target: c.target})
	}
}

// CapturedInput retains only the immutable hit map current at input arrival.
// Its lifetime is bounded by the host's input queue; processing never replaces
// that map with a newer layout and thereby retargets an old physical click.
type CapturedInput struct {
	Input  input.Event
	root   *Root
	layout *layoutSnapshot
}

// CaptureInput may be called by the input reader goroutine. All other root
// operations, including HandleInput, belong to the interface owner sequence.
func (r *Root) CaptureInput(event input.Event) CapturedInput {
	if r == nil {
		return CapturedInput{Input: event}
	}
	captured := CapturedInput{Input: event, root: r}
	history := r.published.Load()
	if history == nil {
		return captured
	}
	var at time.Time
	if mouse, ok := event.(input.Mouse); ok {
		at = mouse.At
	}
	for _, snapshot := range slices.Backward(history.layouts) {
		if at.IsZero() || !at.Before(snapshot.writtenAt) {
			captured.layout = snapshot
			break
		}
	}
	return captured
}

// Handle captures the current written geometry and immediately routes input on the owner sequence.
func (r *Root) Handle(event input.Event) bool { return r.HandleInput(r.CaptureInput(event)) }

// HandleInput routes an arrival snapshot while checking current attachment and interaction eligibility.
func (r *Root) HandleInput(event CapturedInput) bool {
	if r == nil || r.mutable() != nil || event.root != r {
		return false
	}
	r.settleFocus()
	r.FlushNotifications()
	r.inputTurn = next(r.inputTurn)
	turn := r.inputTurn
	var consumed bool
	if mouse, ok := event.Input.(input.Mouse); ok {
		if event.layout != nil && event.layout.epoch != r.geometryEpoch {
			event.layout = nil
		}
		consumed = r.mouse(mouse, event.layout)
	} else {
		n := r.resolve(r.focus)
		if n == nil {
			n = r.content.node
		}
		consumed = r.bubble(n, event.Input, nil)
	}
	return consumed || r.inputTurn != turn
}

func (r *Root) mouse(mouse input.Mouse, snapshot *layoutSnapshot) bool {
	turn := r.inputTurn
	if mouse.Action == input.MouseCancel || mouse.Action == input.MouseDown {
		previous := r.capture
		r.capture = capture{}
		if n := r.resolve(previous.target); n != nil && !previous.revoked {
			previous.cancel(n)
		}
		if r.inputTurn != turn {
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
	if held.revoked || !r.allowed(n) {
		r.revokeCapture(mouse, held, n)
		return true, true
	}
	if mouse.Action == input.MouseUp && !mouse.Releases(held.button) {
		return false, true
	}
	area := held.area
	if placement, ok := findGeometry(snapshot, held.target); ok {
		if !r.currentContent(n, placement) {
			r.revokeCapture(mouse, held, n)
			return true, true
		}
		area = placement.area
	}
	if mouse.Releases(held.button) {
		r.capture = capture{}
	}
	if r.preview(n, mouse, snapshot) {
		return true, true
	}
	return r.deliver(n, mouse, area, snapshot).Consumed, true
}

func (r *Root) revokeCapture(mouse input.Mouse, held capture, n *node) {
	turn := r.inputTurn
	r.capture.revoked = true
	if !held.revoked {
		held.cancel(n)
	}
	if r.inputTurn == turn && mouse.Releases(held.button) {
		r.capture = capture{}
	}
}

func (r *Root) currentContent(n *node, placement geometry) bool {
	return n.config.Semantics == nil || r.project(n.config.Semantics).ContentRevision == placement.content
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
		if !r.currentContent(n, placement) {
			return false
		}
		r.hoverTo(n.target(), mouse, snapshot)
		if r.inputTurn != turn || r.resolve(placement.target) == nil || r.hover != placement.target {
			return true
		}
		return r.bubble(n, mouse, snapshot)
	}
	r.leave(mouse)
	return false
}

func (r *Root) modalPointer(mouse input.Mouse, snapshot *layoutSnapshot) bool {
	modal := r.modal()
	if modal == nil {
		return false
	}
	placement, ok := findGeometry(snapshot, modal.target())
	if !ok {
		return false
	}
	return r.deliver(modal, mouse, placement.area, snapshot).Consumed
}

// preview offers the same route from its outermost allowed ancestor to the target.
// It lets application policy consume a key or paste before a focused primitive.
func (r *Root) preview(start *node, event input.Event, snapshot *layoutSnapshot) bool {
	if start == nil {
		return false
	}
	origin := start.target()
	var path []Target
	for n := start; n != nil && r.enabled(n); n = n.parent {
		path = append(path, n.target())
	}
	for _, target := range slices.Backward(path) {
		n := r.resolve(target)
		if n == nil || !r.enabled(n) || r.resolve(origin) == nil || !r.allowed(start) {
			return true
		}
		if n.config.Preview == nil {
			continue
		}
		var area image.Rectangle
		if _, pointer := event.(input.Mouse); pointer {
			placement, ok := findGeometry(snapshot, target)
			if !ok {
				continue
			}
			area = placement.area
		}
		if r.dispatch(n, n.config.Preview, event, area, snapshot).Consumed || r.resolve(origin) == nil {
			return true
		}
	}
	return false
}

func (r *Root) bubble(start *node, event input.Event, snapshot *layoutSnapshot) bool {
	if r.preview(start, event, snapshot) {
		return true
	}
	for n := start; n != nil; n = n.parent {
		target := n.target()
		if !r.allowed(n) {
			return false
		}
		var area image.Rectangle
		if _, pointer := event.(input.Mouse); pointer {
			placement, ok := findGeometry(snapshot, target)
			if !ok {
				return false
			}
			area = placement.area
		}
		result := r.deliver(n, event, area, snapshot)
		if result.Consumed || r.resolve(target) == nil {
			return true
		}
	}
	return false
}

func (r *Root) deliver(n *node, event input.Event, area image.Rectangle, snapshot *layoutSnapshot) EventResult {
	if n == nil {
		return EventResult{}
	}
	return r.dispatch(n, n.config.Handle, event, area, snapshot)
}

func (r *Root) dispatch(n *node, handle func(Event) EventResult, event input.Event, area image.Rectangle, snapshot *layoutSnapshot) EventResult {
	if n == nil || handle == nil {
		return EventResult{}
	}
	target := n.target()
	turn := r.inputTurn
	focusTurn := r.focusTurn
	_, pointer := event.(input.Mouse)
	if mouse, ok := event.(input.Mouse); ok {
		mouse.Pos = mouse.Pos.Sub(area.Min)
		event = mouse
	}
	delivery := Event{Input: event, Target: target, root: r, area: area, layout: snapshot, turn: turn}
	result := handle(delivery)
	if r.resolve(target) == nil || r.inputTurn != turn || !pointer && r.focusTurn != focusTurn {
		return EventResult{Consumed: true}
	}
	if result.Focus {
		_ = delivery.RequestFocus()
		if !pointer && r.focusTurn != focusTurn {
			return EventResult{Consumed: true}
		}
	}
	if result.Capture && result.Consumed {
		_ = delivery.Capture()
	}
	return result
}

func (r *Root) hoverTo(target Target, mouse input.Mouse, snapshot *layoutSnapshot) {
	if r.hover == target {
		return
	}
	previous := r.hover
	r.hover = target
	if n := r.resolve(previous); n != nil {
		placement, _ := findGeometry(snapshot, previous)
		mouse.Action = input.MouseLeave
		r.deliver(n, mouse, placement.area, snapshot)
	}
}

func (r *Root) leave(mouse input.Mouse) { r.hoverTo(Target{}, mouse, r.written) }

func findGeometry(snapshot *layoutSnapshot, target Target) (geometry, bool) {
	if snapshot != nil {
		for _, placement := range snapshot.geometry {
			if placement.target == target {
				return placement, true
			}
		}
	}
	return geometry{}, false
}
