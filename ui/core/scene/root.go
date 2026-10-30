package scene

import (
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
)

// RootConfig supplies one scene and the bounded history used to resolve timestamped physical input.
type RootConfig struct {
	Element Element
	// InputHistory bounds timestamp lookup for host-buffered mouse reports.
	// Already captured inputs retain their own immutable snapshot separately.
	InputHistory int
	Now          func() time.Time
}

// Root is the sole owner of live attachment, actual focus, capture and geometry.
// Except CaptureInput, its operations run on the UI owner sequence.
// A Root must not be copied after first use.
type Root struct {
	noCopy          noCopy
	content         Element
	nodes           map[NodeID]*node
	nextID          uint64
	modelRevision   uint64
	focus           Target
	capture         capture
	hover           Target
	restore         map[*node]Target
	activeModal     *node
	changes         chan struct{}
	notifications   []func()
	preparing       bool
	observing       bool
	closed          bool
	pending         *Prepared
	layout          *layoutSnapshot
	written         *layoutSnapshot
	inFlight        *layoutSnapshot
	lastSequence    uint64
	writtenSequence uint64
	published       atomic.Pointer[writtenHistory]
	inputHistory    int
	now             func() time.Time
	geometryEpoch   uint64
	namespace       *namespace
	inputTurn       uint64
	focusTurn       uint64
}

type namespace struct{ _ byte }

// NewRoot mounts one content element and establishes its initial focus.
func NewRoot(cfg RootConfig) (*Root, error) {
	r := &Root{nodes: make(map[NodeID]*node), restore: make(map[*node]Target), changes: make(chan struct{})}
	r.namespace = &namespace{}
	if cfg.InputHistory < 0 {
		return nil, ErrInvalid
	}
	r.inputHistory = cfg.InputHistory
	if r.inputHistory == 0 {
		r.inputHistory = 8
	}
	r.now = cfg.Now
	if r.now == nil {
		r.now = time.Now
	}
	if err := r.SetContent(cfg.Element); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Root) mutable() error {
	if r == nil || r.closed {
		return ErrClosed
	}
	if r.preparing || r.observing || r.pending != nil {
		return ErrBusy
	}
	return nil
}

// Content returns the mounted composition handle.
func (r *Root) Content() Element {
	if r == nil {
		return Element{}
	}
	return r.content
}

// SetContent replaces the composition after validating its entire attachment relation.
func (r *Root) SetContent(element Element) error {
	if err := r.mutable(); err != nil {
		return err
	}
	if r.content == element {
		return nil
	}
	if element.node != nil {
		if element.node.parent != nil {
			return ErrMounted
		}
		if err := r.canAttach(element.node); err != nil {
			return err
		}
	}
	old := r.content
	r.content = element
	if old.node != nil {
		r.detach(old.node)
	}
	if element.node != nil {
		r.attach(element.node)
	}
	r.settleFocus()
	r.Changed()
	return nil
}

