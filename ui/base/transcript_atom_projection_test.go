package base_test

import (
	"errors"
	"image"
	"reflect"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestTranscriptPreservesTabAtomsAcrossSearchSelectionAndReflow(t *testing.T) {
	model := transcriptModel(t, base.TranscriptConfig{})
	transcriptAdd(t, model, transcriptBody("\t🏻X"))
	root := transcriptScene(t, model)
	surface := transcriptCommit(t, root, 6, 3, 1)
	projection := model.Layout()
	rows := projection.Rows(0, projection.Height())
	if len(rows) != 2 || rows[1].Content.Width() != 5 {
		t.Fatalf("wrapped rows = %+v", rows)
	}
	for _, expected := range []struct {
		column, width int
		content       string
	}{{2, 2, "🏻"}, {4, 1, "X"}} {
		cell, ok := surface.CellAt(expected.column, 1)
		if !ok || cell.Content() != expected.content || cell.Width() != expected.width {
			t.Fatalf("cell (%d, 1) = %q/%d, want %q/%d", expected.column, cell.Content(), cell.Width(), expected.content, expected.width)
		}
	}
	for _, expected := range []struct {
		query         string
		column, width int
	}{{"🏻", 2, 2}, {"X", 4, 1}} {
		result, err := base.Scan(t.Context(), base.ScanConfig{Query: expected.query, Rows: rows})
		want := []base.Match{{Row: 1, Spans: []base.Span{{Col: expected.column, Width: expected.width}}}}
		if err != nil || !reflect.DeepEqual(result.Matches, want) {
			t.Fatalf("search %q = %+v, %v; want %+v", expected.query, result.Matches, err, want)
		}
	}
	selected, ok := projection.Hit(base.Point{Row: 1, Col: 2})
	if !ok || selected.Start.Position.Byte != 8 || selected.End.Position.Byte != 12 {
		t.Fatalf("modifier hit = %+v, %v", selected, ok)
	}
	if continuation, ok := projection.Hit(base.Point{Row: 1, Col: 3}); !ok || continuation != selected {
		t.Fatalf("modifier continuation = %+v, %v; want %+v", continuation, ok, selected)
	}
	if !model.Selection().SelectWord(projection, base.Point{Row: 1, Col: 3}) {
		t.Fatal("modifier was reclassified as its neighboring space")
	}
	if got, err := model.Selection().Text(projection); err != nil || got != "🏻" {
		t.Fatalf("selected modifier = %q, %v", got, err)
	}
	if got, err := model.Selection().Rects(projection); err != nil || !reflect.DeepEqual(got, []image.Rectangle{image.Rect(2, 1, 4, 2)}) {
		t.Fatalf("modifier rectangle = %v, %v", got, err)
	}
	rows[1].Content[0].Text = "changed"
	if got := projection.Rows(1, 1)[0].Content; got.Width() != 5 || got.String() != "  🏻X" {
		t.Fatalf("caller changed committed row: %q/%d", got.String(), got.Width())
	}
	for sequence, expected := range []struct {
		width int
		start base.Point
	}{{5, base.Point{Row: 2}}, {12, base.Point{Col: 8}}} {
		transcriptCommit(t, root, expected.width, 3, uint64(sequence+2))
		reflowed := model.Layout()
		if got, ok := reflowed.Caret(selected.Start); !ok || got != expected.start {
			t.Fatalf("width %d caret = %+v, %v; want %+v", expected.width, got, ok, expected.start)
		}
		if got, err := model.Selection().Text(reflowed); err != nil || got != "🏻" {
			t.Fatalf("width %d copied modifier = %q, %v", expected.width, got, err)
		}
		want := []image.Rectangle{image.Rect(expected.start.Col, expected.start.Row, expected.start.Col+2, expected.start.Row+1)}
		if got, err := model.Selection().Rects(reflowed); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("width %d modifier rectangle = %v, %v; want %v", expected.width, got, err, want)
		}
	}
}

type boundaryProjection struct{}

func (boundaryProjection) HeightForWidth(int) int { return 1 }

func (boundaryProjection) content(width int) text.Line {
	return text.Line{{Text: " "}, {Text: "🏻", BreakBefore: width >= 4}}
}

func (p boundaryProjection) Draw(view grid.View) {
	width, _ := view.Size()
	p.content(width).Draw(view, 0, 0)
}

func (p boundaryProjection) Rows(width int) []text.Row {
	return []text.Row{{Content: p.content(width)}}
}

func TestTranscriptRejectsChangedAtomBoundariesWithEqualCopiedBytes(t *testing.T) {
	model := transcriptModel(t, base.TranscriptConfig{})
	transcriptAdd(t, model, boundaryProjection{})
	root := transcriptScene(t, model)
	transcriptCommit(t, root, 2, 1, 1)
	before := model.Layout()
	selected, ok := before.Hit(base.Point{})
	if !ok {
		t.Fatal("missing initial display atom")
	}
	model.Selection().Set(selected)
	transcriptCommit(t, root, 4, 1, 2)
	after := model.Layout()
	if before.Rows(0, 1)[0].Content.String() != after.Rows(0, 1)[0].Content.String() {
		t.Fatal("fixture changed copied text")
	}
	if _, ok := after.Caret(selected.Start); ok {
		t.Fatal("changed atom boundaries accepted an earlier projection's point")
	}
	if copied, err := model.Selection().Text(after); !errors.Is(err, base.ErrStaleSelection) {
		t.Fatalf("changed atom boundaries copied %q: %v", copied, err)
	}
}

type borrowedLineProjection struct{ content text.Line }

func (borrowedLineProjection) HeightForWidth(int) int { return 1 }

func (p borrowedLineProjection) Draw(view grid.View) { p.content.Draw(view, 0, 0) }

func (p borrowedLineProjection) Rows(int) []text.Row {
	return []text.Row{{Content: p.content}}
}

func TestTranscriptOwnsTheProjectorsSpanStorage(t *testing.T) {
	content := text.Line{{Text: " "}, {Text: "🏻X", BreakBefore: true}}
	model := transcriptModel(t, base.TranscriptConfig{})
	transcriptAdd(t, model, borrowedLineProjection{content: content})
	transcriptCommit(t, transcriptScene(t, model), 8, 1, 1)
	projection := model.Layout()
	if content[0].BreakBefore {
		t.Fatal("logical rejoining changed the projector's span")
	}
	selected, ok := projection.Hit(base.Point{Col: 1})
	if !ok {
		t.Fatal("missing independent modifier")
	}
	content[0].Text = "changed"
	content[1].BreakBefore = false
	if got := projection.Rows(0, 1)[0].Content; got.String() != " 🏻X" || got.Width() != 4 {
		t.Fatalf("projector changed the committed rows: %q/%d", got.String(), got.Width())
	}
	if got, ok := projection.Caret(selected.End); !ok || got != (base.Point{Col: 3}) {
		t.Fatalf("projector changed committed geometry: %+v, %v", got, ok)
	}
	model.Selection().Set(selected)
	if copied, err := model.Selection().Text(projection); err != nil || copied != "🏻" {
		t.Fatalf("projector changed committed logical text: %q, %v", copied, err)
	}
}
