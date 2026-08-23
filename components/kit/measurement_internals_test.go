package kit

import (
	"image"
	"strconv"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

type maximumWidget struct{}

func (*maximumWidget) Draw(headless.Frame)     {}
func (*maximumWidget) Handle(input.Event) bool { return false }
func (*maximumWidget) Focus(bool)              {}
func (*maximumWidget) Measure(int) int         { return int(^uint(0) >> 1) }

func TestCompositeMeasurementsSaturateAtMaxInt(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	child := &maximumWidget{}
	cases := map[string]int{
		"panel":   NewPanel(PanelConfig{Box: Box{Theme: Theme{}, Glyphs: Glyphs{}}, Content: child}).Measure(80),
		"tabs":    NewTabs(TabsConfig{Items: []headless.Tab{{Title: "one", Of: child}}}).Measure(80),
		"message": (&Message{Trailing: maxInt}).Measure(80),
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
}
