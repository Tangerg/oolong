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
	input    input.Event
	target   Target
	route    inputRoute
	delivery *byte
}

// A route keeps the original hit/focus as well as each callback's receiver.
// Preview, propagation and commands returned by a handler all use this evidence.
type inputRoute struct {
	root            *Root
	origin          Target
	layout          *layoutSnapshot
	turn, focusTurn *byte
	pointer         bool
	// crossesModal admits the origin's ancestors above an active modal, which
	// hold that layer rather than sit beneath it. Only shortcuts cross it.
	crossesModal bool
}

func (r *Root) route(n *node, event input.Event, snapshot *layoutSnapshot) inputRoute {
	_, pointer := event.(input.Mouse)
	return inputRoute{root: r, origin: n.target(), layout: snapshot, turn: r.inputTurn, focusTurn: r.focusTurn, pointer: pointer}
}

func (q inputRoute) check(target Target) error {
	r := q.root
	if r == nil {
		return ErrDetached
	}
	if err := r.mutable(); err != nil {
		return err
	}
	if q.turn != r.inputTurn || !q.pointer && q.focusTurn != r.focusTurn {
		return ErrStale
	}
	if err := q.checkTarget(q.origin); err != nil {
		return err
	}
	if target != q.origin {
		return q.checkTarget(target)
	}
	return nil
}

func (q inputRoute) checkTarget(target Target) error {
	n := q.root.resolve(target)
	if n == nil {
		return ErrStale
	}
	if !q.eligible(n) {
		return ErrUnavailable
	}
	if q.pointer && !q.root.currentGeometry(n, q.layout) {
		return ErrStale
	}
	return nil
}

// eligible applies the interaction rule of this route: a shortcut route asks only
// that n be enabled, because it reaches the ancestors that hold a modal layer.
func (q inputRoute) eligible(n *node) bool {
	if q.crossesModal {
		return q.root.enabled(n)
	}
	return q.root.allowed(n)
}

// Input returns the routed input in local coordinates.
func (e Event) Input() input.Event { return e.input }

func (e Event) Target() Target { return e.target }

// Read returns a value staged in this pointer's arrival frame or, for other
// input, the scene's current admitted frame. A missing projection is distinct
// from a producer that deliberately staged the zero value.
// After [Root.InvalidateGeometry], non-pointer input also reads false until a
// replacing frame is admitted. That revokes coordinates only: keyboard routing
// still follows focus, so a handler whose action does not depend on geometry
// must not treat a false read as a rejected input.
func (e Event) Read[T any](snapshot *Snapshot[T]) (T, bool) {
	var zero T
	layout := e.route.layout
	if !e.route.pointer {
		r := e.route.root
		if r == nil || !r.current(r.layout) {
			return zero, false
		}
		layout = r.layout
	}
	if layout == nil || snapshot == nil {
		return zero, false
	}
	value, ok := layout.projections[snapshot]
	if value == nil {
		return zero, ok
	}
	projection, ok := value.(T)
	return projection, ok
}

// CurrentLayout reports whether this input belongs to the latest admitted geometry.
// It agrees with Read: input that is not a pointer belongs to the latest frame
// while that frame's geometry is current.
func (e Event) CurrentLayout() bool {
	r := e.route.root
	if r == nil || r.closed {
		return false
	}
	if !e.route.pointer {
		return r.current(r.layout)
	}
	return r.layout == e.route.layout && r.current(e.route.layout)
}

func (e Event) checkDelivery() error {
	if e.route.root == nil {
		return ErrDetached
	}
	if e.delivery == nil || e.delivery != e.route.root.delivery {
		if err := e.route.root.mutable(); err != nil {
			return err
		}
		return ErrStale
	}
	return e.route.check(e.target)
}

// RequestFocus asks the scene to focus the target during its current delivery.
func (e Event) RequestFocus() error {
	if err := e.checkDelivery(); err != nil {
		return err
	}
	return e.route.root.RequestFocus(e.target)
}

// FocusNext moves focus to the next eligible target while this delivery is current.
func (e Event) FocusNext() bool { return e.checkDelivery() == nil && e.route.root.FocusNext() }

// FocusPrev moves focus to the previous eligible target while this delivery is current.
func (e Event) FocusPrev() bool { return e.checkDelivery() == nil && e.route.root.FocusPrev() }

