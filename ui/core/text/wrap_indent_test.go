package text_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Indentation before a first word that does not fit with it used to become the
// first row's Gap, which nothing reads on a row that continues nothing: copying
// and searching lost it, and the screen did not show it.
func TestWrappingKeepsLeadingIndentation(t *testing.T) {
	for _, tc := range []struct {
		source string
		width  int
	}{
		{"  ab", 3},
		{"  中文中文", 3},
		{"        abc", 3},
		{"    ab", 3},
	} {
		rows := text.Of(tc.source, grid.Style{}).Wrap(tc.width)
		var rejoined strings.Builder
		for _, row := range rows {
			if !row.Joined && row.Gap != "" {
				t.Fatalf("%q at %d: a first row carries gap %q", tc.source, tc.width, row.Gap)
			}
			if row.Joined {
				rejoined.WriteString(row.Gap)
			}
			rejoined.WriteString(row.Line.String())
			if row.Line.Width() > tc.width {
				t.Fatalf("%q at %d: row %q exceeds the width", tc.source, tc.width, row.Line.String())
			}
		}
		if rejoined.String() != tc.source {
			t.Fatalf("%q at %d rejoined as %q", tc.source, tc.width, rejoined.String())
		}
	}
}
