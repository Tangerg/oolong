package kit_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
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
