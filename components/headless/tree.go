package headless

import (
	"slices"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
)

// Node is one item of a [Tree] and whatever is under it.
type Node[T any] struct {
	Item     T
	Children []Node[T]
}

// Shown is one row a [Tree] is showing: an item, how deep it sits, and what can be
// done with it.
//
// A tree is a list of these — which is not a simplification but the whole design.
// Everything a tree does that is not opening and closing is what a list does, and
// building it on one is what keeps the selection, the scrolling, the wheel and the
// click from being written a second time and getting the edges wrong.
type Shown[T any] struct {
	Item T
	// Depth is how many items are above this one, from zero at the top.
	Depth int
	// Branch says there is something under this item, and Open says it is showing.
	// A branch that is closed is what the reader is being invited to open, and a
	// branch with nothing under it is not a branch.
	Branch, Open bool

	// id is the tree-owned identity of this position. SetNodes reuses it for the node
	// at the same position, which is how open branches survive a refresh without
	// asking T to be comparable or retaining a path whose size grows with depth.
	//
	// It stays inside: a row carries the item, and where the item came from is the
	// caller's to know from the item. Handing out this key would make an internal
	// reconciliation mechanism part of the component contract.
	id uint64
}

// treeNode is the hierarchy after Tree has taken ownership. Identity belongs here
// rather than on Node: it exists only to reconcile two positional snapshots, and an
// application cannot usefully supply or observe it.
type treeNode[T any] struct {
	item     T
	children []treeNode[T]
	id       uint64
}

// Tree is a list of items with items under them, which can be opened and closed.
//
// The items are the caller's shape — see [Node] — and how a row looks is the
// caller's too: a tree that drew its own indentation would have decided what a
// branch mark is, which is a matter of taste and belongs a layer up. What is here is
// which branches are open, which row the cursor is on, and what the keys do.
//
// The zero Tree shows nothing and answers nothing.
type Tree[T any] struct {
	// nodes are private so replacing the hierarchy cannot bypass selection
	// settlement or retain open identities that no longer name a branch.
	nodes []treeNode[T]
	// Row draws one row. at is where it sits among the rows on screen and selected
	// says whether it is the one under the cursor. It runs during drawing and must be
	// an observationally pure projection; state changes belong in event handlers.
	Row func(v grid.View, at int, row Shown[T], selected bool)
	// Keys say which keystrokes produce which of the actions the tree answers to —
	// see [Tree.Do]. Nil reads through [DefaultTreeKeys].
	Keys *keymap.Map

	// list is the rows the tree is showing. It owns the selection and the scroll,
	// and this type owns which rows there are.
	list List[Shown[T]]
	// open is which owned branch identities are showing what is under them.
	open map[uint64]bool
	// nodeIDs owns the stable node namespace. Zero is reserved for no node, which
	// makes accidental empty identities impossible to retain as open state.
	nodeIDs identitySequence
	// matcher owns how far into a multi-chord binding the keys have got.
	matcher keymap.Matcher
}

// NewTree constructs a tree from top-level nodes in display order.
func NewTree[T any](nodes ...Node[T]) *Tree[T] {
	t := &Tree[T]{}
	t.SetNodes(nodes)
	return t
}

// SetNodes replaces the hierarchy. The tree copies the entire node collection; the
// caller may reuse or change every input slice after the call. Item values remain
// caller-owned. Open branches follow their positions where those positions still
// name branches, and selection stays on the same visible row when possible.
//
// A Node graph must be acyclic. Cyclic slice graphs are a programmer error and
// panic here instead of making an ownership copy run until the stack or heap is
// exhausted. Traversal itself is iterative, so valid depth is limited by available
// storage rather than the goroutine stack.
func (t *Tree[T]) SetNodes(nodes []Node[T]) {
	if t == nil {
		return
	}
	t.replaceNodes(nodes)
	t.rebuild()
}

// Nodes returns a complete copy of the hierarchy.
func (t *Tree[T]) Nodes() []Node[T] {
	if t == nil {
		return nil
	}
	return exportNodes(t.nodes)
}

// Rows are the rows the tree is showing, top to bottom, as a copy.
//
// A copy because the visible-row buffer belongs to the tree and changes when nodes
// are replaced or branches open and close. The allocation happens only when somebody
// outside asks for a snapshot; drawing reads the owned buffer directly.
func (t *Tree[T]) Rows() []Shown[T] {
	if t == nil {
		return nil
	}
	return slices.Clone(t.list.items)
}

