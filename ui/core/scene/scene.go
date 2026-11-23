// Package scene owns attachment identity, geometry, focus, pointer capture and
// semantic action routing. Elements compose independent capabilities; application
// state remains with the controller supplying those capabilities.
package scene

import (
	"errors"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/internal/counter"
)

var (
	ErrDetached = errors.New("scene: element is not attached")
	ErrMounted  = errors.New("scene: element is already attached")
	// ErrCleanupPending means the previous attachment still has lifecycle
	// callbacks to finish. Flush the previous root's notifications, then retry.
	ErrCleanupPending = errors.New("scene: previous attachment cleanup pending")
	ErrStale          = errors.New("scene: target attachment has expired")
	ErrBusy           = errors.New("scene: mutation during presentation or observation")
	ErrClosed         = errors.New("scene: root is closed")
	ErrUnavailable    = errors.New("scene: target is unavailable")
	ErrDuplicate      = errors.New("scene: duplicate child or key")
	ErrCycle          = errors.New("scene: attachment cycle")
	ErrInvalid        = errors.New("scene: invalid configuration")
)

type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// NodeID identifies a retained node within one scene namespace.
type (
	NodeID uint64
	// Generation distinguishes successive attachments of the same node.
	Generation uint64
	// PartID identifies a logical part within a component.
	PartID string
)

// Target names one attachment in one root. Copying it does not extend that
// attachment's lifetime; a later element with the same key is a different target.
type Target struct {
	node       NodeID
	generation Generation
	part       PartID
	root       *Root
}

func (t Target) Node() NodeID { return t.node }

func (t Target) Generation() Generation { return t.generation }

func (t Target) Part() PartID { return t.part }

func (t Target) IsZero() bool { return t.root == nil || t.node == 0 }

// Valid is an owner-turn query, not a concurrency-safe lifetime lease.
func (t Target) Valid() bool { return t.root != nil && t.root.resolve(t) != nil }

// Element is a copyable handle. The retained node owns its attachment; a Group
// owns its child sequence. Neither handle exposes a second editable tree.
type Element struct{ node *node }

// ElementConfig composes independent painting, input, semantics and lifecycle capabilities. Projection callbacks must be pure.
type ElementConfig struct {
	Key     string
	Part    PartID
	Measure layout.MeasureFunc
	Paint   func(Frame)
	// Shortcut sees every key bound for anything inside this element first, root
	// first, before any Preview. Unlike Preview and Handle, it still runs while a
	// modal layer inside this element holds the keyboard: an element that contains a
	// layer is not covered by it. It is for commands that belong to the whole
	// composition, such as quitting or cancelling work, which must keep working
	// behind a dialog.
	Shortcut  func(Event) EventResult
	Preview   func(Event) EventResult
	Handle    func(Event) EventResult
	Focusable func() bool
	// Autofocus marks where keyboard focus starts when its layer settles focus
	// with nothing focused: the first focusable element, in tree order, inside an
	// autofocus element, ahead of every focusable element outside one. An editor
	// below a scrolling log, or a form below the document it concerns, starts
	// with the keys while what precedes it stays focusable.
	Autofocus    bool
	Enabled      func() bool
	Semantics    func() Semantic
	Actions      []ActionBinding
	FocusChanged func(bool)
	Detached     func()
}

type node struct {
	config        ElementConfig
	group         *Group
	parent        *node
	root          *Root
	identityRoot  *namespace
	id            NodeID
	generation    Generation
	done          chan struct{}
	keyboardEpoch *byte
	observing     bool
	// lifecycle counts callbacks queued or running for this node's generation.
	lifecycle int
}

// NewElement creates an unmounted opaque node. It panics for an empty or duplicated action identifier.
func NewElement(cfg ElementConfig) Element {
	seen := make(map[ActionID]bool, len(cfg.Actions))
	for _, action := range cfg.Actions {
		if action.ID() == "" || seen[action.ID()] {
			panic("scene: empty or duplicate action identifier")
		}
		seen[action.ID()] = true
	}
	cfg.Key = strings.Clone(cfg.Key)
	cfg.Part = PartID(strings.Clone(string(cfg.Part)))
	cfg.Actions = slices.Clone(cfg.Actions)
	return Element{node: &node{config: cfg}}
}

// Passive configures an element for passive content: it draws content and measures
// its height for a width, and takes neither input nor focus. Callers add a Key or
// semantics to the returned configuration before [NewElement]. It panics for absent
// content, which would draw nothing and measure as nothing in silence.
func Passive(content grid.Drawable) ElementConfig {
	if grid.IsNilDrawable(content) {
		panic("scene: passive element requires content")
	}
	return ElementConfig{
		Measure: layout.MeasureRows(content.HeightForWidth),
		Paint:   func(frame Frame) { content.Draw(frame.View) },
	}
}

