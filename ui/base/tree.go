package base

import (
	"slices"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
)

type Node[T any] struct {
	Item     T
	Children []Node[T]
}

type Shown[T any] struct {
	Item T
	// Ancestor depth starts at zero for roots; visible row order does not affect it.
	Depth        int
	Branch, Open bool
}

type treeNode[T any] struct {
	item     T
	children []treeNode[T]
	open     bool
}

// Visible rows borrow their node; detached public snapshots retain no hierarchy.
type treeRow[T any] struct {
	node  *treeNode[T]
	depth int
}

func (r treeRow[T]) shown() Shown[T] {
	return Shown[T]{Item: r.node.item, Depth: r.depth, Branch: len(r.node.children) > 0, Open: r.node.open}
}

// Tree is a list of items with items under them, which can be opened and closed.
//
// The items are the caller's shape — see [Node] — and how a row looks is the
// caller's too: a tree that drew its own indentation would have decided what a
// branch mark is, which is a matter of taste and belongs a layer up. What is here is
// which branches are open, which row the cursor is on, and what the keys do.
//
// The zero Tree shows nothing and answers nothing. A Tree must not be copied after
// first use: its owned hierarchy, rows and matcher are one mutable
// owner.
// Rebuilding rows panics before mutation if the collection revision is exhausted.
type Tree[T any] struct {
	element scene.Element
	noCopy  noCopy

	nodes []treeNode[T]
	// Row draws one row. at is where it sits among the rows on screen and selected
	// says whether it is the one under the cursor. It runs during drawing and must be
	// an observationally pure projection; state changes belong in event handlers.
	rowPainter func(v grid.View, at int, row Shown[T], selected bool)
	// Keys say which keystrokes produce which of the actions the tree answers to —
	// see [Tree.Do]. Nil reads through [DefaultTreeKeys].
	keys keyBinding

	list    List[treeRow[T]]
	matcher keymap.Matcher
}

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
	if t != nil {
		requireMutable(t.element)
	}
	if t == nil {
		return
	}
	t.list.requireReplacement()
	t.matcher.Reset(func() {
		t.replaceNodes(nodes)
		t.rebuild(nil)
	})
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
	if t == nil || t.list.items == nil {
		return nil
	}
	rows := make([]Shown[T], len(t.list.items))
	for i, row := range t.list.items {
		rows[i] = row.shown()
	}
	return rows
}

// rebuild is the sole transition from the owned hierarchy and open state into the
// visible list. Drawing reads that settled list and therefore cannot clamp selection
// or otherwise change semantic state.
func (t *Tree[T]) rebuild(selectedNode *treeNode[T]) {
	rows := make([]treeRow[T], 0, len(t.list.items))
	selected := t.list.selected
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
		if node == selectedNode {
			selected = len(rows)
		}
		rows = append(rows, treeRow[T]{node: node, depth: current.depth})
		if !node.open {
			continue
		}
		for i := range slices.Backward(node.children) {
			stack = append(stack, pending{node: &node.children[i], depth: current.depth + 1})
		}
	}
	// The tree routes keys through its own matcher, which every caller resets
	// around a rebuild; the inner list never matches keys.
	t.list.replaceItems(rows, selected)
	t.list.notifyChange()
}

// Selected is the row the cursor is on, or -1 when the tree is showing nothing.
func (t *Tree[T]) Selected() int { return t.list.Selected() }

func (t *Tree[T]) Current() (T, bool) {
	row, ok := t.CurrentRow()
	return row.Item, ok
}

// CurrentRow is the whole row under the cursor and whether there is one: what a
// caller asks when it needs more than the item — whether it can be opened, how deep
// it sits, where it came from.
func (t *Tree[T]) CurrentRow() (Shown[T], bool) {
	row, ok := t.list.Current()
	if !ok {
		return Shown[T]{}, false
	}
	return row.shown(), true
}

// Select moves the cursor to a row, clamped to what is showing.
func (t *Tree[T]) Select(at int) {
	t.list.Select(at)
}

// Open and Close preserve the selected identity so changing another branch cannot
// retarget the next action. A hidden descendant moves onto its closing ancestor.
func (t *Tree[T]) Open(at int) bool { return t.set(at, true) }

func (t *Tree[T]) Close(at int) bool { return t.set(at, false) }

func (t *Tree[T]) set(at int, open bool) bool {
	requireMutable(t.element)
	rows := t.list.items
	if at < 0 || at >= len(rows) || len(rows[at].node.children) == 0 || rows[at].node.open == open {
		return false
	}
	selected := t.Selected()
	if !open {
		for i := at + 1; i < len(rows) && rows[i].depth > rows[at].depth; i++ {
			if i == selected {
				selected = at
				break
			}
		}
	}
	selectedNode := rows[selected].node
	t.list.requireReplacement()
	t.matcher.Reset(func() {
		rows[at].node.open = open
		t.rebuild(selectedNode)
	})
	return true
}

