package headless_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

// files is a small tree to walk about in.
func files() []headless.Node[string] {
	return []headless.Node[string]{
		{Item: "core", Children: []headless.Node[string]{
			{Item: "grid"},
			{Item: "term", Children: []headless.Node[string]{{Item: "pty"}}},
		}},
		{Item: "README"},
	}
}

// shape is what the tree is showing, one row per line, indented by depth.
func shape(t *headless.Tree[string]) string {
	var b strings.Builder
	for i, row := range t.Rows() {
		if i > 0 {
			b.WriteString("/")
		}
		b.WriteString(strings.Repeat(" ", row.Depth))
		b.WriteString(row.Item)
	}
	return b.String()
}

func TestATreeShowsWhatHasBeenOpened(t *testing.T) {
	tree := &headless.Tree[string]{Nodes: files()}
	if got := shape(tree); got != "core/README" {
		t.Fatalf("a tree nobody opened shows %q", got)
	}

	tree.Open(0)
	if got := shape(tree); got != "core/ grid/ term/README" {
		t.Fatalf("opening the first shows %q", got)
	}
	tree.Open(2)
	if got := shape(tree); got != "core/ grid/ term/  pty/README" {
		t.Fatalf("opening what is inside it shows %q", got)
	}
	tree.Close(0)
	if got := shape(tree); got != "core/README" {
		t.Fatalf("closing it again shows %q", got)
	}
	// And what was open inside it is still open, so opening it again does not undo
	// the reader's work.
	tree.Open(0)
	if got := shape(tree); got != "core/ grid/ term/  pty/README" {
		t.Fatalf("opening it again shows %q", got)
	}
}

func TestATreeKeepsItsShapeWhenTheItemsAreReplaced(t *testing.T) {
	// A file tree is refreshed while the reader is looking at it. Everything they
	// opened closing again is the difference between a tree and a list of paths.
	tree := &headless.Tree[string]{Nodes: files()}
	tree.Open(0)
	tree.Nodes = files()
	if got := shape(tree); got != "core/ grid/ term/README" {
		t.Fatalf("after the items were replaced the tree shows %q", got)
	}
}

func TestTheArrowsMoveThroughATreeTheWayATreeMoves(t *testing.T) {
	tree := &headless.Tree[string]{Nodes: files()}
	paintWidget(12, 6, tree)

	right := input.Key{Code: input.Right}
	left := input.Key{Code: input.Left}

	// Right opens; right again steps into what was opened.
	tree.Handle(right)
	tree.Handle(right)
	if got, _ := tree.Current(); got != "grid" {
		t.Fatalf("two rights left the cursor on %q", got)
	}
	// Left on something with nothing to close goes up to what it is under.
	tree.Handle(left)
	if got, _ := tree.Current(); got != "core" {
		t.Fatalf("left from a leaf left the cursor on %q", got)
	}
	// And left again closes it.
	tree.Handle(left)
	if got := shape(tree); got != "core/README" {
		t.Fatalf("left on an open branch shows %q", got)
	}
	// A tree answers to the names as well as to the keys, which is what lets one be
	// driven from a menu or from a test that presses nothing.
	if !tree.Do(headless.Toggle) || shape(tree) != "core/ grid/ term/README" {
		t.Fatalf("toggling shows %q", shape(tree))
	}
}

func TestATreeDrawsTheRowsThatFit(t *testing.T) {
	tree := &headless.Tree[string]{
		Nodes: files(),
		Row: func(v grid.View, _ int, row headless.Shown[string], selected bool) {
			mark := " "
			if selected {
				mark = ">"
			}
			v.Text(0, 0, mark+strings.Repeat(" ", row.Depth)+row.Item, grid.Style{})
		},
	}
	tree.Open(0)
	rows := paintWidget(10, 3, tree)
	equalRows(t, rows, []string{">core.....", "  grid....", "  term...."})
	if tree.Measure(10) != 4 {
		t.Fatalf("a tree showing four rows asked for %d", tree.Measure(10))
	}
}