// Result answers this event for a handler that reports only whether it consumed the
// input. A consumed left-button press also takes focus and captures the pointer, so
// the drag and release completing the gesture come back to this target even after the
// pointer leaves it. Every other consumed event only stops propagation.
func (e Event) Result(consumed bool) EventResult {
	mouse, pointer := e.input.(input.Mouse)
	press := consumed && pointer && mouse.Presses(input.ButtonLeft)
	return EventResult{Consumed: consumed, Capture: press, Focus: press}
}

// Capture claims and consumes this delivered press until release or revocation.
func (e Event) Capture() error {
	if err := e.checkDelivery(); err != nil {
		return err
	}
	mouse, ok := e.input.(input.Mouse)
	if !ok || mouse.Action != input.MouseDown {
		return ErrInvalid
	}
	placement, _ := e.route.layout.lookup(e.target)
	e.route.root.capture = capture{target: e.target, button: mouse.Button, content: placement.content, press: e.route.turn}
	return nil
}

// CapturedInput retains only the immutable hit map current at input arrival.
// Its lifetime is bounded by the host's input queue; processing never replaces
// that map with a newer layout and thereby retargets an old physical click.
type CapturedInput struct {
	input  input.Event
	root   *Root
	layout *layoutSnapshot
}

func (c CapturedInput) Input() input.Event { return c.input }

// CaptureInput associates physical input with the writer's completed prefix at
// Mouse.At, or its reception time when At is zero. writtenAt must be a pure,
// concurrency-safe query and return false for uncertain or expired evidence.
// Invalid input returns a zero capture; uncertain coordinates retain no hit map.
// Equality with layout admission cannot prove which geometry preceded the input.
// This operation and CapturePresentedInput may run outside the UI owner.
func (r *Root) CaptureInput(event input.Event, writtenAt func(time.Time) (uint64, bool)) CapturedInput {
	if input.Validate(event) != nil {
		return CapturedInput{}
	}
	event = input.Stamp([]input.Event{event}, time.Now())[0]
	if r == nil {
		return CapturedInput{input: event}
	}
	captured := CapturedInput{input: event, root: r}
	mouse, pointer := event.(input.Mouse)
	if !pointer {
		return captured
	}
	if writtenAt == nil {
		return captured
	}
	sequence, known := writtenAt(mouse.At)
	if !known {
		return captured
	}
	captured.layout = r.published.Load().layoutAt(sequence, &mouse.At)
	return captured
}

// CapturePresentedInput binds synthetic pointer intent to a completed writer
// prefix observed by its source at admission. It does not infer causal order from
// the clock. Physical reports must use CaptureInput instead. The source must
// capture before buffering, using this root's writer sequence space; expired or
// unsettled geometry returns a zero capture. Mouse.At remains gesture timing.
// The immutable geometry query is safe outside the UI owner.
func (r *Root) CapturePresentedInput(mouse input.Mouse, sequence uint64) CapturedInput {
	if r == nil || input.Validate(mouse) != nil {
		return CapturedInput{}
	}
	layout := r.published.Load().layoutAt(sequence, nil)
	if layout == nil {
		return CapturedInput{}
	}
	return CapturedInput{input: input.Stamp([]input.Event{mouse}, time.Now())[0], root: r, layout: layout}
}

func (g *inputGeometry) layoutAt(sequence uint64, arrived *time.Time) *layoutSnapshot {
	if g == nil || g.pending {
		return nil
	}
	for _, snapshot := range slices.Backward(g.layouts) {
		if snapshot.sequence > sequence {
			continue
		}
		if arrived == nil || arrived.After(snapshot.admittedAt) {
			return snapshot
		}
		if arrived.Equal(snapshot.admittedAt) || snapshot.hasOutput {
			return nil
		}
	}
	return nil
}

// Handle injects synthetic input against the owner's current written geometry.
// Physical input must use CaptureInput with transport completion evidence; an
// event timestamp alone cannot prove which output had completed at its arrival.
func (r *Root) Handle(event input.Event) bool {
	if r == nil || input.Validate(event) != nil {
		return false
	}
	return r.HandleInput(CapturedInput{input: event, root: r, layout: r.frames.written})
}

// HandleInput routes an arrival snapshot while checking current attachment and interaction eligibility.
func (r *Root) HandleInput(event CapturedInput) bool {
	if r == nil || r.mutable() != nil || event.root != r {
		return false
	}
	held := r.capture
	if held.endsOn(event.input) {
		// Eligibility and lifecycle callbacks can unwind before routing. The
		// physical edge must still end its old gesture, never a reentrant one.
		defer r.finishCapture(held, r.resolve(held.target))
	}
	r.inputTurn = new(byte)
	turn := r.inputTurn
	r.settleFocus()
	r.FlushNotifications()
	if r.inputTurn != turn || r.mutable() != nil {
		return true
	}
	var consumed bool
	if mouse, ok := event.input.(input.Mouse); ok {
		if !r.current(event.layout) {
			event.layout = nil
		}
		consumed = r.mouse(mouse, event.layout)
	} else {
		consumed = r.keyboard(event.input)
	}
	return consumed || r.inputTurn != turn
}

