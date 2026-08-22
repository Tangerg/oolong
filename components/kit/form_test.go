package kit_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/keymap"
)

func TestAFormIsDressedByTheThemeAndDrawsItself(t *testing.T) {
	// The fields draw themselves, because a field is generic over what it holds and
	// nothing here could name every kind of one. So this is where the look goes in.
	var name string
	var model string
	modelField := &headless.Select[string]{Label: "Model", Value: headless.Bind(&model)}
	modelField.SetOptions(headless.Options("fast", "good"))
	form := headless.NewForm(
		&headless.Text{Label: "Name", Value: headless.Bind(&name), Placeholder: "who?"},
		modelField,
	)
	keys := headless.DefaultFormKeys()
	form.Keys = keys
	view := kit.NewForm(kit.FormConfig{
		Theme: kit.Dark(), Glyphs: kit.Unicode(), Controller: form, Title: "New session",
		Hints: []keymap.Action{headless.Submit, headless.Cancel},
	})

	rows := paintWidget(24, view.Measure(24), view)
	drawn := strings.Join(rows, "\n")
	for _, want := range []string{"New session", "Name", "who?", "Model", "fast", "good", "enter submit"} {
		if !strings.Contains(drawn, want) {
			t.Fatalf("drawn:\n%s\nwant %q in it", drawn, want)
		}
	}
	// The mark beside the chosen row comes from the glyph set, because which characters
	// a terminal can draw is a fact about the terminal.
	if !strings.Contains(drawn, kit.Unicode().Taken) {
		t.Fatalf("drawn:\n%s\nwant the mark beside the choice", drawn)
	}
}

func TestAFormAppearanceDoesNotReplaceTheControllersLook(t *testing.T) {
	field := new(headless.Select[string])
	field.SetOptions(headless.Options("one", "two"))
	form := headless.NewForm(field)
	form.Look = headless.Look{Taken: "C", Free: "-"}
	view := kit.NewForm(kit.FormConfig{Theme: kit.Dark(), Glyphs: kit.Unicode(), Controller: form})

	_ = paintWidget(12, view.Measure(12), view)
	rows := paintWidget(12, form.Measure(12), form)
	if len(rows) == 0 || !strings.HasPrefix(rows[0], "C") {
		t.Fatalf("form after appearance draw = %q, want the controller's C mark", rows)
	}
}

func TestAFormAppearanceDoesNotTakeOwnershipOfControllerKeys(t *testing.T) {
	form := headless.NewForm(&headless.Text{Label: "Name"})
	view := kit.NewForm(kit.FormConfig{
		Controller: form,
		Hints:      []keymap.Action{headless.Submit},
	})

	rows := paintWidget(20, view.Measure(20), view)
	if form.Keys != nil {
		t.Fatal("dressing a form materialized behavior configuration on its controller")
	}
	if drawn := strings.Join(rows, "\n"); !strings.Contains(drawn, "enter submit") {
		t.Fatalf("default form hint was not projected: %q", drawn)
	}
}

func TestAFormShowsWhatWasWrongInTheColourForIt(t *testing.T) {
	theme := kit.Dark()
	field := &headless.Text{Label: "Name", Check: func(string) error { return errors.New("required") }}
	form := headless.NewForm(field)
	view := kit.NewForm(kit.FormConfig{Theme: theme, Controller: form})
	form.Submit()

	s := grid.NewSurface(20, view.Measure(20))
	headless.NewRoot(view).Draw(s.View())
	// The row under the field is the problem, drawn in the one style a theme has for
	// saying something is wrong.
	if c := cellAt(s, 0, 2); c.Style.FG != theme.Danger.FG {
		t.Fatalf("the problem is drawn %+v, want the danger style", c)
	}
}

func TestDressedControllersAreRequiredAtConstruction(t *testing.T) {
	for name, build := range map[string]func(){
		"form": func() { kit.NewForm(kit.FormConfig{}) },
		"tree": func() { kit.NewTree(kit.TreeConfig[string]{}) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("missing controller did not panic")
				}
			}()
			build()
		})
	}
}
