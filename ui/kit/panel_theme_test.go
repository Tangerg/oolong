package kit_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
	"github.com/Tangerg/oolong/ui/kit"
)

// A panel used to keep a static copy of the palette in its Box. Changing the
// application theme then restyled its content while its frame stayed behind.
func TestPanelFollowsItsApplicationTheme(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)
	if kit.Dark().Border == kit.Light().Border {
		t.Fatal("the fixture needs palettes with distinct borders")
	}
	owner := kit.NewThemeOwner(kit.Dark())
	panel := kit.NewPanel(kit.PanelConfig{Scope: owner.Scope(), Box: kit.Box{Glyphs: kit.Unicode()}})
	root := scenes.Mount(panel)
	border := func() grid.Style {
		surface := grid.NewSurface(6, 3)
		root.Draw(surface.View())
		cell, _ := surface.CellAt(0, 1)
		return cell.Style
	}
	if got := border(); got != kit.Dark().Border {
		t.Fatalf("border style = %+v, want the dark palette's", got)
	}
	owner.Set(kit.Light())
	if got := border(); got != kit.Light().Border {
		t.Fatalf("border style after a theme change = %+v, want the light palette's", got)
	}
}

func TestPanelRefusesAPaletteInsideItsBox(t *testing.T) {
	for name, configure := range map[string]func(){
		"construction": func() { kit.NewPanel(kit.PanelConfig{Box: kit.Box{Theme: kit.Dark()}}) },
		"replacement": func() {
			panel := kit.NewPanel(kit.PanelConfig{Theme: kit.Dark()})
			panel.SetBox(kit.Box{Theme: kit.Light()})
		},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("a Box.Theme was accepted as a second palette owner")
				}
			}()
			configure()
		})
	}
}

func TestPanelMarksItsBorderWhileTheKeyboardIsInside(t *testing.T) {
	theme := kit.Dark()
	if theme.Border == theme.Accent {
		t.Fatal("the fixture needs a palette whose border and accent differ")
	}
	focusable := func() scene.Element {
		return scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }})
	}
	content, elsewhere := focusable(), focusable()
	panel := kit.NewPanel(kit.PanelConfig{Theme: theme, Box: kit.Box{Glyphs: kit.Unicode()}, Content: content})
	rows := scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Items: []scene.Item{
		{Element: panel.Element(), Size: layout.Fixed(3)},
		{Element: elsewhere, Size: layout.Fixed(1)},
	}})
	root, err := scene.NewRoot(scene.RootConfig{Element: rows.Element()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(root.Close)
	var sequence uint64
	border := func() grid.Style {
		surface := grid.NewSurface(6, 4)
		candidate, err := root.Prepare(surface.View())
		if err != nil {
			t.Fatal(err)
		}
		sequence++
		candidate.Commit(sequence)
		cell, _ := surface.CellAt(0, 1)
		return cell.Style
	}
	if err := elsewhere.RequestFocus(); err != nil {
		t.Fatal(err)
	}
	if got := border(); got != theme.Border {
		t.Fatalf("unfocused border = %+v, want the palette's border", got)
	}
	if err := content.RequestFocus(); err != nil {
		t.Fatal(err)
	}
	if got := border(); got != theme.Accent {
		t.Fatalf("focused border = %+v, want the palette's accent", got)
	}
}
