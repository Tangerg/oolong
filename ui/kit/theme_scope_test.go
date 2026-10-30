package kit_test

import (
	"context"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestThemeCascadeSharesOneLiveOwnerAndOwnsOverrides(t *testing.T) {
	initial := kit.Dark()
	owner := kit.NewThemeOwner(initial)
	branchText := grid.Style{FG: grid.RGBColor(1, 2, 3)}
	branch := owner.Scope().With(kit.ThemeOverride{Text: &branchText})
	instanceText := grid.Style{} // An explicit terminal-default style is an override.
	first := kit.NewButton(kit.ButtonConfig{Label: "one", Scope: branch})
	second := kit.NewButton(kit.ButtonConfig{Label: "two", Scope: branch, Appearance: kit.ThemeOverride{Text: &instanceText}})
	// Disabled buttons use Muted; supply an action so this test observes Text.
	active := func(context.Context) (scene.ActionResult[struct{}], error) {
		return scene.ActionResult[struct{}]{State: scene.Applied}, nil
	}
	third := kit.NewButton(kit.ButtonConfig{Label: "three", Scope: branch, Activate: active})
	fourth := kit.NewButton(kit.ButtonConfig{Label: "four", Scope: branch, Appearance: kit.ThemeOverride{Text: &instanceText}, Activate: active})
	group := scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Items: []scene.Item{{Element: first.Element(), Size: layout.Fixed(1)}, {Element: second.Element(), Size: layout.Fixed(1)}, {Element: third.Element(), Size: layout.Fixed(1)}, {Element: fourth.Element(), Size: layout.Fixed(1)}}})
	root := newTestRoot(group.Element())
	if err := fourth.Element().RequestFocus(); err != nil {
		t.Fatal(err)
	}
	// A scope owns the pointed-to override, not the caller's variable.
	branchText = grid.Style{FG: grid.RGBColor(9, 8, 7)}
	instanceText = branchText
	snapshot := branch.Snapshot()
	if snapshot.Palette.Text.FG != grid.RGBColor(1, 2, 3) || snapshot.Revision != 1 {
		t.Fatal("scope retained caller-owned token pointers")
	}
	surface := grid.NewSurface(12, 4)
	root.Draw(surface.View())
	if cellAt(surface, 0, 2).Style.FG != grid.RGBColor(1, 2, 3) {
		t.Fatal("subtree override was not used by component")
	}
	// Focus style changes accents, so read the explicit override through a separate
	// owner-free snapshot with the same instance construction in the next frame.
	wake := root.root.Changes()
	updated := kit.Light()
	owner.Set(updated)
	select {
	case <-wake:
	default:
		t.Fatal("theme change did not wake the scene")
	}
	if owner.Revision() != 2 || branch.Snapshot().Revision != 2 || branch.Snapshot().Palette.Muted != updated.Muted {
		t.Fatal("scope copied a stale application palette")
	}
	if owner.Set(updated) || owner.Revision() != 2 {
		t.Fatal("no-op palette update advanced revision")
	}
	if err := third.Element().RequestFocus(); err != nil {
		t.Fatal(err)
	}
	root.Draw(surface.View())
	if cellAt(surface, 0, 3).Style.FG != (grid.Color{}) {
		t.Fatal("explicit instance terminal default was treated as absent")
	}
	if cellAt(surface, 0, 0).Style.FG != updated.Muted.FG {
		t.Fatal("a component retained an independent live theme copy")
	}
}

func TestThemeUpdateDuringPaintIsRejectedBeforeVersionOrPaletteChanges(t *testing.T) {
	initial := kit.Dark()
	owner := kit.NewThemeOwner(initial)
	button := kit.NewButton(kit.ButtonConfig{Label: "save", Scope: owner.Scope()})
	if err := button.Element().SetPaint(func(scene.Frame) { owner.Set(kit.Light()) }); err != nil {
		t.Fatal(err)
	}
	root := newTestRoot(button)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("theme changed during Paint")
			}
		}()
		root.Draw(grid.NewSurface(10, 1).View())
	}()
	if owner.Revision() != 1 || owner.Scope().Snapshot().Palette != initial {
		t.Fatal("rejected update partially changed theme")
	}
}

func TestKitButtonUsesOneControllerForKeysPointerAndAX(t *testing.T) {
	calls := 0
	button := kit.NewButton(kit.ButtonConfig{Label: "Save", Activate: func(context.Context) (scene.ActionResult[struct{}], error) {
		calls++
		return scene.ActionResult[struct{}]{State: scene.Applied}, nil
	}})
	if button.Element() != button.Controller().Element() {
		t.Fatal("skin created a second behaviour identity")
	}
	root := newTestRoot(button)
	root.Draw(grid.NewSurface(10, 1).View())
	root.Handle(input.Key{Code: input.Enter})
	root.Handle(input.Mouse{Button: input.ButtonLeft, Action: input.MouseDown})
	if calls != 1 {
		t.Fatal("button activated before matching release")
	}
	root.Handle(input.Mouse{Button: input.ButtonLeft, Action: input.MouseUp})
	invocation, err := root.root.Invoke(context.Background(), button.Element().Target(), scene.ActionActivate, struct{}{})
	if err != nil || invocation.State != scene.Applied || calls != 3 {
		t.Fatalf("calls=%d invocation=%v err=%v", calls, invocation, err)
	}
}