// Handle answers keys, the wheel and a press, reporting whether it consumed the
// event.
func (t *Tree[T]) Handle(ev input.Event) bool {
	requireMutable(t.element)
	if _, ok := ev.(input.Mouse); ok {
		return t.list.Handle(ev)
	}
	key, ok := ev.(input.Key)
	if !ok {
		return false
	}
	_, handled := t.matcher.Handle(t.resolvedKeys(), key, keyboardReceipt(&t.element), t.Do)
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

func (t *Tree[T]) into(at int) bool {
	rows := t.list.items
	if at < 0 || at+1 >= len(rows) || rows[at+1].depth <= rows[at].depth {
		return false
	}
	t.list.Select(at + 1)
	return true
}

func (t *Tree[T]) upToParent(at int) bool {
	rows := t.list.items
	if at < 0 || at >= len(rows) || rows[at].depth == 0 {
		return false
	}
	for i := at - 1; i >= 0; i-- {
		if rows[i].depth < rows[at].depth {
			t.list.Select(i)
			return true
		}
	}
	return false
}

func (t *Tree[T]) focusChanged(has bool) {
	change := func() {
		t.list.focusChanged(has)
	}
	if has {
		change()
	} else {
		t.matcher.Reset(change)
	}
}

func (t *Tree[T]) Focused() bool { return t.list.Focused() }

func (t *Tree[T]) HeightForWidth(int) int { return t.list.Len() }

func (t *Tree[T]) Scroll() *Scroll { return t.list.Scroll() }

func (t *Tree[T]) Draw(v scene.Frame) {
	t.DrawRows(v, t.rowPainter)
}

// DrawRows paints the rows that fit with draw.
//
// Like [List.DrawRows], it lets an appearance compose with the controller without
// replacing Row. Selection, scrolling and committed pointer geometry remain owned by
// the tree; only the appearance of this frame is supplied by the caller.
func (t *Tree[T]) DrawRows(v scene.Frame, draw func(grid.View, int, Shown[T], bool)) {
	var paint func(grid.View, int, treeRow[T], bool)
	if draw != nil {
		paint = func(view grid.View, at int, row treeRow[T], selected bool) {
			draw(view, at, row.shown(), selected)
		}
	}
	t.list.DrawRows(v, paint)
}

// replaceNodes takes ownership of a hierarchy and reconciles it with the previous
// positional snapshot. The explicit frame stack serves two purposes: arbitrary
// valid depth cannot exhaust the call stack, and active source slices make a cycle
// observable before it can become unbounded work.
func (t *Tree[T]) replaceNodes(nodes []Node[T]) {
	if len(nodes) == 0 {
		t.nodes = nil
		return
	}
	t.nodes = copyTree(nodes, t.nodes)
}

// treeSlice identifies one caller-owned child slice by its first element and
// length, which is what makes revisiting it detectable.
type treeSlice[T any] struct {
	first  *Node[T]
	length int
}

// treeCopy owns one iterative transfer from caller-owned nodes into the tree. Its
// active slice identities make a cycle an explicit state transition instead of an
// accidental exhaustion of stack or heap.
type treeCopy[T any] struct {
	active map[treeSlice[T]]struct{}
	stack  []treeCopyFrame[T]
}

type treeCopyFrame[T any] struct {
	source []Node[T]
	old    []treeNode[T]
	target []treeNode[T]
	at     int
	key    treeSlice[T]
}

func copyTree[T any](source []Node[T], old []treeNode[T]) []treeNode[T] {
	target := make([]treeNode[T], len(source))
	rootKey := treeSlice[T]{&source[0], len(source)}
	copying := treeCopy[T]{
		active: map[treeSlice[T]]struct{}{rootKey: {}},
		stack: []treeCopyFrame[T]{
			{source: source, old: old, target: target, key: rootKey},
		},
	}
	copying.run()
	return target
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
	current.target[i] = treeNode[T]{item: source.Item}
	if len(source.Children) == 0 {
		return
	}
	var oldChildren []treeNode[T]
	if i < len(current.old) {
		previous := &current.old[i]
		current.target[i].open = previous.open
		oldChildren = previous.children
	}
	key := treeSlice[T]{&source.Children[0], len(source.Children)}
	if _, cyclic := c.active[key]; cyclic {
		panic("base: cyclic tree node collection")
	}
	children := make([]treeNode[T], len(source.Children))
	current.target[i].children = children
	c.active[key] = struct{}{}
	c.stack = append(c.stack, treeCopyFrame[T]{
		source: source.Children, old: oldChildren, target: children, key: key,
	})
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

func (t *Tree[T]) resolvedKeys() *keymap.Map { return t.keys.resolve(treeKeys) }

// Subscribe observes the same completed visible collection and selection owned by List.
func (t *Tree[T]) Subscribe(callback func()) func() { return t.list.Subscribe(callback) }

func (t *Tree[T]) Row() func(grid.View, int, Shown[T], bool) { return t.rowPainter }

func (t *Tree[T]) SetRow(value func(grid.View, int, Shown[T], bool)) {
	element := t.Element()
	requireMutable(element)
	t.rowPainter = value
	element.Invalidate()
}

func (t *Tree[T]) Keys() *keymap.Map { return t.keys.current }

func (t *Tree[T]) SetKeys(keys *keymap.Map) {
	t.keys.set(t.Element(), &t.matcher, keys)
}
