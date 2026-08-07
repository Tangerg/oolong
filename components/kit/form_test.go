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
	form := &headless.Form{Fields: []headless.Field{
		&headless.Text{Label: "Name", Value: headless.Bind(&name), Placeholder: "who?"},
		&headless.Select[string]{
			Label:   "Model",
			Options: headless.Options("fast", "good"),
			Value:   headless.Bind(&model),
		},
	}}
	keys := headless.DefaultFormKeys()
	view := kit.Form{
		Of:     form,
		Theme:  kit.Dark(),
		Glyphs: kit.Unicode(),
		Title:  "New session",
		Keys:   keys,
		Hints:  []keymap.Action{headless.Submit, headless.Cancel},
	}

	rows := paintWidget(24, view.Measure(24), &view)
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

func TestAFormShowsWhatWasWrongInTheColourForIt(t *testing.T) {
	theme := kit.Dark()
	field := &headless.Text{Label: "Name", Check: func(string) error { return errors.New("required") }}
	form := &headless.Form{Fields: []headless.Field{field}}
	view := kit.Form{Of: form, Theme: theme}
	form.Submit()

	s := grid.NewSurface(20, view.Measure(20))
	headless.NewRoot(&view).Draw(s.View())
	// The row under the field is the problem, drawn in the one style a theme has for
	// saying something is wrong.
	if c := cellAt(s, 0, 2); c.Style.FG != theme.Danger.FG {
		t.Fatalf("the problem is drawn %+v, want the danger style", c)
	}
}
