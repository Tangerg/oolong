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
	input     input.Event
	target    Target
	root      *Root
	area      image.Rectangle
	layout    *layoutSnapshot
	turn      uint64
	delivery  uint64
	focusTurn uint64
}

// Input returns the routed input in local coordinates.
func (e Event) Input() input.Event { return e.input }

// Target returns the exact attachment that received this delivery.
func (e Event) Target() Target { return e.target }

// Read returns the geometry projection belonging to this input, not the latest
// admitted frame. Keyboard events read the current projection. Missing geometry
// is explicit; a caller must not substitute a newer text or scrolling layout.
func (e Event) Read[T any](snapshot *Snapshot[T]) (T, bool) {
	var zero T
	if snapshot == nil {
		return zero, false
	}
	if _, pointer := e.input.(input.Mouse); !pointer {
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

// checkDelivery guards the authority of one callback, independently of whether
// its input and frame remain useful as read-only evidence afterwards.
func (e Event) checkDelivery() error {
	if e.root == nil {
		return ErrDetached
	}
	if err := e.root.mutable(); err != nil {
		return err
	}
	_, pointer := e.input.(input.Mouse)
	if e.turn != e.root.inputTurn || e.delivery != e.root.delivery || e.root.resolve(e.target) == nil || !pointer && e.focusTurn != e.root.focusTurn {
		return ErrStale
	}
	return nil
}

// RequestFocus asks the scene to focus the target during its current delivery.
func (e Event) RequestFocus() error {
	if err := e.checkDelivery(); err != nil {
		return err
	}
	return e.root.RequestFocus(e.target)
}

// FocusNext traverses eligible targets while this delivery is current.
func (e Event) FocusNext(backward bool) bool {
	return e.checkDelivery() == nil && e.root.FocusNext(backward)
}

// Capture claims this delivered pointer press until release or revocation.
func (e Event) Capture() error {
	if err := e.checkDelivery(); err != nil {
		return err
	}
	if e.layout == nil || e.layout.epoch != e.root.geometryEpoch {
		return ErrStale
	}
	mouse, ok := e.input.(input.Mouse)
	if !ok || mouse.Action != input.MouseDown {
		return ErrInvalid
	}
	n := e.root.resolve(e.target)
	if !e.root.allowed(n) {
		return ErrUnavailable
	}
	e.root.capture = capture{target: e.target, button: mouse.Button, area: e.area}
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
		n.config.Handle(Event{input: input.Mouse{Action: input.MouseCancel, Button: c.button}, target: c.target})
	}
}

// CapturedInput retains only the immutable hit map current at input arrival.
// Its lifetime is bounded by the host's input queue; processing never replaces
// that map with a newer layout and thereby retargets an old physical click.
type CapturedInput struct {
	input  input.Event
	root   *Root
	layout *layoutSnapshot
}

// Input returns the input that arrived with this geometry.
func (c CapturedInput) Input() input.Event { return c.input }

// CaptureInput accepts immutable terminal values; invalid inputs return a zero capture.
// It may be called by the input reader goroutine. All other root
// operations, including HandleInput, belong to the interface owner sequence.
func (r *Root) CaptureInput(event input.Event) CapturedInput {
	if input.ByteSize(event) == 0 {
		return CapturedInput{}
	}
	if r == nil {
		return CapturedInput{input: event}
	}
	captured := CapturedInput{input: event, root: r}
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
	r.inputTurn = next(r.inputTurn)
	turn := r.inputTurn
	r.settleFocus()
	r.FlushNotifications()
	if r.inputTurn != turn || r.mutable() != nil {
		return true
	}
	var consumed bool
	if mouse, ok := event.input.(input.Mouse); ok {
		if event.layout != nil && event.layout.epoch != r.geometryEpoch {
			event.layout = nil
		}
		consumed = r.mouse(mouse, event.layout)
	} else {
		n := r.resolve(r.focus)
		if n == nil {
			n = r.content.node
		}
		consumed = r.bubble(n, event.input, nil)
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
	modal := r.modal()
	for n := start; n != nil && r.enabled(n); n = n.parent {
		path = append(path, n.target())
		if n == modal {
			break
		}
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
		if r.dispatch(n, n.config.Preview, event, area, snapshot).Consumed || !r.routeCurrent(origin, snapshot) {
			return true
		}
	}
	return false
}

func (r *Root) routeCurrent(origin Target, snapshot *layoutSnapshot) bool {
	n := r.resolve(origin)
	if r.mutable() != nil || !r.allowed(n) {
		return false
	}
	if snapshot == nil {
		return true
	}
	p, ok := findGeometry(snapshot, origin)
	return snapshot.epoch == r.geometryEpoch && ok && r.currentContent(n, p)
}

func (r *Root) bubble(start *node, event input.Event, snapshot *layoutSnapshot) bool {
	if start == nil {
		return false
	}
	origin := start.target()
	if !r.routeCurrent(origin, snapshot) || r.preview(start, event, snapshot) {
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
		if result.Consumed || !r.routeCurrent(origin, snapshot) {
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
	r.delivery = next(r.delivery)
	defer func() { r.delivery = next(r.delivery) }()
	delivery := Event{input: event, target: target, root: r, area: area, layout: snapshot, turn: turn, delivery: r.delivery, focusTurn: focusTurn}
	result := handle(delivery)
	if r.mutable() != nil || r.resolve(target) == nil || r.inputTurn != turn || !pointer && r.focusTurn != focusTurn {
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
