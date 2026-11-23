package scene

import (
	"fmt"
	"sync/atomic"

	"github.com/Tangerg/oolong/ui/internal/counter"
)

const (
	defaultInputHistory     = 8
	defaultMaxPendingFrames = 8
)

// RootConfig supplies one scene and the bounded history used to resolve timestamped physical input.
type RootConfig struct {
	Element Element
	// InputHistory bounds admitted geometry retained for host-buffered mouse reports.
	// Already captured inputs retain their own immutable snapshot separately.
	InputHistory int
	// MaxPendingFrames bounds admitted input maps awaiting writer confirmation.
	// Zero selects the package default. Prepare returns ErrBusy at capacity,
	// before painting.
	MaxPendingFrames int
}

// Root is the sole owner of live attachment, actual focus, capture and geometry.
// Except CaptureInput and CapturePresentedInput, its operations run on the UI owner sequence.
// A Root must not be copied after first use.
type Root struct {
	noCopy                noCopy
	content               Element
	nodes                 map[NodeID]*node
	nextID                uint64
	modelRevision         uint64
	focus                 Target
	previousKeyboardRoute []Target
	capture               capture
	hover                 Target
	scopes                []focusScope
	changes               chan struct{}
	notifications         []lifecycleCall
	flushing              bool
	preparing             bool
	observing             bool
	closed                bool
	pending               *Prepared
	layout                *layoutSnapshot
	frames                frameLedger
	maxPendingFrames      int
	published             atomic.Pointer[inputGeometry]
	inputHistory          int
	geometryEpoch         *byte
	namespace             *namespace
	delivery              *byte
	inputTurn             *byte
	focusTurn             *byte
}

type namespace struct{ _ byte }

func NewRoot(cfg RootConfig) (*Root, error) {
	if cfg.InputHistory < 0 || cfg.MaxPendingFrames < 0 {
		return nil, ErrInvalid
	}
	r := &Root{nodes: make(map[NodeID]*node), changes: make(chan struct{}), namespace: &namespace{}}
	r.maxPendingFrames = cfg.MaxPendingFrames
	if r.maxPendingFrames == 0 {
		r.maxPendingFrames = defaultMaxPendingFrames
	}
	r.inputHistory = cfg.InputHistory
	if r.inputHistory == 0 {
		r.inputHistory = defaultInputHistory
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
	if r.settling() {
		return ErrBusy
	}
	return nil
}

// projecting reports a pure phase: a frame being painted or a read-only observation.
func (r *Root) projecting() bool { return r.preparing || r.observing }

// settling reports that the model is not the root's to change: it is being
// projected, or a prepared frame still awaits its commit or abort.
func (r *Root) settling() bool { return r.projecting() || r.pending != nil }

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
		successor, ok := counter.Next(*nextID)
		if !ok {
			return fmt.Errorf("%w: attachment identity exhausted", ErrInvalid)
		}
		*nextID = successor
	} else if _, ok := counter.Next(n.generation); !ok {
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
		r.setFocus(Target{}, nil)
	}
	delete(r.nodes, n.id)
	n.root = nil
	n.keyboardEpoch = new(byte)
	close(n.done)
	n.done = nil
	if r.capture.target == previous {
		held := r.capture
		r.capture.phase = captureRevoked
		if held.phase != captureRevoked && n.config.Handle != nil {
			r.notify(n, func() { held.cancel(n) })
		}
	}
	if r.hover == previous {
		r.revokeHover(n)
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

const (
	eligible eligibility = iota
	// disabled means the node's own or an ancestor's Enabled refuses it.
	disabled
	// outsideModal means an active modal layer does not contain the node.
	outsideModal
)

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
	if r == nil || r.flushing || r.settling() {
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
		// A callback may leave the next frame prepared. Its model must stay
		// unchanged until that candidate settles, including during this flush.
		if r.pending != nil {
			return
		}
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
	r.nodes = nil
	r.capture = capture{}
	r.scopes = nil
	r.previousKeyboardRoute = nil
	r.layout = nil
	r.frames.revoke()
	r.frames.shown = nil
	r.published.Store(nil)
	r.signal()
}
