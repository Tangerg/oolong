package kit

import (
	"image"
	"math"
	"strconv"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

type maximumWidget struct{ _ byte }

func (*maximumWidget) Draw(scene.Frame)        {}
func (*maximumWidget) Handle(input.Event) bool { return false }
func (*maximumWidget) Focus(bool)              {}
func (*maximumWidget) HeightForWidth(int) int  { return int(^uint(0) >> 1) }

func TestCompositeMeasurementsSaturateAtMaxInt(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	maxInt := int(^uint(0) >> 1)
	child := &maximumWidget{}
	cases := map[string]int{
		"panel": NewPanel(PanelConfig{Box: Box{Glyphs: Glyphs{}}, Content: scenes.Element(child)}).HeightForWidth(80),
		"tabs":  NewTabs(TabsConfig{Items: []base.Tab{{Title: "one", Content: scenes.Element(&maximumWidget{})}}}).HeightForWidth(80),
		"entry": (&Entry{Trailing: maxInt}).HeightForWidth(80),
	}
	for name, got := range cases {
		if got != maxInt {
			t.Errorf("%s measured %d, want saturation at %d", name, got, maxInt)
		}
	}
}

func TestExtremeBoxInsetsHaveNoInterior(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	box := Box{Padding: layout.Uniform(maxInt)}
	if got := box.Overhead(); got != image.Pt(maxInt, maxInt) {
		t.Fatalf("overhead = %v, want saturated extents", got)
	}
	if got := box.InnerRect(image.Pt(80, 24)); !got.Empty() {
		t.Fatalf("extreme inset produced interior %v", got)
	}
}

func TestLineNumberWidthDoesNotWrapAtIntegerLimits(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	want := len(strconv.Itoa(maxInt)) + 1 // the default gap
	if got := (LineNumbers{First: maxInt}).Width(maxInt); got != want {
		t.Fatalf("line-number width = %d, want %d", got, want)
	}
	for _, lines := range []int{math.MinInt, -1, 0, 1} {
		if got := (LineNumbers{}).Width(lines); got != 2 {
			t.Errorf("line-number width for %d lines = %d, want one digit and one gap", lines, got)
		}
	}
}

func TestMeterValueColumnIsAtomic(t *testing.T) {
	narrow := layoutMeter(3, 20, 5)
	if !narrow.value.Empty() {
		t.Fatalf("narrow value rectangle = %v, want no partial value", narrow.value)
	}
	if narrow.track.Empty() {
		t.Fatal("narrow meter did not return omitted-value space to its track")
	}

	exact := layoutMeter(5, 20, 5)
	if got := exact.value.Dx(); got != 5 {
		t.Fatalf("exact value width = %d, want 5", got)
	}
}

func TestCellHeightIsIndependentOfItsPreferredColumnWidth(t *testing.T) {
	cell := LabelCell(Label{Text: "header"})
	for _, width := range []int{1, 6, 20} {
		if got := cell.HeightForWidth(width); got != 1 {
			t.Errorf("cell height at width %d = %d, want one row", width, got)
		}
	}
	columns := Table{Rows: 1, Columns: []Column{{Size: layout.Measured(0, 0)}}, Cell: func(int, int) Cell { return cell }}
	if got := columns.Layout(20).Widths()[0]; got != cell.Preferred {
		t.Errorf("fitted column width = %d, want %d", got, cell.Preferred)
	}
}

func TestHugeClippedMeterUsesOnlyVisibleCells(t *testing.T) {
	surface := grid.NewSurface(2, 1)
	writes := 0
	v := surface.View().RestrictWrites(func() error {
		writes++
		if writes > 4 {
			t.Fatal("meter visited cells outside the visible region")
		}
		return nil
	}).Sub(grid.Area(0, 0, math.MaxInt, 1))
	bar{fraction: 1, glyphs: ASCII()}.Draw(v)
	for x := range 2 {
		cell, _ := surface.View().CellAt(x, 0)
		if cell.Content() != "#" {
			t.Fatal("full meter lost its visible extent")
		}
	}
}