// rebuild is the sole transition from the owned hierarchy and open state into the
// visible list. Drawing reads that settled list and therefore cannot clamp selection
// or otherwise change semantic state.
func (t *Tree[T]) rebuild() {
	rows := make([]Shown[T], 0, len(t.list.items))
	type pending struct {
		node  *treeNode[T]
		depth int
	}
	stack := make([]pending, 0, len(t.nodes))
	for i := range slices.Backward(t.nodes) {
		stack = append(stack, pending{node: &t.nodes[i]})
	}
	for len(stack) > 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		node := current.node
		branch := len(node.children) > 0
		open := branch && t.open[node.id]
		rows = append(rows, Shown[T]{
			Item: node.item, Depth: current.depth, Branch: branch, Open: open, id: node.id,
		})
		if !open {
			continue
		}
		for i := range slices.Backward(node.children) {
			stack = append(stack, pending{node: &node.children[i], depth: current.depth + 1})
		}
	}
	t.list.SetItems(rows)
}

// Selected is the row the cursor is on, or -1 when the tree is showing nothing.
func (t *Tree[T]) Selected() int { return t.list.Selected() }

// Current is the item under the cursor and whether there is one.
func (t *Tree[T]) Current() (T, bool) {
	row, ok := t.CurrentRow()
	return row.Item, ok
}

// CurrentRow is the whole row under the cursor and whether there is one: what a
// caller asks when it needs more than the item — whether it can be opened, how deep
// it sits, where it came from.
func (t *Tree[T]) CurrentRow() (Shown[T], bool) {
	return t.list.Current()
}

// Select moves the cursor to a row, clamped to what is showing.
func (t *Tree[T]) Select(at int) {
	t.list.Select(at)
}

// Open shows what is under the row at, and reports whether that changed anything:
// a leaf, or a branch that was already open, changes nothing.
func (t *Tree[T]) Open(at int) bool { return t.set(at, true) }

// Close hides it again, and reports the same.
func (t *Tree[T]) Close(at int) bool { return t.set(at, false) }

// set opens or closes a row.
func (t *Tree[T]) set(at int, open bool) bool {
	rows := t.list.items
	if at < 0 || at >= len(rows) || !rows[at].Branch || rows[at].Open == open {
		return false
	}
	if t.open == nil {
		t.open = map[uint64]bool{}
	}
	if open {
		t.open[rows[at].id] = true
	} else {
		delete(t.open, rows[at].id)
	}
	t.rebuild()
	return true
}

// Handle answers keys, the wheel and a press, reporting whether it consumed the
// event.
func (t *Tree[T]) Handle(ev input.Event) bool {
	if _, ok := ev.(input.Mouse); ok {
		return t.list.Handle(ev)
	}
	key, ok := ev.(input.Key)
	if !ok {
		return false
	}
	_, handled := t.matcher.Handle(t.keys(), key, t.Do)
	return handled
}

// Do runs one of the tree's actions by name, reporting whether it was one this tree
// knows. See [Doer].
//
// Everything about moving through the rows is the list's, because that is what it
// is. What is the tree's is opening and closing — and the two edges that make a
// tree feel like a tree: opening a leaf does nothing, and closing something already
// closed goes up to whatever it is under.
func (t *Tree[T]) Do(action keymap.Action) bool {
	at := t.Selected()
	switch action {
	case Expand:
		if !t.Open(at) {
			// Nothing to open. Stepping into it is what a reader means by pressing it
			// again, and standing still is what a leaf does.
			return t.into(at)
		}
		return true
	case Collapse:
		if !t.Close(at) {
			return t.upToParent(at)
		}
		return true
	case Toggle:
		if row, ok := t.CurrentRow(); ok && row.Open {
			return t.Close(at)
		}
		return t.Open(at)
	default:
		return t.list.Do(action)
	}
}

// into steps to the first row under an open branch.
func (t *Tree[T]) into(at int) bool {
	rows := t.list.items
	if at < 0 || at+1 >= len(rows) || rows[at+1].Depth <= rows[at].Depth {
		return false
	}
	t.list.Select(at + 1)
	return true
}

// upToParent moves the cursor to whatever the row at is under.
//
// It is found by walking back to the first row less deep, which is what the rows
// already say: the parent of a row is the nearest one above it with a smaller depth.
func (t *Tree[T]) upToParent(at int) bool {
	rows := t.list.items
	if at < 0 || at >= len(rows) || rows[at].Depth == 0 {
		return false
	}
	for i := at - 1; i >= 0; i-- {
		if rows[i].Depth < rows[at].Depth {
			t.list.Select(i)
			return true
		}
	}
	return false
}

// Focus takes the keyboard, or gives it up — see [List.Focus], which is where the
// rows this is made of hold it.
func (t *Tree[T]) Focus(has bool) {
	if !has {
		t.matcher.Clear()
	}
	t.list.Focus(has)
}

// Focused reports whether this tree has the keyboard.
func (t *Tree[T]) Focused() bool { return t.list.Focused() }

// Measure is one row per row showing, which is what a container needs to decide how
// much room to give it.
func (t *Tree[T]) Measure(int) int { return t.list.Len() }

// Scroll exposes the position, for a scrollbar drawn beside the tree.
func (t *Tree[T]) Scroll() *Scroll { return t.list.Scroll() }

// Draw paints the rows that fit.
func (t *Tree[T]) Draw(v Frame) {
	t.DrawRows(v, t.Row)
}

