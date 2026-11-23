package kit_test

import (
	"slices"
	"strconv"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

// unprojected draws text it does not offer for copying, like an image or a gauge.
type unprojected struct{ rows int }

func (u unprojected) HeightForWidth(int) int { return u.rows }
func (u unprojected) Draw(v grid.View) {
	for y := range u.rows {
		v.Text(0, y, "#", grid.Style{})
	}
}

func drawn(d grid.Drawable, width int) []string {
	surface := grid.NewSurface(width, d.HeightForWidth(width))
	d.Draw(surface.View())
	_, height := surface.View().Size()
	rows := make([]string, height)
	for y := range height {
		rows[y] = transcriptRow(surface.View(), y)
	}
	return rows
}

func projected(p text.Projector, width int) []string {
	var rows []string
	for _, row := range p.Rows(width) {
		rows = append(rows, row.Content.String()+"@"+strconv.Itoa(row.Offset))
	}
	return rows
}

// A composed block is copied the way it is drawn: each part's text sits on the rows
// and at the columns where the part was drawn, and a part with no text keeps its rows.
func TestAStackProjectsEveryPartWhereItDrawsIt(t *testing.T) {
	block := kit.Stack{
		kit.Label{Text: "speaker"},
		kit.Indent{Body: &said{rows: []string{"first", "second"}}, Left: 2},
		unprojected{rows: 1},
		nil,
		kit.Gap(1),
	}
	if got := block.HeightForWidth(20); got != 5 {
		t.Fatalf("height = %d, want 5", got)
	}
	if got, want := drawn(block, 20), []string{"speaker", "  first", "  second", "#", ""}; !slices.Equal(got, want) {
		t.Fatalf("drawn = %q, want %q", got, want)
	}
	if got, want := projected(block, 20), []string{"speaker@0", "first@2", "second@2", "@0", "@0"}; !slices.Equal(got, want) {
		t.Fatalf("projected = %q, want %q", got, want)
	}
}

// A part that claims more text rows than it draws cannot push the parts after it.
func TestAStackCutsAPartsProjectionToItsDrawnHeight(t *testing.T) {
	block := kit.Stack{overclaiming{}, kit.Label{Text: "next"}}
	if got, want := projected(block, 10), []string{"kept@0", "next@0"}; !slices.Equal(got, want) {
		t.Fatalf("projected = %q, want %q", got, want)
	}
}

type overclaiming struct{}

func (overclaiming) HeightForWidth(int) int { return 1 }
func (overclaiming) Draw(v grid.View)       { v.Text(0, 0, "kept", grid.Style{}) }
func (overclaiming) Rows(int) []text.Row {
	return []text.Row{{Content: text.Of("kept", grid.Style{})}, {Content: text.Of("extra", grid.Style{})}}
}

func TestAnIndentKeepsOneColumnForItsBody(t *testing.T) {
	indent := kit.Indent{Body: kit.Label{Text: "x"}, Left: 4}
	if got := drawn(indent, 3); !slices.Equal(got, []string{""}) {
		t.Fatalf("a body squeezed out of view drew %q", got)
	}
	if got := (kit.Indent{Body: kit.Label{Text: "x"}, Left: -2}).Rows(5); got[0].Offset != 0 {
		t.Fatalf("a negative indent shifted the text by %d", got[0].Offset)
	}
	if (kit.Indent{}).HeightForWidth(10) != 0 || kit.Gap(-1).HeightForWidth(10) != 0 {
		t.Fatal("absent content or a negative gap took rows")
	}
}

func TestALabelProjectsItsWholeTextWhereItIsAligned(t *testing.T) {
	label := kit.Label{Text: "status", Align: layout.End, Ellipsis: "…"}
	if got := projected(label, 10); !slices.Equal(got, []string{"status@4"}) {
		t.Fatalf("aligned projection = %q", got)
	}
	if got := label.Rows(3)[0]; got.Content.String() != "status" || got.Offset != 0 {
		t.Fatalf("a truncated label projected %q at %d, want the whole text from the start", got.Content.String(), got.Offset)
	}
}