func (e Element) IsZero() bool { return e.node == nil }

func (e Element) Key() string {
	if e.node == nil {
		return ""
	}
	return e.node.config.Key
}

// Target returns the current attachment identity, or zero when unmounted.
func (e Element) Target() Target {
	if e.node == nil || e.node.root == nil {
		return Target{}
	}
	return e.node.target()
}

func (e Element) Alive() bool { return !e.Target().IsZero() }

// HasBeenAttached distinguishes a standalone element from one whose scene lifetime
// ended. Zero and never-mounted elements return false; detachment keeps it true.
// Like Target, this is an owner-turn query, not a concurrency-safe lifetime lease.
func (e Element) HasBeenAttached() bool { return e.node != nil && e.node.identityRoot != nil }

func (e Element) Focused() bool {
	return e.node != nil && e.node.root != nil && e.node.root.focus == e.node.target()
}

func (e Element) FocusedWithin() bool {
	if e.node == nil || e.node.root == nil {
		return false
	}
	return containsNode(e.node, e.node.root.resolve(e.node.root.focus))
}

// KeyboardLease binds input to its receiver and a continuous keyboard route.
// A receiver on that route owns its subtree's lifetime; a delegated receiver
// borrows the current origin's lifetime. With no focus, the active modal or root
// content is the origin. Leaving that route or remounting never revives a lease.
// Standalone input lasts until attachment. Queries belong to the UI owner sequence.
type KeyboardLease struct {
	element Element
	target  Target
	scope   Target
	epoch   *byte
}

func (e Element) KeyboardLease() KeyboardLease {
	if e.node == nil {
		return KeyboardLease{}
	}
	lease := KeyboardLease{element: e, target: e.Target(), epoch: e.node.keyboardEpoch}
	if r := e.node.root; r != nil {
		route := r.keyboardRoute(r.focus, r.modal())
		if len(route) == 0 {
			return KeyboardLease{}
		}
		lease.scope = lease.target
		if !slices.Contains(route, lease.scope) {
			lease.scope = route[0]
		}
		scope := r.resolve(lease.scope)
		if scope == nil {
			return KeyboardLease{}
		}
		lease.epoch = scope.keyboardEpoch
	}
	return lease
}

func (l KeyboardLease) Valid() bool {
	n := l.element.node
	if n == nil {
		return false
	}
	if l.target.IsZero() {
		return n.root == nil && n.keyboardEpoch == l.epoch
	}
	r := l.target.root
	scope := r.resolve(l.scope)
	if r.resolve(l.target) != n || scope == nil || scope.keyboardEpoch != l.epoch {
		return false
	}
	return slices.Contains(r.keyboardRoute(r.focus, r.modal()), l.scope)
}

// FocusNext lets a deferred key operation move focus forward only while its
// original keyboard scope still owns input.
func (l KeyboardLease) FocusNext() bool {
	return l.target.root != nil && l.Valid() && l.target.root.FocusNext()
}

// FocusPrev is [KeyboardLease.FocusNext] in the other direction.
func (l KeyboardLease) FocusPrev() bool {
	return l.target.root != nil && l.Valid() && l.target.root.FocusPrev()
}

func (e Element) RequestFocusFirst() error {
	if e.node == nil || e.node.root == nil {
		return ErrDetached
	}
	r := e.node.root
	if err := r.mutable(); err != nil {
		return err
	}
	var target Target
	active := r.modal()
	r.walk(e.node, func(n *node) bool {
		if r.focusableIn(n, active) {
			target = n.target()
			return false
		}
		return true
	})
	if target.IsZero() {
		return ErrUnavailable
	}
	return r.RequestFocus(target)
}

// Attachment is an immutable generation lease. Done closes synchronously before
// detachment notifications; it is safe to observe from a worker goroutine.
type Attachment struct {
	target Target
	done   <-chan struct{}
}

var detached = func() <-chan struct{} { done := make(chan struct{}); close(done); return done }()

// Attachment returns an immutable lifetime lease for this mount generation.
func (e Element) Attachment() Attachment {
	if e.node == nil || e.node.root == nil {
		return Attachment{}
	}
	return Attachment{target: e.node.target(), done: e.node.done}
}

func (a Attachment) Target() Target { return a.target }

// Done closes synchronously when this attachment is revoked. An empty lease is already done.
func (a Attachment) Done() <-chan struct{} {
	if a.done == nil {
		return detached
	}
	return a.done
}

func (e Element) RequestFocus() error {
	if e.node == nil || e.node.root == nil {
		return ErrDetached
	}
	return e.node.root.RequestFocus(e.node.target())
}

