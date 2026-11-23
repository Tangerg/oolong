package scene

import (
	"fmt"
	"image"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/layout"
)

type Item struct {
	Key     string
	Size    layout.Sizing
	Element Element
	// Paint supplies this child's appearance while in this relation. Nil uses
	// the element's own painter. Descendants are traversed in either case.
	Paint func(Frame)
}

// GroupConfig arranges children in two independent respects. Placement is
// Layout when set, otherwise full overlapping bounds for an overlay, otherwise a
// flow along Axis with Gap. Layout is pure and returns exactly one rectangle per
// item in the group's coordinates. Measurement, unless Element.Measure overrides
// it, is the largest child for an overlay, the flow's wanted extent for a flow,
// and for a custom Layout that is not an overlay, the children's extents summed
// along Axis with Gap between them.
type GroupConfig struct {
	Element ElementConfig
	Axis    layout.Axis
	Gap     int
	Items   []Item
	Overlay bool
	Modal   bool
	Layout  func(image.Point, []Item) []image.Rectangle
}

// validateArrangement refuses a gap that no arrangement would use, rather than
// letting a measurement silently ignore it.
func (cfg GroupConfig) validateArrangement() error {
	switch {
	case cfg.Gap < 0:
		return fmt.Errorf("%w: negative group gap %d", ErrInvalid, cfg.Gap)
	case cfg.Overlay && cfg.Gap != 0:
		return fmt.Errorf("%w: an overlay group has no gap", ErrInvalid)
	default:
		return nil
	}
}

// Group owns one ordered child relation. A Group must not be copied after first use.
type Group struct {
	noCopy  noCopy
	element Element
	arrange arrangement
	items   []Item
}

// arrangement is the part of a GroupConfig a Group keeps; its element and items
// have owners of their own.
type arrangement struct {
	axis    layout.Axis
	gap     int
	overlay bool
	modal   bool
	layout  func(image.Point, []Item) []image.Rectangle
}

// NewGroup creates an ordered composition. It panics for an invalid axis or
// initial child relation, a negative gap, or a gap on an overlay.
func NewGroup(cfg GroupConfig) *Group {
	if err := cfg.Axis.Validate(); err != nil {
		panic(err)
	}
	if err := cfg.validateArrangement(); err != nil {
		panic(err)
	}
	g := &Group{arrange: arrangement{axis: cfg.Axis, gap: cfg.Gap, overlay: cfg.Overlay, modal: cfg.Modal, layout: cfg.Layout}}
	g.element = NewElement(cfg.Element)
	g.element.node.group = g
	if err := g.Set(cfg.Items...); err != nil {
		panic(err)
	}
	return g
}

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

// At reads one current child relation without copying the collection. A nil
// group or out-of-range index returns a zero Item and false.
func (g *Group) At(index int) (Item, bool) {
	if g == nil || index < 0 || index >= len(g.items) {
		return Item{}, false
	}
	return g.items[index], true
}

// CheckSet validates a proposed child relation without publishing it. Owners that
// compose several groups may preflight all relations before changing their model.
// It does not reserve children; each subsequent Set validates its current state.
func (g *Group) CheckSet(items ...Item) error {
	_, err := g.checkSet(items, nil)
	return err
}

func (g *Group) checkSet(items []Item, nextID *uint64) (map[*node]bool, error) {
	if g == nil || g.element.IsZero() {
		return nil, ErrInvalid
	}
	n := g.element.node
	if nextID == nil && n.root != nil {
		next := n.root.nextID
		nextID = &next
	}
	if err := g.element.CheckMutation(); err != nil {
		return nil, err
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
		if err := checkChild(n, child, nextID); err != nil {
			return nil, err
		}
	}
	return children, nil
}

func checkChild(parent, child *node, nextID *uint64) error {
	if err := (Element{node: child}).CheckMutation(); err != nil {
		return err
	}
	for ancestor := parent; ancestor != nil; ancestor = ancestor.parent {
		if ancestor == child {
			return ErrCycle
		}
	}
	if child.parent != nil && child.parent != parent || child.root != nil && child.parent != parent {
		return ErrMounted
	}
	if parent.root != nil && child.root == nil {
		return parent.root.checkAttachment(child, nextID)
	}
	return nil
}

