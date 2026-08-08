package headless

import (
	"slices"
	"strconv"

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

	// path is which node this is, by position: "0.3.1" is the second child of the
	// fourth child of the first item. It is what remembers which branches are open
	// across a rebuild of the tree, and it follows position rather than identity
	// because identity is something only the caller has.
	//
	// It stays inside: a row carries the item, and where the item came from is the
	// caller's to know from the item. Handing out a decoded path would be handing out
	// this key, and then it could not be changed.
	path string
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
	// settlement or retain open paths that no longer name a branch.
	nodes []Node[T]
	// Row draws one row. at is where it sits among the rows on screen and selected
	// says whether it is the one under the cursor.
	Row func(v grid.View, at int, row Shown[T], selected bool)
	// Keys say which keystrokes produce which of the actions the tree answers to —
	// see [Tree.Do]. Nil reads through [DefaultTreeKeys].
	Keys *keymap.Map

	// list is the rows the tree is showing. It owns the selection and the scroll,
	// and this type owns which rows there are.
	list List[Shown[T]]
	// open is which branches are showing what is under them, by path.
	open map[string]bool
	// pending is how far into a multi-chord binding the keys typed so far have got.
	pending keymap.Pending
}

// NewTree constructs a tree from top-level nodes in display order.
func NewTree[T any](nodes ...Node[T]) *Tree[T] {
	t := &Tree[T]{}
	t.SetNodes(nodes)
	return t
}

// SetNodes replaces the hierarchy. The tree recursively copies the node collection;
// the caller may reuse or change every input slice after the call. Item values remain
// caller-owned. Open branches follow their positional paths where those paths still
// name branches, and selection stays on the same visible row when possible.
func (t *Tree[T]) SetNodes(nodes []Node[T]) {
	if t == nil {
		return
	}
	t.nodes = cloneNodes(nodes)
	t.rebuild()
}

// Nodes returns a recursive copy of the hierarchy.
func (t *Tree[T]) Nodes() []Node[T] {
	if t == nil {
		return nil
	}
	return cloneNodes(t.nodes)
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
	t.pruneOpen()
	rows := make([]Shown[T], 0, len(t.list.items))
	var walk func(nodes []Node[T], depth int, prefix string)
	walk = func(nodes []Node[T], depth int, prefix string) {
		for i, node := range nodes {
			path := prefix + strconv.Itoa(i)
			open := t.open[path]
			rows = append(rows, Shown[T]{
				Item:   node.Item,
				Depth:  depth,
				Branch: len(node.Children) > 0,
				Open:   open && len(node.Children) > 0,
				path:   path,
			})
			if open {
				walk(node.Children, depth+1, path+".")
			}
		}
	}
	walk(t.nodes, 0, "")
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
		t.open = map[string]bool{}
	}
	if open {
		t.open[rows[at].path] = true
	} else {
		delete(t.open, rows[at].path)
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
	action, mine := t.keys().Lookup(key, &t.pending)
	switch {
	case !mine:
		return false
	case action == "":
		return true // the start of a binding more than one chord long
	}
	return t.Do(action)
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
func (t *Tree[T]) Focus(has bool) { t.list.Focus(has) }

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

// pruneOpen bounds retained expansion state to branches the current hierarchy can
// express. Descendants of a closed branch remain valid so reopening their parent can
// restore the shape the reader left; paths removed from the hierarchy do not linger.
func (t *Tree[T]) pruneOpen() {
	if len(t.open) == 0 {
		return
	}
	branches := make(map[string]struct{}, len(t.open))
	var walk func(nodes []Node[T], prefix string)
	walk = func(nodes []Node[T], prefix string) {
		for i, node := range nodes {
			path := prefix + strconv.Itoa(i)
			if len(node.Children) > 0 {
				branches[path] = struct{}{}
				walk(node.Children, path+".")
			}
		}
	}
	walk(t.nodes, "")
	for path := range t.open {
		if _, ok := branches[path]; !ok {
			delete(t.open, path)
		}
	}
	if len(t.open) == 0 {
		t.open = nil
	}
}

func cloneNodes[T any](nodes []Node[T]) []Node[T] {
	if len(nodes) == 0 {
		return nil
	}
	cloned := make([]Node[T], len(nodes))
	for i, node := range nodes {
		cloned[i] = Node[T]{Item: node.Item, Children: cloneNodes(node.Children)}
	}
	return cloned
}

// keys is the map to read through, standing in the default for a caller who set
// none.
func (t *Tree[T]) keys() *keymap.Map {
	if t.Keys != nil {
		return t.Keys
	}
	return treeKeys()
}
