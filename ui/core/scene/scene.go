// Package scene owns attachment identity, geometry, focus, pointer capture and
// semantic action routing. Elements compose independent capabilities; application
// state remains with the controller supplying those capabilities.
package scene

import (
	"errors"
	"fmt"
	"image"
	"math"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/layout"
)

// Scene errors distinguish attachment, eligibility and lifecycle failures.
var (
	ErrDetached    = errors.New("scene: element is not attached")
	ErrMounted     = errors.New("scene: element is already attached")
	ErrStale       = errors.New("scene: target attachment has expired")
	ErrBusy        = errors.New("scene: mutation during presentation or observation")
	ErrClosed      = errors.New("scene: root is closed")
	ErrUnavailable = errors.New("scene: target is unavailable")
	ErrDuplicate   = errors.New("scene: duplicate child or key")
	ErrCycle       = errors.New("scene: attachment cycle")
	ErrInvalid     = errors.New("scene: invalid configuration")
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
	Node       NodeID
	Generation Generation
	Part       PartID
	root       *Root
}

// IsZero reports whether this handle names no attachment.
func (t Target) IsZero() bool { return t.root == nil || t.Node == 0 }

// Valid is an owner-turn query, not a concurrency-safe lifetime lease.
func (t Target) Valid() bool { return t.root != nil && t.root.resolve(t) != nil }

// Element is a copyable handle. The retained node owns its attachment; a Group
// owns its child sequence. Neither handle exposes a second editable tree.
type Element struct{ node *node }

// ElementConfig composes independent painting, input, semantics and lifecycle capabilities. Projection callbacks must be pure.
type ElementConfig struct {
	Key          string
	Part         PartID
	Measure      func(layout.Axis, int) int
	Paint        func(Frame)
	Preview      func(Event) EventResult
	Handle       func(Event) EventResult
	Focusable    func() bool
	Enabled      func() bool
	Semantics    func() Semantic
	Actions      []ActionBinding
	FocusChanged func(bool)
	Detached     func()
}

