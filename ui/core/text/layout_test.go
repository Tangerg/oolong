package text_test

import (
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestLayoutUsesOneMappingForWrappingCaretHitAndSelection(t *testing.T) {
	l := text.NewLayout(text.LayoutConfig{Lines: []string{"a界bcd", "x\ty"}, Width: 4, Revision: 7})
	rows := l.Rows()
	if len(rows) != 5 || rows[0].Text != "a界b" || rows[1].Text != "cd" {
		t.Fatalf("rows: %+v", rows)
	}
	boundary := len("a界b")
	if point := l.Caret(text.Position{Byte: boundary}); point != image.Pt(0, 1) {
		t.Fatalf("forward affinity: %v", point)
	}
	if point := l.Caret(text.Position{Byte: boundary, Affinity: text.Backward}); point != image.Pt(4, 0) {
		t.Fatalf("backward affinity: %v", point)
	}
	if hit := l.Hit(image.Pt(2, 0)); hit.Byte != 1 {
		t.Fatalf("wide cluster was split: %+v", hit)
	}
	if hit := l.Hit(image.Pt(4, 0)); hit.Byte != boundary || hit.Affinity != text.Backward {
		t.Fatalf("row-end affinity: %+v", hit)
	}
	selected := text.Range{Start: text.Position{Byte: 1}, End: text.Position{Byte: boundary}}
	if rects := l.Rects(selected); !slices.Equal(rects, []image.Rectangle{image.Rect(1, 0, 4, 1)}) {
		t.Fatalf("selection cells: %v", rects)
	}
	if value := l.Text(selected); value != "界b" {
		t.Fatalf("selected logical text: %q", value)
	}
	if l.Revision() != 7 {
		t.Fatal("layout lost its source revision")
	}
}

func TestLayoutNormalizesClustersAndPreservesHardNewlines(t *testing.T) {
	l := text.NewLayout(text.LayoutConfig{Lines: []string{"e\u0301👨‍👩‍👧‍👦", "tail"}, Width: 2})
	if position := l.Normalize(text.Position{Byte: 1, Affinity: text.Backward}); position.Byte != 0 {
		t.Fatalf("backward normalization: %+v", position)
	}
	if position := l.Normalize(text.Position{Byte: 1}); position.Byte != len("e\u0301") {
		t.Fatalf("forward normalization: %+v", position)
	}
	end := text.Position{Line: 2}
	if copied := l.Text(text.Range{End: end}); copied != "e\u0301👨‍👩‍👧‍👦\ntail" {
		t.Fatalf("soft/hard line breaks: %q", copied)
	}
	other := text.NewLayout(text.LayoutConfig{Lines: []string{"e\u0301👨‍👩‍👧‍👦", "tail"}, Width: 20})
	anchor := l.Hit(image.Pt(0, 1))
	if got := other.Hit(other.Caret(anchor)); got.Line != anchor.Line || got.Byte != anchor.Byte {
		t.Fatalf("resize changed logical anchor: %+v -> %+v", anchor, got)
	}
}

func TestLayoutOwnsSourceAndNeverCopiesHiddenControlBytes(t *testing.T) {
	lines := []string{"a\x1bb\x00c\t"}
	l := text.NewLayout(text.LayoutConfig{Lines: lines, Width: 0})
	lines[0] = "changed"
	rows := l.Rows()
	rows[0].Text = "another change"
	if value := l.Text(text.Range{End: text.Position{Line: 1}}); value != "abc\t" {
		t.Fatalf("unsafe or aliased logical text: %q", value)
	}
	row, ok := l.Row(0)
	if !ok || row.Width != text.Width(row.Text) {
		t.Fatal("row metrics differ from terminal metrics")
	}
	surface := grid.NewSurface(12, 1)
	l.Draw(surface.View(), grid.Style{})
	if cell, ok := surface.CellAt(2, 0); !ok || cell.Content() != "c" {
		t.Fatalf("draw: %+v", cell)
	}
}

func TestLayoutPreservesWhitespaceAndClipsSelectionWithoutSplittingAtoms(t *testing.T) {
	l := text.NewLayout(text.LayoutConfig{Lines: []string{"word   next"}, Width: 5})
	var joined strings.Builder
	for _, row := range l.Rows() {
		joined.WriteString(row.Text)
	}
	if joined.String() != "word   next" {
		t.Fatalf("editable wrapping consumed characters: %q", joined.String())
	}
	unwrapped := text.NewLayout(text.LayoutConfig{Lines: []string{"abcdef"}, Width: 2, Wrap: text.WrapNone})
	if unwrapped.Len() != 1 {
		t.Fatal("no-wrap mode wrapped")
	}
	if rects := unwrapped.Rects(text.Range{End: text.Position{Byte: 6}}); !slices.Equal(rects, []image.Rectangle{image.Rect(0, 0, 2, 1)}) {
		t.Fatalf("selection escaped allocation: %v", rects)
	}
}

func FuzzLayoutPositionsRemainGraphemeBoundaries(f *testing.F) {
	for _, value := range []string{"hello world", "你\t好 e\u0301", "👨‍👩‍👧‍👦x", "a  b", "\xffa\x1b"} {
		f.Add(value, uint8(4))
	}
	f.Fuzz(func(t *testing.T, source string, width uint8) {
		if len(source) > 1024 {
			return
		}
		logical := text.Printable(source)
		l := text.NewLayout(text.LayoutConfig{Lines: []string{source}, Width: int(width%32) + 1})
		boundaries := map[int]bool{0: true, len(logical): true}
		for at, cluster := range text.Clusters(logical) {
			boundaries[at], boundaries[at+len(cluster)] = true, true
		}
		var joined strings.Builder
		for y, row := range l.Rows() {
			if !boundaries[row.From] || !boundaries[row.To] {
				t.Fatalf("wrap split a grapheme: %+v", row)
			}
			joined.WriteString(row.Text)
			for x := 0; x <= min(row.Width+1, 128); x++ {
				position := l.Hit(image.Pt(x, y))
				if !boundaries[position.Byte] {
					t.Fatalf("hit split a grapheme: %+v", position)
				}
				if normalized := l.Normalize(position); normalized != position {
					t.Fatalf("hit was not normalized: %+v -> %+v", position, normalized)
				}
			}
		}
		if joined.String() != logical {
			t.Fatalf("wrap changed logical text: %q -> %q", logical, joined.String())
		}
	})
}
