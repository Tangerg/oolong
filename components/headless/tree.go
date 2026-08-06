package headless

import (
	"slices"
	"strconv"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
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
	// Nodes are the items at the top, in order.
	Nodes []Node[T]
	// Row draws one row. at is where it sits among the rows on screen and selected
	// says whether it is the one under the cursor.
	Row func(v grid.View, at int, row Shown[T], selected bool)
	// Keys say which keystrokes produce which of the actions the tree answers to —
	// see [Tree.Do]. Nil reads through [DefaultTreeKeys].
	Keys *input.Keymap

	// list is the rows the tree is showing. It owns the selection and the scroll,
	// and this type owns which rows there are.
	list List[Shown[T]]
	// open is which branches are showing what is under them, by path.
	open map[string]bool
	// pending is how far into a multi-chord binding the keys typed so far have got.
	pending input.Pending
}

// Rows are the rows the tree is showing, top to bottom, as a copy.
//
// A copy because this is the only way out of the tree and the tree rebuilds its own
// rows on every call: handing out the buffer it builds them in would hand out
// something that changes under the caller the next time anything is drawn. Nothing
// inside asks for this — see [Tree.rows], which is the same answer without the copy
// — so the allocation happens only when somebody outside wants to look.
func (t *Tree[T]) Rows() []Shown[T] { return slices.Clone(t.rows()) }

// rows rebuilds what the tree is showing.
//
// From the nodes every time, because the nodes are the caller's and may have changed
// between two frames. Which branches are open is remembered here, so a tree that is
// refreshed keeps the shape the reader gave it.
func (t *Tree[T]) rows() []Shown[T] {
	rows := t.list.Items[:0]
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
	walk(t.Nodes, 0, "")
	t.list.SetItems(rows)
	return rows
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
	t.rows()
	return t.list.Current()
}

// Select moves the cursor to a row, clamped to what is showing.
func (t *Tree[T]) Select(at int) {
	t.rows()
	t.list.Select(at)
}

// Open shows what is under the row at, and reports whether that changed anything:
// a leaf, or a branch that was already open, changes nothing.
func (t *Tree[T]) Open(at int) bool { return t.set(at, true) }

// Close hides it again, and reports the same.
func (t *Tree[T]) Close(at int) bool { return t.set(at, false) }

// set opens or closes a row.
func (t *Tree[T]) set(at int, open bool) bool {
	rows := t.rows()
	if at < 0 || at >= len(rows) || !rows[at].Branch || rows[at].Open == open {
		return false
	}
	if t.open == nil {
		t.open = map[string]bool{}
	}
	t.open[rows[at].path] = open
	t.rows()
	return true
}

// Handle answers keys, the wheel and a press, reporting whether it consumed the
// event.
func (t *Tree[T]) Handle(ev input.Event) bool {
	t.rows()
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
func (t *Tree[T]) Do(action input.Action) bool {
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
	rows := t.rows()
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
	rows := t.rows()
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

// Measure is one row per row showing, which is what a container needs to decide how
// much room to give it.
func (t *Tree[T]) Measure(int) int { return len(t.rows()) }

// Scroll exposes the position, for a scrollbar drawn beside the tree.
func (t *Tree[T]) Scroll() *Scroll { return t.list.Scroll() }

// Draw paints the rows that fit.
func (t *Tree[T]) Draw(v grid.View) {
	t.rows()
	t.list.Row = t.row
	t.list.Draw(v)
}

// row hands one row to whoever knows what a row looks like.
func (t *Tree[T]) row(v grid.View, at int, shown Shown[T], selected bool) {
	if t.Row != nil {
		t.Row(v, at, shown, selected)
	}
}

// keys is the map to read through, standing in the default for a caller who set
// none.
func (t *Tree[T]) keys() *input.Keymap {
	if t.Keys != nil {
		return t.Keys
	}
	return treeKeys()
}