// keyboard routes input that follows focus. Shortcuts see a key first. A modal
// layer that has not reached the screen as it is now composed takes nothing typed,
// because the key or paste was meant for what the user could see; terminal reports
// and focus notices still arrive.
func (r *Root) keyboard(event input.Event) bool {
	route := r.keyboardRoute(r.focus, r.modal())
	if len(route) == 0 {
		return false
	}
	q := r.route(r.resolve(route[0]), event, nil)
	if q.shortcuts(event) {
		return true
	}
	if modal := r.modal(); modal != nil && typed(event) && !r.presented(modal) {
		return true
	}
	return q.bubble(event)
}

// typed reports whether event is something a person entered.
func typed(event input.Event) bool {
	switch event.(type) {
	case input.Key, input.Paste:
		return true
	default:
		return false
	}
}

// shortcuts offers a key to the Shortcut of every node from the root content down
// to the origin, including those above an active modal.
func (q inputRoute) shortcuts(event input.Event) bool {
	if _, ok := event.(input.Key); !ok {
		return false
	}
	q.crossesModal = true
	var path []*node
	for n := q.root.resolve(q.origin); n != nil; n = n.parent {
		path = append(path, n)
	}
	for _, n := range slices.Backward(path) {
		if q.offer(n, n.config.Shortcut, event).Consumed {
			return true
		}
	}
	return false
}

// presented reports whether a confirmed frame carried every child of modal under
// its current attachment. Opening a layer or replacing its content attaches a child
// anew, so either one waits for the frame that shows it.
func (r *Root) presented(modal *node) bool {
	for _, item := range modal.group.items {
		if _, ok := r.frames.shown.lookup(item.Element.node.target()); !ok {
			return false
		}
	}
	return true
}

func (q inputRoute) preview(start *node, event input.Event) bool {
	if q.check(q.origin) != nil {
		return true
	}
	var path []*node
	modal := q.root.modal()
	for n := start; n != nil; n = n.parent {
		path = append(path, n)
		if n == modal {
			break
		}
	}
	for _, n := range slices.Backward(path) {
		if q.offer(n, n.config.Preview, event).Consumed {
			return true
		}
	}
	return false
}

func (q inputRoute) bubble(event input.Event) bool {
	n := q.root.resolve(q.origin)
	if n == nil || q.preview(n, event) {
		return true
	}
	modal := q.root.modal()
	for ; n != nil; n = n.parent {
		if q.offer(n, n.config.Handle, event).Consumed {
			return true
		}
		if n == modal {
			break
		}
	}
	return false
}

func (q inputRoute) offer(n *node, handle func(Event) EventResult, event input.Event) EventResult {
	if handle == nil {
		return EventResult{}
	}
	var area image.Rectangle
	if q.pointer {
		p, ok := q.layout.lookup(n.target())
		if !ok {
			return EventResult{Consumed: true}
		}
		area = p.area
	}
	return q.dispatch(n, handle, event, area)
}

func (q inputRoute) dispatch(n *node, handle func(Event) EventResult, event input.Event, area image.Rectangle) EventResult {
	target := n.target()
	if q.check(target) != nil {
		return EventResult{Consumed: true}
	}
	if handle == nil {
		return EventResult{}
	}
	if mouse, ok := event.(input.Mouse); ok {
		mouse.Pos = mouse.Pos.Sub(area.Min)
		event = mouse
	}
	r := q.root
	r.delivery = new(byte)
	defer func() { r.delivery = nil }()
	delivery := Event{input: event, target: target, route: q, delivery: r.delivery}
	result := handle(delivery)
	if delivery.checkDelivery() != nil {
		return EventResult{Consumed: true}
	}
	if result.Focus {
		_ = delivery.RequestFocus()
		if !q.pointer && q.focusTurn != r.focusTurn {
			return EventResult{Consumed: true}
		}
	}
	// Capturing a gesture claims it, so a capture implies consumption.
	if result.Capture {
		result.Consumed = true
		_ = delivery.Capture()
	}
	if mouse, ok := event.(input.Mouse); ok && mouse.Action == input.MouseDown && r.capture.target == target && r.capture.phase == captureActive {
		result.Consumed = true
	}
	return result
}
