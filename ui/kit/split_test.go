package kit_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestSplitPinsTheEndAndDropsItRatherThanCuttingIt(t *testing.T) {
	split := kit.Split{Start: text.Of("workspace title", grid.Style{}), End: text.Of("12k", grid.Style{}), Ellipsis: "…"}
	for _, test := range []struct {
		width int
		want  string
	}{
		{width: 20, want: "workspace title  12k"},
		{width: 10, want: "works… 12k"},
		{width: 4, want: "wor…"},
	} {
		got := grid.Render(test.width, 1, split.Draw)[0]
		if strings.TrimRight(got, " ") != strings.TrimRight(test.want, " ") {
			t.Errorf("width %d drew %q, want %q", test.width, got, test.want)
		}
	}
}

func TestSplitProjectsTheWholeStartAndThePinnedEnd(t *testing.T) {
	split := kit.Split{Start: text.Of("left", grid.Style{}), End: text.Of("right", grid.Style{}), Gap: 2}
	rows := split.Rows(20)
	if len(rows) != 1 || rows[0].Content.String() != "left           right" {
		t.Fatalf("projected %q", rows[0].Content.String())
	}
	if rows := split.Rows(6); rows[0].Content.String() != "left" {
		t.Fatalf("a dropped end was projected: %q", rows[0].Content.String())
	}
}
