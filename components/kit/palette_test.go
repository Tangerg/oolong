package kit_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
)

func commands(t *testing.T) *headless.Commands[struct{}] {
	t.Helper()
	var c headless.Commands[struct{}]
	c.Add(headless.Command{Name: "new-session", Title: "start again"}, struct{}{})
	c.Add(headless.Command{Name: "clear", Title: "empty the screen"}, struct{}{})
	return &c
}

// TestThePaletteShowsWhyEachCommandIsThere. With subsequence matching the reason is
// often several letters apart, and a list of names that all look equally like the
// query tells a user nothing about why any of them is in it.
func TestThePaletteShowsWhyEachCommandIsThere(t *testing.T) {
	th := kit.Dark()
	found := commands(t).Find("ns")
	if len(found) == 0 || found[0].Command.Name != "new-session" {
		t.Fatalf("the query found %+v", found)
	}

	s := grid.NewSurface(40, 2)
	kit.Palette{Found: found, Theme: th}.Draw(s.View())

	// "n" of "new" and "s" of "session" are the two matched letters, and only those.
	// The first row is the one under the cursor, so a matched letter on it is the
	// accent over the selection rather than over ordinary text.
	picked := th.Selection.Merge(th.Accent)
	accented := 0
	for x := range 40 {
		if c := cellAt(s.View(), x, 0); c.Style == picked {
			accented++
		}
	}
	if accented != 2 {
		t.Errorf("%d characters were picked out, want 2", accented)
	}
	if row := rowOf(s.View(), 0, 40); !strings.HasPrefix(row, "new-session") {
		t.Errorf("the row is %q", row)
	}
}

func TestThePaletteMarksTheSelection(t *testing.T) {
	th := kit.Dark()
	s := grid.NewSurface(40, 2)
	kit.Palette{
		Found:    commands(t).Find(""),
		Selected: 1,
		Theme:    th,
		Glyphs:   kit.ASCII(),
	}.Draw(s.View())

	if got := rowOf(s.View(), 1, 40); !strings.HasPrefix(got, kit.ASCII().Marker+" ") {
		t.Errorf("the selected row is %q, want the marker", got)
	}
	if got := rowOf(s.View(), 0, 40); strings.HasPrefix(got, kit.ASCII().Marker) {
		t.Errorf("an unselected row is %q", got)
	}
	// And the names stay in line, because the marker's width is held clear on every
	// row rather than only on the one that has it.
	if strings.Index(rowOf(s.View(), 0, 40), "new") != strings.Index(rowOf(s.View(), 1, 40), "clear") {
		t.Error("the names are not in line")
	}
}

func TestThePaletteSaysWhenNothingMatched(t *testing.T) {
	// Nothing at all leaves the space blank, which reads as a bug rather than as an
	// answer.
	s := grid.NewSurface(40, 2)
	kit.Palette{Found: nil}.Draw(s.View())
	if got := rowOf(s.View(), 0, 40); !strings.Contains(got, "no matching command") {
		t.Errorf("the row is %q, want the palette to say so in words", got)
	}
	// And a caller with something better to say says it.
	s.Reset()
	kit.Palette{Found: nil, Empty: "nothing here"}.Draw(s.View())
	if got := rowOf(s.View(), 0, 40); !strings.Contains(got, "nothing here") {
		t.Errorf("the row is %q", got)
	}
}

func TestThePaletteIsAtLeastOneRowTall(t *testing.T) {
	// The message needs somewhere to go.
	if got := (kit.Palette{}).Measure(40); got != 1 {
		t.Errorf("an empty palette measures %d rows, want 1", got)
	}
	if got := (kit.Palette{Found: commands(t).Find("")}).Measure(40); got != 2 {
		t.Errorf("measured %d rows for two commands", got)
	}
}

func TestThePaletteDrawsIntoWhateverItIsGiven(t *testing.T) {
	found := commands(t).Find("")
	// Narrower than a name, shorter than the list, and nothing at all.
	kit.Palette{Found: found}.Draw(grid.NewSurface(4, 1).View())
	kit.Palette{Found: found}.Draw(grid.NewSurface(40, 1).View())
	kit.Palette{Found: found}.Draw(grid.View{})
}

// TestAWidgetWithNoLookDrawsNoLook. A theme nobody set is not a theme to fall back
// on: it is a caller who has not said, and guessing would put colours on a terminal
// whose own the interface has never asked about.
func TestAWidgetWithNoLookDrawsNoLook(t *testing.T) {
	s := grid.NewSurface(40, 2)
	kit.Palette{Found: commands(t).Find(""), Selected: 0}.Draw(s.View())
	if got := cellAt(s.View(), 0, 0).Style; got != (grid.Style{}) {
		t.Errorf("an undressed palette drew %+v", got)
	}
}
