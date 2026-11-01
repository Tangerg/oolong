package scene

import (
	"fmt"
	"math"
	"slices"
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
	// MaxPendingFrames bounds admitted input maps awaiting writer confirmation.
	// Zero allows eight. Prepare returns ErrBusy at capacity, before painting.
	MaxPendingFrames int
	Now              func() time.Time
}

// Root is the sole owner of live attachment, actual focus, capture and geometry.
// Except CaptureInput, its operations run on the UI owner sequence.
// A Root must not be copied after first use.
type Root struct {
	noCopy           noCopy
	content          Element
	nodes            map[NodeID]*node
	nextID           uint64
	modelRevision    uint64
	focus            Target
	capture          capture
	hover            Target
	scopes           []focusScope
	changes          chan struct{}
	notifications    []lifecycleCall
	flushing         bool
	preparing        bool
	observing        bool
	closed           bool
	pending          *Prepared
	layout           *layoutSnapshot
	written          *layoutSnapshot
	inFlight         []*layoutSnapshot
	maxPendingFrames int
	lastSequence     uint64
	writtenSequence  uint64
	published        atomic.Pointer[writtenHistory]
	inputHistory     int
	now              func() time.Time
	geometryEpoch    uint64
	namespace        *namespace
	delivery         uint64
	inputTurn        uint64
	focusTurn        uint64
}

type namespace struct{ _ byte }

// NewRoot mounts one content element and establishes its initial focus.
func NewRoot(cfg RootConfig) (*Root, error) {
	r := &Root{nodes: make(map[NodeID]*node), changes: make(chan struct{})}
	r.namespace = &namespace{}
	if cfg.InputHistory < 0 || cfg.MaxPendingFrames < 0 {
		return nil, ErrInvalid
	}
	r.maxPendingFrames = cfg.MaxPendingFrames
	if r.maxPendingFrames == 0 {
		r.maxPendingFrames = 8
	}
	r.inputHistory = cfg.InputHistory
	if r.inputHistory == 0 {
		r.inputHistory = 8
	}
	r.now = cfg.Now
	if r.now == nil {
		r.now = time.Now
	}
	// A focus predicate can observe a provisional attachment and then unwind
	// the constructor. Revoke its lease without publishing lifecycle callbacks
	// from a root that never became available to its owner.
	mounted := false
	defer func() {
		if !mounted {
			r.revoke()
			for _, call := range r.notifications {
				call.release()
			}
			r.notifications = nil
		}
	}()
	if err := r.SetContent(cfg.Element); err != nil {
		return nil, err
	}
	mounted = true
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
// It returns [ErrCleanupPending] if a previous generation still has callbacks to deliver.
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
	r.Changed()
	return nil
}

func (r *Root) canAttach(n *node) error {
	nextID := r.nextID
	return r.checkAttachment(n, &nextID)
}