type node struct {
	config       ElementConfig
	group        *Group
	parent       *node
	root         *Root
	identityRoot *namespace
	id           NodeID
	generation   Generation
	done         chan struct{}
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

// IsZero reports whether this is the empty element handle.
func (e Element) IsZero() bool { return e.node == nil }

// Key returns the application's stable locator key.
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

// Alive reports whether the element currently belongs to a scene.
func (e Element) Alive() bool { return !e.Target().IsZero() }

// Focused reports whether this exact element owns scene keyboard focus.
func (e Element) Focused() bool {
	return e.node != nil && e.node.root != nil && e.node.root.focus == e.node.target()
}

// FocusedWithin reports whether this element or one of its descendants owns scene keyboard focus.
func (e Element) FocusedWithin() bool {
	if e.node == nil || e.node.root == nil {
		return false
	}
	for n := e.node.root.resolve(e.node.root.focus); n != nil; n = n.parent {
		if n == e.node {
			return true
		}
	}
	return false
}

// RequestFocusFirst focuses the first eligible node within this composition.
func (e Element) RequestFocusFirst() error {
	if e.node == nil || e.node.root == nil {
		return ErrDetached
	}
	r := e.node.root
	var target Target
	r.walk(e.node, func(n *node) bool {
		if r.focusable(n) {
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

// Target returns the exact generation associated with this lifetime lease.
func (a Attachment) Target() Target { return a.target }

// Done closes synchronously when this attachment is revoked. An empty lease is already done.
func (a Attachment) Done() <-chan struct{} {
	if a.done == nil {
		return detached
	}
	return a.done
}

// RequestFocus asks this element's scene to assign it keyboard focus.
func (e Element) RequestFocus() error {
	if e.node == nil || e.node.root == nil {
		return ErrDetached
	}
	return e.node.root.RequestFocus(e.node.target())
}

// Invalidate announces a completed controller mutation. It is not a setter for
// semantic values: Observe obtains those directly from the controller.
func (e Element) Invalidate() {
	if e.node != nil && e.node.root != nil {
		e.node.root.Changed()
	}
}

// CheckMutation lets a behavior owner reject a prohibited phase before changing
// its own model. An unattached model can be edited without a running scene.
func (e Element) CheckMutation() error {
	if e.node == nil || e.node.root == nil {
		return nil
	}
	return e.node.root.mutable()
}

// SetPaint fills an unmounted behavior part's visual slot. After mounting, its
// painter is fixed; changing appearance is data consumed by that same painter.
func (e Element) SetPaint(paint func(Frame)) error {
	if e.node == nil {
		return ErrInvalid
	}
	if e.node.root != nil {
		return ErrMounted
	}
	e.node.config.Paint = paint
	return nil
}

// SetMeasure fills the measurement slot before attachment. The callback must be a pure projection.
func (e Element) SetMeasure(measure func(layout.Axis, int) int) error {
	if e.node == nil {
		return ErrInvalid
	}
	if e.node.root != nil {
		return ErrMounted
	}
	e.node.config.Measure = measure
	return nil
}

// SetFocusable configures keyboard eligibility before attachment. Composite
// controls may keep keyboard focus on an input while a list remains pointer
// interactive. The predicate must be pure; the scene still owns actual focus.
func (e Element) SetFocusable(focusable func() bool) error {
	if e.node == nil {
		return ErrInvalid
	}
	if e.node.root != nil {
		return ErrMounted
	}
	e.node.config.Focusable = focusable
	return nil
}

// Measure returns the nonnegative intrinsic extent, delegating group composition when no callback is supplied.
func (e Element) Measure(axis layout.Axis, across int) int {
	if e.node == nil {
		return 0
	}
	if measure := e.node.config.Measure; measure != nil {
		return max(0, measure(axis, max(0, across)))
	}
	if e.node.group != nil {
		return e.node.group.measure(axis, max(0, across))
	}
	return 0
}

func (n *node) target() Target {
	return Target{Node: n.id, Generation: n.generation, Part: n.config.Part, root: n.root}
}

// Item is one keyed child relation and its layout request.
type Item struct {
	Key     string
	Size    layout.Sizing
	Element Element
}

// GroupConfig selects child geometry. Layout is a pure variation point. Its returned rectangles
// are in the group's coordinates and must have exactly one entry per item.
// Nil uses Flow, or overlapping full bounds when Overlay is true.
type GroupConfig struct {
	Element ElementConfig
	Axis    layout.Axis
	Gap     int
	Items   []Item
	Overlay bool
	Modal   bool
	Layout  func(image.Point, []Item) []image.Rectangle
}

// Group owns one ordered child relation. A Group must not be copied after first use.
type Group struct {
	noCopy  noCopy
	element Element
	config  GroupConfig
	items   []Item
}

// NewGroup creates an ordered composition. It panics for an invalid initial child relation.
func NewGroup(cfg GroupConfig) *Group {
	g := &Group{config: cfg}
	g.config.Items = nil
	g.config.Element = ElementConfig{}
	g.element = NewElement(cfg.Element)
	g.element.node.group = g
	if err := g.Set(cfg.Items...); err != nil {
		panic(err)
	}
	return g
}

// Element returns the group's stable opaque scene node.
func (g *Group) Element() Element {
	if g == nil {
		return Element{}
	}
	return g.element
}

// Items returns a detached copy of the ordered child relation.
func (g *Group) Items() []Item {
	if g == nil {
		return nil
	}
	return slices.Clone(g.items)
}

// CheckSet validates a proposed child relation without publishing it. Owners that
// compose several groups may preflight all relations before changing their model.
// It does not reserve children; each subsequent Set validates its current state.
func (g *Group) CheckSet(items ...Item) error {
	_, err := g.checkSet(items)
	return err
}

func (g *Group) checkSet(items []Item) (map[*node]bool, error) {
	if g == nil {
		return nil, ErrInvalid
	}
	n := g.element.node
	if n.root != nil {
		if err := n.root.mutable(); err != nil {
			return nil, err
		}
	}
	keys := make(map[string]bool, len(items))
	children := make(map[*node]bool, len(items))
	for _, item := range items {
		child := item.Element.node
		if child == nil {
			return nil, fmt.Errorf("%w: empty child", ErrInvalid)
		}
		if children[child] || item.Key != "" && keys[item.Key] {
			return nil, ErrDuplicate
		}
		children[child], keys[item.Key] = true, true
		if err := checkChild(n, child); err != nil {
			return nil, err
		}
	}
	return children, nil
}

func checkChild(parent, child *node) error {
	for ancestor := parent; ancestor != nil; ancestor = ancestor.parent {
		if ancestor == child {
			return ErrCycle
		}
	}
	if child.parent != nil && child.parent != parent || child.root != nil && child.parent != parent {
		return ErrMounted
	}
	if parent.root != nil && child.root == nil {
		return parent.root.canAttach(child)
	}
	return nil
}

// Set validates the complete relation before changing it. Reordering retained
// children keeps their attachment; removal immediately revokes its generation.
func (g *Group) Set(items ...Item) error {
	children, err := g.checkSet(items)
	if err != nil {
		return err
	}
	n := g.element.node
	previous := g.items
	g.items = slices.Clone(items)
	for i := range g.items {
		g.items[i].Key = strings.Clone(g.items[i].Key)
	}
	for _, item := range previous {
		child := item.Element.node
		if !children[child] {
			if n.root != nil {
				n.root.detach(child)
			}
			child.parent = nil
		}
	}
	for _, item := range g.items {
		child := item.Element.node
		child.parent = n
		if n.root != nil && child.root == nil {
			n.root.attach(child)
		}
	}
	if n.root != nil {
		n.root.settleFocus()
		n.root.Changed()
	}
	return nil
}

// Move changes a retained child's position without unmounting it.
func (g *Group) Move(from, to int) error {
	if g == nil || from < 0 || to < 0 || from >= len(g.items) || to >= len(g.items) {
		return ErrInvalid
	}
	items := g.Items()
	item := items[from]
	if from < to {
		copy(items[from:to], items[from+1:to+1])
	} else {
		copy(items[to+1:from+1], items[to:from])
	}
	items[to] = item
	return g.Set(items...)
}

func (g *Group) measure(axis layout.Axis, across int) int {
	if g.config.Overlay || axis != g.config.Axis {
		extent := 0
		for _, item := range g.items {
			extent = max(extent, item.Element.Measure(axis, across))
		}
		return extent
	}
	extent := 0
	for i, item := range g.items {
		if i != 0 {
			extent = layout.Sum(extent, max(0, g.config.Gap))
		}
		extent = layout.Sum(extent, item.Element.Measure(axis, across))
	}
	return extent
}

func (g *Group) rectangles(space image.Point) []image.Rectangle {
	if g.config.Layout != nil {
		rects := g.config.Layout(space, g.Items())
		if len(rects) != len(g.items) {
			panic("scene: layout returned a different number of child rectangles")
		}
		return rects
	}
	if g.config.Overlay {
		rects := make([]image.Rectangle, len(g.items))
		for i := range rects {
			rects[i] = image.Rectangle{Max: space}
		}
		return rects
	}
	slots := make([]layout.Slot, len(g.items))
	for i, item := range g.items {
		slots[i] = layout.Slot{Size: item.Size, Of: item.Element}
	}
	return (layout.Flow{Axis: g.config.Axis, Gap: g.config.Gap}).Rects(space, slots)
}

func next(value uint64) uint64 {
	if value == math.MaxUint64 {
		panic("scene: identity or revision exhausted")
	}
	return value + 1
}
