package kit_test

import (
	"errors"
	"image"
	"math"
	"slices"
	"strconv"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

func expectLayoutConfigPanic(t *testing.T, want error, run func()) {
	t.Helper()
	defer func() {
		err, _ := recover().(error)
		if !errors.Is(err, want) {
			t.Errorf("panic = %v, want %v", err, want)
		}
	}()
	run()
}

func TestInvalidLayoutConfigurationCannotPartiallyPaint(t *testing.T) {
	invalidAlign := layout.Align(math.MaxUint8)
	theme := kit.Dark()
	theme.Surface = grid.Style{BG: grid.RGBColor(10, 20, 30)}
	theme.Scrim = kit.Scrim{Color: grid.RGBColor(20, 30, 40), Opacity: 0.5}
	for _, tc := range []struct {
		name string
		want error
		draw func(grid.View)
	}{
		{"label", layout.ErrInvalidAlign, (kit.Label{Text: "label", Align: invalidAlign}).Draw},
		{"image", layout.ErrInvalidAlign, (kit.Image{Alt: "image", Align: invalidAlign}).Draw},
		{"box title", layout.ErrInvalidAlign, func(view grid.View) {
			kit.Box{Theme: theme, Glyphs: kit.ASCII(), TitleAlign: invalidAlign}.Draw(view)
		}},
		{"box footer", layout.ErrInvalidAlign, func(view grid.View) {
			kit.Box{Theme: theme, Glyphs: kit.ASCII(), Title: "valid title", FooterAlign: invalidAlign}.Draw(view)
		}},
		{"table", layout.ErrInvalidAlign, (kit.Table{Columns: []kit.Column{{Title: "valid"}, {Align: invalidAlign}}}).Draw},
		{"overlay", layout.ErrInvalidAnchor, func(view grid.View) {
			kit.Overlay{Anchor: layout.Anchor(math.MaxUint8), Theme: theme}.Draw(view)
		}},
	} {
		for _, empty := range []bool{false, true} {
			t.Run(tc.name+"/empty="+strconv.FormatBool(empty), func(t *testing.T) {
				surface := grid.NewSurface(12, 3)
				view := surface.View()
				view.Fill(view.Bounds(), grid.Style{BG: grid.RGBColor(90, 80, 70)})
				view.Text(0, 0, "retained", grid.Style{FG: grid.RGBColor(60, 50, 40)})
				before := [][]grid.Cell{surface.Row(0), surface.Row(1), surface.Row(2)}
				if empty {
					view = view.Sub(image.Rectangle{})
				}
				expectLayoutConfigPanic(t, tc.want, func() { tc.draw(view) })
				for y, row := range before {
					if !slices.Equal(surface.Row(y), row) {
						t.Fatalf("rejected configuration changed row %d", y)
					}
				}
			})
		}
	}
}

func TestTableValidatesEveryColumnBeforeMeasurementCallbacks(t *testing.T) {
	for _, width := range []int{0, 12} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			calls := 0
			table := kit.Table{
				Columns: []kit.Column{
					{Title: "measured", Size: layout.Measured(0, 0)},
					{Title: "invalid", Align: layout.Align(math.MaxUint8)},
				},
				Rows: 1,
				Cell: func(int, int) kit.Cell {
					calls++
					return kit.Cell{Preferred: 2}
				},
				Sorted: func() (int, bool, bool) {
					calls++
					return 0, false, true
				},
			}
			expectLayoutConfigPanic(t, layout.ErrInvalidAlign, func() { table.Layout(width) })
			if calls != 0 {
				t.Fatalf("invalid column reached projection callbacks %d times", calls)
			}
		})
	}
}

func TestLabelCellCannotCaptureInvalidAlignment(t *testing.T) {
	expectLayoutConfigPanic(t, layout.ErrInvalidAlign, func() {
		kit.LabelCell(kit.Label{Align: layout.Align(math.MaxUint8)})
	})
}

func TestInvalidContainerLayoutCannotClaimChildren(t *testing.T) {
	for _, tc := range []struct {
		name string
		want error
		make func(scene.Element)
	}{
		{"panel", layout.ErrInvalidAlign, func(child scene.Element) {
			kit.NewPanel(kit.PanelConfig{Content: child, Box: kit.Box{FooterAlign: layout.Align(math.MaxUint8)}})
		}},
		{"dialog", layout.ErrInvalidAnchor, func(child scene.Element) {
			kit.NewDialog(kit.DialogConfig{Body: child, Where: layout.Placement{Anchor: layout.Anchor(math.MaxUint8)}})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			child := scene.NewElement(scene.ElementConfig{})
			expectLayoutConfigPanic(t, tc.want, func() { tc.make(child) })
			group := scene.NewGroup(scene.GroupConfig{})
			if err := group.Set(scene.Item{Element: child}); err != nil {
				t.Fatalf("rejected container retained its child: %v", err)
			}
		})
	}
}

func TestPanelRejectsChangedAlignmentBeforeMeasuringItsChild(t *testing.T) {
	calls := 0
	child := scene.NewElement(scene.ElementConfig{Measure: func(layout.Axis, int) int {
		calls++
		return 1
	}})
	panel := kit.NewPanel(kit.PanelConfig{Content: child})
	box := panel.Box()
	box.FooterAlign = layout.Align(math.MaxUint8)
	expectLayoutConfigPanic(t, layout.ErrInvalidAlign, func() { panel.SetBox(box) })
	if panel.Box().FooterAlign != layout.Start {
		t.Fatal("invalid box was installed")
	}
	if calls != 0 {
		t.Fatalf("invalid panel reached its child measurer %d times", calls)
	}
}

func TestDialogRejectsChangedAnchorBeforePaintingItsBackdrop(t *testing.T) {
	dialog := kit.NewDialog(kit.DialogConfig{
		Initial: true,
		Theme:   kit.Theme{Scrim: kit.Scrim{Color: grid.RGBColor(20, 30, 40), Opacity: 0.5}},
	})
	root, err := scene.NewRoot(scene.RootConfig{Element: dialog.Element()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(root.Close)
	surface := grid.NewSurface(12, 3)
	view := surface.View()
	view.Fill(view.Bounds(), grid.Style{BG: grid.RGBColor(90, 80, 70)})
	before := [][]grid.Cell{surface.Row(0), surface.Row(1), surface.Row(2)}
	where := dialog.Where()
	where.Anchor = layout.Anchor(math.MaxUint8)
	expectLayoutConfigPanic(t, layout.ErrInvalidAnchor, func() { dialog.SetWhere(where) })
	if dialog.Where().Anchor == where.Anchor {
		t.Fatal("invalid anchor was installed")
	}
	for y, row := range before {
		if !slices.Equal(surface.Row(y), row) {
			t.Fatalf("rejected dialog changed backdrop row %d", y)
		}
	}
	where.Anchor = layout.Middle
	dialog.SetWhere(where)
	frame, err := root.Prepare(view)
	if err != nil {
		t.Fatalf("rejected anchor blocked the next frame: %v", err)
	}
	frame.Abort()
}
