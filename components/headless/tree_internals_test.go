package headless

import "testing"

func TestTreeDepthUsesStorageRatherThanTheCallStackOrPathCopies(t *testing.T) {
	const depth = 32_768
	nodes := []Node[int]{{Item: depth - 1}}
	for value := depth - 2; value >= 0; value-- {
		nodes = []Node[int]{{Item: value, Children: nodes}}
	}

	tree := NewTree(nodes...)
	tree.open = make(map[uint64]bool, depth-1)
	owned := tree.nodes
	for len(owned) > 0 {
		node := &owned[0]
		if len(node.children) == 0 {
			break
		}
		tree.open[node.id] = true
		owned = node.children
	}
	tree.rebuild()

	rows := tree.Rows()
	if len(rows) != depth {
		t.Fatalf("visible rows = %d, want %d", len(rows), depth)
	}
	if got := rows[len(rows)-1].Depth; got != depth-1 {
		t.Fatalf("last depth = %d, want %d", got, depth-1)
	}

	snapshot := tree.Nodes()
	for level := range depth {
		if len(snapshot) != 1 {
			t.Fatalf("level %d has %d nodes, want 1", level, len(snapshot))
		}
		if snapshot[0].Item != level {
			t.Fatalf("level %d contains %d", level, snapshot[0].Item)
		}
		snapshot = snapshot[0].Children
	}
	if len(snapshot) != 0 {
		t.Fatalf("deep snapshot has %d trailing nodes", len(snapshot))
	}
}

func TestTreeRejectsCyclicNodeStorageAtItsOwnershipBoundary(t *testing.T) {
	nodes := make([]Node[int], 1)
	nodes[0] = Node[int]{Item: 1, Children: nodes}
	tree := NewTree(Node[int]{Item: 7})
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("cyclic hierarchy did not panic")
			}
		}()
		tree.SetNodes(nodes)
	}()
	if got, ok := tree.Current(); !ok || got != 7 {
		t.Fatalf("invalid replacement changed the old tree to %d, %v", got, ok)
	}
}