func (r *Root) checkAttachment(n *node, nextID *uint64) error {
	if err := (Element{node: n}).CheckMutation(); err != nil {
		return err
	}
	if n.root != nil {
		return ErrMounted
	}
	if n.lifecycle != 0 {
		return ErrCleanupPending
	}
	if n.identityRoot != r.namespace {
		if *nextID == math.MaxUint64 {
			return fmt.Errorf("%w: attachment identity exhausted", ErrInvalid)
		}
		*nextID++
	} else if n.generation == Generation(math.MaxUint64) {
		return fmt.Errorf("%w: attachment identity exhausted", ErrInvalid)
	}
	if n.group != nil {
		for _, item := range n.group.items {
			if err := r.checkAttachment(item.Element.node, nextID); err != nil {
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
			r.notify(n, func() { held.cancel(n) })
		}
	}
	if r.hover == previous {
		r.hover = Target{}
		if n.config.Handle != nil {
			r.notify(n, func() { n.config.Handle(Event{input: input.Mouse{Action: input.MouseLeave}, target: previous}) })
		}
	}
	if n.group != nil {
		for _, item := range n.group.items {
			r.detach(item.Element.node)
		}
	}
	if n.config.Detached != nil {
		r.notify(n, n.config.Detached)
	}
}

func (r *Root) resolve(target Target) *node {
	if r == nil || r.closed || target.root != r {
		return nil
	}
	n := r.nodes[target.node]
	if n == nil || n.generation != target.generation || n.config.Part != target.part {
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
	if r.pending != nil {
		panic(ErrBusy)
	}
	return r.readOnly(read)
}

func (r *Root) readOnly[T any](read func() T) T {
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
	return n != nil && r.allowedIn(n, r.modal())
}

func (r *Root) allowedIn(n, modal *node) bool {
	if !r.enabled(n) {
		return false
	}
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

func (r *Root) walkModals(visit func(*node)) {
	r.walk(r.content.node, func(n *node) bool {
		if n.group != nil && n.group.config.Modal && len(n.group.items) != 0 && r.enabled(n) {
			visit(n)
		}
		return true
	})
}

func (r *Root) modal() *node {
	var current *node
	r.walkModals(func(n *node) { current = n })
	return current
}

func (r *Root) focusableIn(n, modal *node) bool {
	return r.allowedIn(n, modal) && n.config.Focusable != nil && r.project(n.config.Focusable)
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
	if !r.focusableIn(n, r.modal()) {
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
	if old := r.nodes[previous.node]; previous.root == r && old != nil && old.generation == previous.generation {
		if notify := old.config.FocusChanged; notify != nil {
			r.notify(old, func() { notify(false) })
		}
	}
	if n := r.resolve(target); n != nil && n.config.FocusChanged != nil {
		notify := n.config.FocusChanged
		turn := r.focusTurn
		r.notify(n, func() {
			if r.focusTurn == turn {
				notify(true)
			}
		})
	}
	r.modelRevision = next(r.modelRevision)
	r.signal()
}

// Each modal layer remembers the focus to restore when that layer disappears.
type focusScope struct{ modal, opener Target }

func (r *Root) settleFocus() {
	var modals []Target
	var active *node
	r.walkModals(func(n *node) {
		active = n
		modals = append(modals, n.target())
	})
	common := 0
	for common < len(modals) && common < len(r.scopes) && modals[common] == r.scopes[common].modal {
		common++
	}
	if common < len(r.scopes) || common < len(modals) {
		// A layer's last focus is the next layer's opener, or the current focus
		// for the top layer. Carry it by attachment when layers move. Removing a
		// covered layer splices its opener into the surviving return path.
		within := make(map[Target]Target, len(r.scopes)-common)
		opener := r.focus
		for i := len(r.scopes) - 1; i >= common; i-- {
			scope := r.scopes[i]
			within[scope.modal] = opener
			opener = scope.opener
		}
		r.scopes = slices.Delete(r.scopes, common, len(r.scopes))
		for _, modal := range modals[common:] {
			r.scopes = append(r.scopes, focusScope{modal: modal, opener: opener})
			opener = within[modal]
		}
		if opener != r.focus {
			if n := r.resolve(opener); n != nil && r.focusableIn(n, active) {
				r.setFocus(opener)
			}
		}
	}
	if n := r.resolve(r.focus); n != nil && r.focusableIn(n, active) {
		return
	}
	var first Target
	r.walk(r.content.node, func(n *node) bool {
		if r.focusableIn(n, active) {
			first = n.target()
			return false
		}
		return true
	})
	r.setFocus(first)
}

// FocusNext is a traversal mechanism. Composite controllers may request a
// particular target instead to implement roving or validation-error policies.
func (r *Root) FocusNext(backward bool) bool {
	if r.mutable() != nil {
		return false
	}
	var targets []Target
	active := r.modal()
	r.walk(r.content.node, func(n *node) bool {
		if r.focusableIn(n, active) {
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

// A queued lifecycle callback still owns access to its controller even after
// detach revokes routing. Hold that generation through callback return, including
// reentrant mutation, panic and Goexit; other nodes have no such obligation.
type lifecycleCall struct {
	owner *node
	run   func()
}

func (r *Root) notify(n *node, run func()) {
	n.lifecycle++
	r.notifications = append(r.notifications, lifecycleCall{owner: n, run: run})
}

func (c lifecycleCall) release() { c.owner.lifecycle-- }

func (c lifecycleCall) deliver() {
	defer c.release()
	c.run()
}

// FlushNotifications runs settlement notifications after the operation that
// produced them has completed. Runtime owners call it between owner turns, never
// while a frame is prepared or admitted. Notifications may schedule new work.
func (r *Root) FlushNotifications() {
	if r == nil || r.flushing || r.preparing || r.observing || r.pending != nil {
		return
	}
	r.flushing = true
	defer func() {
		r.flushing = false
		// A closed runtime may never have another owner turn. Even while
		// unwinding a failed callback, finish every revoked generation's cleanup.
		if r.closed && len(r.notifications) != 0 {
			r.FlushNotifications()
		}
	}()
	for remaining := len(r.notifications); len(r.notifications) > 0 && (remaining > 0 || r.closed); remaining-- {
		// Claim only this callback. A panic or Goexit must not discard its
		// successors, and a recursive flush must not overtake them.
		call := r.notifications[0]
		r.notifications[0] = lifecycleCall{}
		r.notifications = r.notifications[1:]
		if len(r.notifications) == 0 {
			r.notifications = nil
		}
		call.deliver()
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
	// Focus predicates can exit abnormally; the installed model must already
	// have its new observation revision and wakeup before they run.
	r.modelRevision = next(r.modelRevision)
	r.signal()
	r.settleFocus()
}

func (r *Root) signal() {
	if r.changes != nil {
		close(r.changes)
	}
	if !r.closed {
		r.changes = make(chan struct{})
	}
}

// Changes returns a broadcast channel closed by the next model, layout or write transition. It stays closed after Close.
func (r *Root) Changes() <-chan struct{} {
	if r == nil {
		return nil
	}
	return r.changes
}

// Close revokes every attachment and clears routing state. Every pending cleanup runs even if another callback exits abnormally.
// It panics during a pure phase or while a prepared candidate awaits settlement; abort that candidate first.
func (r *Root) Close() {
	if r == nil {
		return
	}
	if !r.closed {
		if err := r.mutable(); err != nil {
			panic(err)
		}
		r.revoke()
	}
	r.FlushNotifications()
}

func (r *Root) revoke() {
	if r.content.node != nil {
		r.detach(r.content.node)
	}
	r.content = Element{}
	r.closed = true
	r.capture = capture{}
	r.scopes = nil
	r.layout, r.written, r.inFlight = nil, nil, nil
	r.published.Store(nil)
	r.signal()
}