// DrawRows paints the rows that fit with draw.
//
// Like [List.DrawRows], it lets an appearance compose with the controller without
// replacing Row. Selection, scrolling and committed pointer geometry remain owned by
// the tree; only the appearance of this frame is supplied by the caller.
func (t *Tree[T]) DrawRows(v Frame, draw func(grid.View, int, Shown[T], bool)) {
	t.list.DrawRows(v, draw)
}

// replaceNodes takes ownership of a hierarchy and reconciles it with the previous
// positional snapshot. The explicit frame stack serves two purposes: arbitrary
// valid depth cannot exhaust the call stack, and active source slices make a cycle
// observable before it can become unbounded work.
func (t *Tree[T]) replaceNodes(nodes []Node[T]) {
	if len(nodes) == 0 {
		t.nodes = nil
		t.open = nil
		return
	}
	owned, nodeIDs, branches := copyTree(nodes, t.nodes, t.nodeIDs, len(t.open))
	t.retainOpenBranches(branches)
	t.nodes, t.nodeIDs = owned, nodeIDs
}

func (t *Tree[T]) retainOpenBranches(branches map[uint64]struct{}) {
	for id := range t.open {
		if _, ok := branches[id]; !ok {
			delete(t.open, id)
		}
	}
	if len(t.open) == 0 {
		t.open = nil
	}
}

// treeCopy owns one iterative transfer from caller-owned nodes into the tree. Its
// active slice identities make a cycle an explicit state transition instead of an
// accidental exhaustion of stack or heap.
type treeCopy[T any] struct {
	ids      identitySequence
	branches map[uint64]struct{}
	active   map[*Node[T]]struct{}
	stack    []treeCopyFrame[T]
}

type treeCopyFrame[T any] struct {
	source []Node[T]
	old    []treeNode[T]
	target []treeNode[T]
	at     int
	key    *Node[T]
}

func copyTree[T any](
	source []Node[T],
	old []treeNode[T],
	ids identitySequence,
	branchCapacity int,
) ([]treeNode[T], identitySequence, map[uint64]struct{}) {
	target := make([]treeNode[T], len(source))
	rootKey := &source[0]
	copying := treeCopy[T]{
		ids:      ids,
		branches: make(map[uint64]struct{}, branchCapacity),
		active:   map[*Node[T]]struct{}{rootKey: {}},
		stack: []treeCopyFrame[T]{
			{source: source, old: old, target: target, key: rootKey},
		},
	}
	copying.run()
	return target, copying.ids, copying.branches
}

func (c *treeCopy[T]) run() {
	for len(c.stack) > 0 {
		current := &c.stack[len(c.stack)-1]
		if current.at == len(current.source) {
			delete(c.active, current.key)
			c.stack = c.stack[:len(c.stack)-1]
			continue
		}
		c.copyNext(current)
	}
}

func (c *treeCopy[T]) copyNext(current *treeCopyFrame[T]) {
	i := current.at
	current.at++
	source := current.source[i]
	id, oldChildren := c.identityAt(current.old, i)
	current.target[i] = treeNode[T]{item: source.Item, id: id}
	if len(source.Children) == 0 {
		return
	}
	c.branches[id] = struct{}{}
	key := &source.Children[0]
	if _, cyclic := c.active[key]; cyclic {
		panic("headless: cyclic tree node collection")
	}
	children := make([]treeNode[T], len(source.Children))
	current.target[i].children = children
	c.active[key] = struct{}{}
	c.stack = append(c.stack, treeCopyFrame[T]{
		source: source.Children, old: oldChildren, target: children, key: key,
	})
}

func (c *treeCopy[T]) identityAt(old []treeNode[T], index int) (uint64, []treeNode[T]) {
	if index < len(old) {
		return old[index].id, old[index].children
	}
	id, ok := c.ids.next()
	if !ok {
		panic("headless: tree exhausted node identities")
	}
	return id, nil
}

// exportNodes copies the owned hierarchy without making valid depth a call-stack
// limit. The internal graph is acyclic by construction, so no second cycle check is
// needed at this boundary.
func exportNodes[T any](nodes []treeNode[T]) []Node[T] {
	if len(nodes) == 0 {
		return nil
	}
	exported := make([]Node[T], len(nodes))
	type frame struct {
		source []treeNode[T]
		target []Node[T]
	}
	stack := []frame{{source: nodes, target: exported}}
	for len(stack) > 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		for i := range current.source {
			source := &current.source[i]
			current.target[i].Item = source.item
			if len(source.children) == 0 {
				continue
			}
			children := make([]Node[T], len(source.children))
			current.target[i].Children = children
			stack = append(stack, frame{source: source.children, target: children})
		}
	}
	return exported
}

// keys is the map to read through, standing in the default for a caller who set
// none.
func (t *Tree[T]) keys() *keymap.Map {
	if t.Keys != nil {
		return t.Keys
	}
	return treeKeys()
}