// Set validates the complete relation before changing it. Reordering retained
// children keeps their attachment; removal immediately revokes its generation.
// Attaching a child whose previous callbacks have not settled returns [ErrCleanupPending].
func (g *Group) Set(items ...Item) error {
	children, err := g.checkSet(items, nil)
	if err != nil {
		return err
	}
	g.install(items, children)
	if r := g.element.node.root; r != nil {
		r.Changed()
	}
	return nil
}

func (g *Group) install(items []Item, children map[*node]bool) {
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
}

type GroupItems struct {
	Group *Group
	Items []Item
}

// CheckChildItems validates replacements for this group's direct child groups.
// It does not reserve children. Each group may appear once and each element may
// have only one parent. Moving between groups requires detachment first.
func (g *Group) CheckChildItems(updates ...GroupItems) error {
	_, err := g.checkChildItems(updates)
	return err
}

// SetChildItems replaces the specified sibling relations together. All checks
// finish before any detach or attach; focus predicates observe the complete tree.
// It accepts direct child groups of this parent only, including before mounting.
func (g *Group) SetChildItems(updates ...GroupItems) error {
	children, err := g.checkChildItems(updates)
	if err != nil {
		return err
	}
	for i, update := range updates {
		update.Group.install(update.Items, children[i])
	}
	if r := g.element.node.root; r != nil && len(updates) > 0 {
		r.Changed()
	}
	return nil
}

func (g *Group) checkChildItems(updates []GroupItems) ([]map[*node]bool, error) {
	if g == nil || g.element.IsZero() {
		return nil, ErrInvalid
	}
	if err := g.element.CheckMutation(); err != nil {
		return nil, err
	}
	parent := g.element.node
	var nextID uint64
	if parent.root != nil {
		nextID = parent.root.nextID
	}
	groups := make(map[*Group]bool, len(updates))
	claimed := make(map[*node]bool)
	children := make([]map[*node]bool, len(updates))
	for i, update := range updates {
		target := update.Group
		if target == nil || target.element.IsZero() || target.element.node.parent != parent {
			return nil, ErrInvalid
		}
		if groups[target] {
			return nil, ErrDuplicate
		}
		groups[target] = true
		relation, err := target.checkSet(update.Items, &nextID)
		if err != nil {
			return nil, err
		}
		for child := range relation {
			if claimed[child] {
				return nil, ErrDuplicate
			}
			claimed[child] = true
		}
		children[i] = relation
	}
	return children, nil
}

// Move changes a retained child's position without unmounting it.
func (g *Group) Move(from, to int) error {
	if g == nil || from < 0 || to < 0 || from >= len(g.items) || to >= len(g.items) {
		return ErrInvalid
	}
	items := g.Items()
	item := items[from]
	return g.Set(slices.Insert(slices.Delete(items, from, from+1), to, item)...)
}

func (g *Group) measure(axis layout.Axis, across int) int {
	if !g.arrange.overlay && g.arrange.layout == nil {
		flow, slots := g.flow()
		if axis == flow.Axis {
			return flow.Wanted(across, slots)
		}
		return flow.CrossWanted(across, slots)
	}
	if g.arrange.overlay || axis != g.arrange.axis {
		extent := 0
		for _, item := range g.items {
			extent = max(extent, item.Element.Measure(axis, across))
		}
		return extent
	}
	extent := 0
	for i, item := range g.items {
		if i != 0 {
			extent = layout.Sum(extent, g.arrange.gap)
		}
		extent = layout.Sum(extent, item.Element.Measure(axis, across))
	}
	return extent
}

func (g *Group) rectangles(space image.Point) []image.Rectangle {
	if g.arrange.layout != nil {
		rects := g.arrange.layout(space, g.Items())
		if len(rects) != len(g.items) {
			panic("scene: layout returned a different number of child rectangles")
		}
		// Nested layouts can reuse the same scratch slice during this traversal.
		return slices.Clone(rects)
	}
	if g.arrange.overlay {
		rects := make([]image.Rectangle, len(g.items))
		for i := range rects {
			rects[i] = image.Rectangle{Max: space}
		}
		return rects
	}
	flow, slots := g.flow()
	return flow.Rects(space, slots)
}

func (g *Group) flow() (layout.Flow, []layout.Slot) {
	slots := make([]layout.Slot, len(g.items))
	for i, item := range g.items {
		slots[i] = layout.Slot{Size: item.Size, Of: item.Element}
	}
	return layout.Flow{Axis: g.arrange.axis, Gap: g.arrange.gap}, slots
}
