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
	view := kit.NewForm(kit.Dark(), kit.Unicode(), form)
	view.Title = "New session"
	view.Keys = keys
	view.Hints = []keymap.Action{headless.Submit, headless.Cancel}

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
	view := kit.NewForm(kit.Dark(), kit.Unicode(), form)

	_ = paintWidget(12, view.Measure(12), view)
	rows := paintWidget(12, field.Measure(12), field)
	if len(rows) == 0 || !strings.HasPrefix(rows[0], "C") {
		t.Fatalf("field after appearance draw = %q, want the controller's C mark", rows)
	}
}

func TestAFormShowsWhatWasWrongInTheColourForIt(t *testing.T) {
	theme := kit.Dark()
	field := &headless.Text{Label: "Name", Check: func(string) error { return errors.New("required") }}
	form := headless.NewForm(field)
	view := kit.NewForm(theme, kit.Glyphs{}, form)
	form.Submit()

	s := grid.NewSurface(20, view.Measure(20))
	headless.NewRoot(view).Draw(s.View())
	// The row under the field is the problem, drawn in the one style a theme has for
	// saying something is wrong.
	if c := cellAt(s, 0, 2); c.Style.FG != theme.Danger.FG {
		t.Fatalf("the problem is drawn %+v, want the danger style", c)
	}
}
