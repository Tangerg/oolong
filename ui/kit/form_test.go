package kit_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestFormComposesOneControllerWithThemeAndHelp(t *testing.T) {
	field := base.NewText(base.TextConfig{Label: "Name", Placeholder: "who?"})
	choice := base.NewSelect(base.SelectConfig[string]{Same: base.Equal[string], Label: "Model", Options: base.Options("fast", "good")})
	form := kit.NewForm(kit.FormConfig{Theme: kit.Dark(), Glyphs: kit.Unicode(), Fields: []base.Field{field, choice}, Title: "New session", Hints: []keymap.Action{base.Submit, base.Cancel}})
	owner := form.Controller()
	if owner == nil || form.Controller() != owner {
		t.Fatal("appearance manufactured another controller")
	}
	drawn := strings.Join(paintWidget(24, form.HeightForWidth(24), form), "\n")
	for _, want := range []string{"New session", "Name", "who?", "Model", "fast", "good", "enter submit", kit.Unicode().Taken} {
		if !strings.Contains(drawn, want) {
			t.Fatalf("missing %q in %q", want, drawn)
		}
	}
	if owner.Keys != nil {
		t.Fatal("drawing hints materialized default behavior configuration")
	}
}

func TestFormAppearanceDoesNotMutateFieldsConfiguration(t *testing.T) {
	field := base.NewText(base.TextConfig{Initial: "answer", Placeholder: "original"})
	form := kit.NewForm(kit.FormConfig{Theme: kit.Dark(), Glyphs: kit.Unicode(), Fields: []base.Field{field}})
	revision := field.Revision()
	_ = paintWidget(20, form.HeightForWidth(20), form)
	if field.Text() != "answer" || field.Placeholder != "original" || field.Revision() != revision || field.Keys != nil {
		t.Fatal("appearance changed owner configuration")
	}
}

func TestFormShowsValidationUsingThemeDangerStyle(t *testing.T) {
	theme := kit.Dark()
	field := base.NewText(base.TextConfig{Label: "Name", Check: func(string) error { return errors.New("required") }})
	form := kit.NewForm(kit.FormConfig{Theme: theme, Fields: []base.Field{field}})
	_, _ = form.Controller().Submit()
	surface := grid.NewSurface(20, form.HeightForWidth(20))
	newTestRoot(form).Draw(surface.View())
	if cell := cellAt(surface, 0, 2); cell.Style.FG != theme.Danger.FG {
		t.Fatalf("problem style=%+v want danger", cell.Style)
	}
}

func TestKitConstructorsCreateTheirOwnReachableController(t *testing.T) {
	form := kit.NewForm(kit.FormConfig{})
	tree := kit.NewTree(kit.TreeConfig[string]{})
	if form.Controller() == nil || tree.Controller() == nil {
		t.Fatal("zero config has no owned controller")
	}
}