// Invalidate announces a completed controller mutation. It is not a setter for
// semantic values: Observe obtains those directly from the controller.
// It panics during a read-only projection or presentation.
func (e Element) Invalidate() {
	if err := e.CheckMutation(); err != nil {
		panic(err)
	}
	if e.node != nil && e.node.root != nil {
		e.node.root.Changed()
	}
}

// CheckMutation lets a behavior owner reject a prohibited phase before changing
// its own model, including a read-only borrow before the composition is mounted.
func (e Element) CheckMutation() error {
	if e.node == nil {
		return nil
	}
	if e.node.root != nil {
		return e.node.root.mutable()
	}
	if e.detachedOwner().observing {
		return ErrBusy
	}
	return nil
}

// CheckProjection validates a controller's preparation boundary. Its element
// must be mounted in the frame's scene; ordinary Prepare may stage geometry,
// while a read-only projection or an unsettled candidate may not advance it.
func (e Element) CheckProjection(frame Frame) error {
	if e.node == nil {
		return ErrDetached
	}
	if e.node.root == nil {
		if e.detachedOwner().observing {
			return ErrBusy
		}
		return ErrDetached
	}
	if e.node.root.observing || e.node.root.pending != nil {
		return ErrBusy
	}
	if err := frame.checkProjection(); err != nil {
		return err
	}
	if e.node.root != frame.lease.tx.root {
		return ErrInvalid
	}
	return nil
}

// ReadOnly borrows the composition for a synchronous projection. Controller and
// attachment mutations are refused until read returns, including before mounting.
// Nested borrows preserve the enclosing phase. Panic and Goexit release the borrow
// before propagating. It panics for an empty element, nil callback, or an unsettled
// frame candidate; settle that candidate before entering another projection.
func (e Element) ReadOnly(read func()) {
	if e.node == nil || read == nil {
		panic(ErrInvalid)
	}
	e.project(func() struct{} { read(); return struct{}{} })
}

func (e Element) project[T any](read func() T) T {
	if e.node.root != nil {
		return e.node.root.project(read)
	}
	owner := e.detachedOwner()
	previous := owner.observing
	owner.observing = true
	defer func() { owner.observing = previous }()
	return read()
}

func (e Element) detachedOwner() *node {
	owner := e.node
	for owner.parent != nil {
		owner = owner.parent
	}
	return owner
}

// SetPaint fills an unmounted behavior part's visual slot. After mounting, its
// painter is fixed; changing appearance is data consumed by that same painter.
// It panics for a zero or mounted element, or during presentation or observation.
func (e Element) SetPaint(paint func(Frame)) {
	e.configure().Paint = paint
}

// configure is the one rule every configuration slot obeys: it can be filled only
// on an unmounted element outside a read-only phase. Filling a slot is assembly, so
// a zero element, a mounted one or a slot filled during presentation is a program
// error and panics with [ErrInvalid], [ErrMounted] or [ErrBusy].
func (e Element) configure() *ElementConfig {
	if e.node == nil {
		panic(ErrInvalid)
	}
	if err := e.CheckMutation(); err != nil {
		panic(err)
	}
	if e.node.root != nil {
		panic(ErrMounted)
	}
	return &e.node.config
}

// SetMeasure fills the measurement slot before attachment. The callback must be a
// pure projection. It panics as [Element.SetPaint] does.
func (e Element) SetMeasure(measure layout.MeasureFunc) {
	e.configure().Measure = measure
}

// SetAutofocus marks the element as where focus starts in its layer, as
// [ElementConfig.Autofocus] does. It panics as [Element.SetPaint] does.
func (e Element) SetAutofocus(autofocus bool) {
	e.configure().Autofocus = autofocus
}

// SetFocusable configures keyboard eligibility before attachment. Composite
// controls may keep keyboard focus on an input while a list remains pointer
// interactive; nil makes the element never focusable. The predicate must be pure;
// the scene still owns actual focus. It panics as [Element.SetPaint] does.
func (e Element) SetFocusable(focusable func() bool) {
	e.configure().Focusable = focusable
}

// Measure returns the nonnegative intrinsic extent, delegating group composition when no callback is supplied.
// It panics before calling application code for an invalid axis or while its scene has an unsettled frame candidate.
func (e Element) Measure(axis layout.Axis, across int) int {
	if err := axis.Validate(); err != nil {
		panic(err)
	}
	if e.node == nil {
		return 0
	}
	return e.project(func() int {
		if measure := e.node.config.Measure; measure != nil {
			return max(0, measure(axis, max(0, across)))
		}
		if e.node.group != nil {
			return e.node.group.measure(axis, max(0, across))
		}
		return 0
	})
}

func (n *node) target() Target {
	return Target{node: n.id, generation: n.generation, part: n.config.Part, root: n.root}
}

func next(value uint64) uint64 {
	successor, ok := counter.Next(value)
	if !ok {
		panic("scene: identity or revision exhausted")
	}
	return successor
}