func (r *Root) canAttach(n *node) error {
	if n.root != nil {
		return ErrMounted
	}
	if n.generation == Generation(math.MaxUint64) || r.nextID == math.MaxUint64 {
		return fmt.Errorf("%w: attachment identity exhausted", ErrInvalid)
	}
	if n.group != nil {
		for _, item := range n.group.items {
			if err := r.canAttach(item.Element.node); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Root) attach(n *node) {
	if n.identityRoot != r.namespace {
		r.nextID = next(r.nextID)
		n.id, n.identityRoot = NodeID(r.nextID), r.namespace
		n.generation = 0
	}
	n.generation = Generation(next(uint64(n.generation)))
	n.root = r
	n.done = make(chan struct{})
	r.nodes[n.id] = n
	if n.group != nil {
		for _, item := range n.group.items {
			r.attach(item.Element.node)
		}
	}
}

func (r *Root) detach(n *node) {
	previous := n.target()
	if r.focus == previous {
		r.setFocus(Target{})
	}
	delete(r.nodes, n.id)
	n.root = nil
	close(n.done)
	n.done = nil
	if r.capture.target == previous {
		held := r.capture
		r.capture.revoked = true
		if n.config.Handle != nil {
			r.notifications = append(r.notifications, func() { held.cancel(n) })
		}
	}
	if r.hover == previous {
		r.hover = Target{}
		if n.config.Handle != nil {
			r.notifications = append(r.notifications, func() { n.config.Handle(Event{Input: input.Mouse{Action: input.MouseLeave}, Target: previous}) })
		}
	}
	if n.group != nil {
		for _, item := range n.group.items {
			r.detach(item.Element.node)
		}
	}
	if n.config.Detached != nil {
		r.notifications = append(r.notifications, n.config.Detached)
	}
}

func (r *Root) resolve(target Target) *node {
	if r == nil || r.closed || target.root != r {
		return nil
	}
	n := r.nodes[target.Node]
	if n == nil || n.generation != target.Generation || n.config.Part != target.Part {
		return nil
	}
	return n
}

// Owns checks the target namespace without claiming that its attachment remains alive.
func (r *Root) Owns(target Target) bool { return r != nil && target.root == r }

// Valid reports whether this exact target generation remains attached to the root.
func (r *Root) Valid(target Target) bool { return r.resolve(target) != nil }

// project applies the same read barrier to eligibility and preconditions as to
// painting and observation. A query cannot detach the target it is authorizing.
func (r *Root) project[T any](read func() T) T {
	previous := r.observing
	r.observing = true
	defer func() { r.observing = previous }()
	return read()
}

func (r *Root) enabled(n *node) bool {
	if n == nil || n.root != r {
		return false
	}
	for ancestor := n; ancestor != nil; ancestor = ancestor.parent {
		if ancestor.config.Enabled != nil && !r.project(ancestor.config.Enabled) {
			return false
		}
	}
	return true
}

func (r *Root) allowed(n *node) bool {
	if !r.enabled(n) {
		return false
	}
	modal := r.modal()
	if modal == nil {
		return true
	}
	for ancestor := n; ancestor != nil; ancestor = ancestor.parent {
		if ancestor == modal {
			return true
		}
	}
	return false
}

func (r *Root) modal() *node {
	var current *node
	r.walk(r.content.node, func(n *node) bool {
		if n.group != nil && n.group.config.Modal && len(n.group.items) != 0 && r.enabled(n) {
			current = n
		}
		return true
	})
	return current
}

func (r *Root) focusable(n *node) bool {
	return r.allowed(n) && n.config.Focusable != nil && r.project(n.config.Focusable)
}

// Focus returns the single current keyboard target, or zero when none is eligible.
func (r *Root) Focus() Target {
	if r == nil {
		return Target{}
	}
	return r.focus
}

// RequestFocus focuses an eligible current target and defers notifications until state has settled.
func (r *Root) RequestFocus(target Target) error {
	if err := r.mutable(); err != nil {
		return err
	}
	n := r.resolve(target)
	if n == nil {
		return ErrStale
	}
	if !r.focusable(n) {
		return ErrUnavailable
	}
	r.setFocus(target)
	return nil
}

func (r *Root) setFocus(target Target) {
	if r.focus == target {
		return
	}
	previous := r.focus
	r.focus = target
	r.focusTurn = next(r.focusTurn)
	if old := r.nodes[previous.Node]; previous.root == r && old != nil && old.generation == previous.Generation {
		if notify := old.config.FocusChanged; notify != nil {
			r.notifications = append(r.notifications, func() { notify(false) })
		}
	}
	if n := r.resolve(target); n != nil && n.config.FocusChanged != nil {
		notify := n.config.FocusChanged
		r.notifications = append(r.notifications, func() {
			if r.focus == target {
				notify(true)
			}
		})
	}
	r.modelRevision = next(r.modelRevision)
	r.signal()
}

func (r *Root) settleFocus() {
	modal := r.modal()
	if modal != r.activeModal {
		previous := r.activeModal
		r.activeModal = modal
		r.restoreFocus(previous)
		if modal != nil {
			if _, remembered := r.restore[modal]; !remembered {
				r.restore[modal] = r.focus
			}
		}
	}
	if n := r.resolve(r.focus); n != nil && r.focusable(n) {
		return
	}
	var first Target
	r.walk(r.content.node, func(n *node) bool {
		if r.focusable(n) {
			first = n.target()
			return false
		}
		return true
	})
	r.setFocus(first)
}

func (r *Root) restoreFocus(previous *node) {
	if previous == nil || r.modalEligible(previous) {
		return
	}
	candidate, ok := r.restore[previous]
	if !ok {
		return
	}
	delete(r.restore, previous)
	if n := r.resolve(candidate); n != nil && r.focusable(n) {
		r.setFocus(candidate)
	}
}

func (r *Root) modalEligible(n *node) bool {
	if n == nil || n.root != r || n.group == nil || !n.group.config.Modal || len(n.group.items) == 0 {
		return false
	}
	return r.enabled(n)
}

// FocusNext is a traversal mechanism. Composite controllers may request a
// particular target instead to implement roving or validation-error policies.
func (r *Root) FocusNext(backward bool) bool {
	if r.mutable() != nil {
		return false
	}
	var targets []Target
	r.walk(r.content.node, func(n *node) bool {
		if r.focusable(n) {
			targets = append(targets, n.target())
		}
		return true
	})
	if len(targets) == 0 {
		return false
	}
	at := -1
	for i, target := range targets {
		if target == r.focus {
			at = i
			break
		}
	}
	switch {
	case backward && at < 0:
		at = len(targets) - 1
	case backward:
		at = (at - 1 + len(targets)) % len(targets)
	default:
		at = (at + 1) % len(targets)
	}
	r.setFocus(targets[at])
	return true
}

func (r *Root) walk(n *node, visit func(*node) bool) bool {
	if n == nil || n.root != r {
		return true
	}
	if !visit(n) {
		return false
	}
	if n.group != nil {
		for _, item := range n.group.items {
			if !r.walk(item.Element.node, visit) {
				return false
			}
		}
	}
	return true
}

// FlushNotifications runs settlement notifications after the operation that
// produced them has completed. Runtime owners call it between owner turns, never
// while a frame is prepared or admitted. Notifications may schedule new work.
func (r *Root) FlushNotifications() {
	if r == nil || r.preparing || r.observing || r.pending != nil {
		return
	}
	batch := r.notifications
	r.notifications = nil
	for _, notify := range batch {
		notify()
	}
}

// Changed registers one completed model transition and wakes observers.
// It panics when called during a pure phase or while a prepared candidate awaits settlement.
func (r *Root) Changed() {
	if r == nil || r.closed {
		return
	}
	if err := r.mutable(); err != nil {
		panic(err)
	}
	r.settleFocus()
	r.modelRevision = next(r.modelRevision)
	r.signal()
}

func (r *Root) signal() {
	if r.changes != nil {
		close(r.changes)
	}
	r.changes = make(chan struct{})
}

// Changes returns a broadcast channel closed by the next model, layout or write transition.
func (r *Root) Changes() <-chan struct{} {
	if r == nil {
		return nil
	}
	return r.changes
}

// Close revokes every attachment and clears routing state. Repeated calls have no effect.
// It panics during a pure phase or while a prepared candidate awaits settlement; abort that candidate first.
func (r *Root) Close() {
	if r == nil || r.closed {
		return
	}
	if err := r.mutable(); err != nil {
		panic(err)
	}
	if r.content.node != nil {
		r.detach(r.content.node)
	}
	r.content = Element{}
	r.closed = true
	r.capture = capture{}
	clear(r.restore)
	r.activeModal = nil
	r.layout, r.written, r.inFlight = nil, nil, nil
	r.published.Store(nil)
	r.signal()
	r.FlushNotifications()
}
