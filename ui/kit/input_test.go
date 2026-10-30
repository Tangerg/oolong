package kit_test

import (
	"context"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestInputStandaloneUsesOneOwnerForAXKeysAndLiveTheme(t *testing.T) {
	owner := kit.NewThemeOwner(kit.Dark())
	field := kit.NewInput(kit.InputConfig{Key: "project.name", Label: "Name", Initial: "初始", Scope: owner.Scope()})
	if field.Element() != field.Controller().Element() {
		t.Fatal("input skin allocated a second behaviour identity")
	}
	root := newTestRoot(field)
	surface := grid.NewSurface(12, 2)
	root.Draw(surface.View())
	observation, err := root.root.Observe(scene.Query{})
	if err != nil || len(observation.Nodes) != 1 || observation.Nodes[0].Key != "project.name" || observation.Nodes[0].Name != "Name" {
		t.Fatalf("input semantics=%v err=%v", observation, err)
	}
	result, err := root.root.Invoke(context.Background(), field.Element().Target(), scene.ActionFill, "你好")
	if err != nil || result.State != scene.Applied || field.Controller().Text() != "你好" {
		t.Fatalf("AX result=%v err=%v", result, err)
	}
	root.Handle(input.Key{Code: input.End})
	root.Handle(input.Key{Rune: '!'})
	if field.Controller().Text() != "你好!" {
		t.Fatal("keyboard did not edit the AX-filled document")
	}
	wake := root.root.Changes()
	owner.Set(kit.Light())
	select {
	case <-wake:
	default:
		t.Fatal("theme did not invalidate standalone input")
	}
	root.Draw(surface.View())
	if cellAt(surface, 0, 1).Style.FG != kit.Light().Text.FG {
		t.Fatal("input retained stale palette")
	}
}

func TestInputControllerAsFormFieldUsesTheFormsExplicitPresentation(t *testing.T) {
	red := grid.Style{FG: grid.RGBColor(255, 0, 0)}
	input := kit.NewInput(kit.InputConfig{Initial: "shared", Appearance: kit.ThemeOverride{Text: &red}})
	form := kit.NewForm(kit.FormConfig{Theme: kit.Light(), Fields: []base.Field{input.Controller()}})
	surface := grid.NewSurface(12, 1)
	newTestRoot(form).Draw(surface.View())
	if form.Controller().Fields()[0] != input.Controller() || input.Controller().Text() != "shared" {
		t.Fatal("form introduced another text owner")
	}
	if cellAt(surface, 0, 0).Style.FG != kit.Light().Text.FG {
		t.Fatal("standalone override competed with Form's field presentation slot")
	}
}
