package kit

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/diff"
	"github.com/Tangerg/oolong/ui/core/grid"
)

func TestInvalidatingTextLayoutsReleasesStaleRowsImmediately(t *testing.T) {
	t.Run("diff", func(t *testing.T) {
		view := NewDiff(DiffConfig{Theme: Dark(), Glyphs: Unicode(), Hunks: []diff.Hunk{{Lines: diff.Script{{
			Kind: diff.Added,
			Text: strings.Repeat("old content ", 128),
		}}}}})

		view.layout(20)
		view.SetHunks(nil)
		if view.wrapped.valid || len(view.wrapped.value) != 0 {
			t.Fatalf("invalidated layout = %+v", view.wrapped)
		}
		for i, cached := range view.wrapped.value[:cap(view.wrapped.value)] {
			if !reflect.DeepEqual(cached, diffRow{}) {
				t.Fatalf("cached diff row %d retained stale content %+v", i, cached)
			}
		}
	})

	t.Run("paragraph", func(t *testing.T) {
		paragraph := NewParagraph(strings.Repeat("old content ", 128), grid.Style{})
		paragraph.rows(20)
		paragraph.SetText(nil)
		if paragraph.wrap.valid || len(paragraph.wrap.value) != 0 {
			t.Fatalf("invalidated paragraph retained %d row(s)", len(paragraph.wrap.value))
		}
		for i, cached := range paragraph.wrap.value[:cap(paragraph.wrap.value)] {
			if !reflect.DeepEqual(cached, row{}) {
				t.Fatalf("cached paragraph row %d retained stale content %+v", i, cached)
			}
		}
	})
}

func TestDiffReusesTheLayoutMeasuredForItsFrame(t *testing.T) {
	view := NewDiff(DiffConfig{Theme: Dark(), Glyphs: Unicode(), Hunks: []diff.Hunk{{Lines: diff.Script{{
		Kind: diff.Added,
		Text: "one two three four",
	}}}}})

	measured := view.layout(8)
	drawn := view.layout(8)
	if len(measured) == 0 || len(drawn) == 0 || &measured[0] != &drawn[0] {
		t.Fatal("a second layout at one width did not reuse the measured rows")
	}
}

func TestParagraphCopyDetachesItsWrap(t *testing.T) {
	original := NewParagraph("original words", grid.Style{})
	want := original.Rows(8)

	copied := *original
	copied.SetText(linesOf("replacement", grid.Style{}))
	_ = copied.Rows(8)

	if got := original.Rows(8); !reflect.DeepEqual(got, want) {
		t.Fatalf("original rows after changing copy = %+v, want %+v", got, want)
	}
}

func TestDiffCopyDetachesItsLayout(t *testing.T) {
	original := NewDiff(DiffConfig{Theme: Theme{}, Glyphs: Glyphs{}, Hunks: []diff.Hunk{{Lines: diff.Script{
		{Kind: diff.Added, Text: "original words"},
	}}}})

	want := append([]diffRow(nil), original.layout(8)...)

	copied := *original
	copied.SetHunks([]diff.Hunk{{Lines: diff.Script{
		{Kind: diff.Removed, Text: "replacement"},
	}}})
	_ = copied.layout(8)

	if got := original.layout(8); !reflect.DeepEqual(got, want) {
		t.Fatalf("original layout after changing copy = %+v, want %+v", got, want)
	}
}
