package link

import "testing"

func TestFilteringLinksClearsDiscardedStorage(t *testing.T) {
	found := []Link{
		{Start: 0, End: 10, Target: "kept"},
		{Start: 2, End: 5, Target: "discarded"},
		{Start: 20, End: 25, Target: "also kept"},
	}
	ordered := inOrder(found)
	if len(ordered) != 2 {
		t.Fatalf("kept %d links, want two", len(ordered))
	}
	for i, discarded := range found[len(ordered):] {
		if discarded != (Link{}) {
			t.Fatalf("discarded slot %d retained %+v", i, discarded)
		}
	}
}
