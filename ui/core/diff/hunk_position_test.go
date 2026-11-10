package diff_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/diff"
)

// A hunk with no lines on one side used to report position zero for it, losing
// where an insertion or deletion applies.
func TestOneSidedHunksKeepTheirPositions(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after []string
		old, new      int
	}{
		{name: "insertion", before: []string{"a", "b"}, after: []string{"a", "X", "b"}, old: 2, new: 2},
		{name: "deletion", before: []string{"a", "X", "b"}, after: []string{"a", "b"}, old: 2, new: 2},
		{name: "insertion at the start", before: []string{"a"}, after: []string{"X", "a"}, old: 1, new: 1},
		{name: "insertion at the end", before: []string{"a"}, after: []string{"a", "X"}, old: 2, new: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hunks := diff.Between(tc.before, tc.after).Hunks(0)
			if len(hunks) != 1 || hunks[0].Old != tc.old || hunks[0].New != tc.new {
				t.Fatalf("hunks = %+v, want one at old %d new %d", hunks, tc.old, tc.new)
			}
		})
	}
}
