package headless_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
)

func TestSettingsRoutesNavigationAndValueActionsSeparately(t *testing.T) {
	settings := &headless.Settings[string]{}
	settings.SetItems([]string{"theme", "wrap"})
	var index int
	var item string
	var action keymap.Action
	settings.Change = func(at int, current string, do keymap.Action) bool {
		index, item, action = at, current, do
		return true
	}

	if !settings.Handle(input.Key{Code: input.Down}) || settings.Selected() != 1 {
		t.Fatalf("down selected %d, want row 1", settings.Selected())
	}
	if !settings.Handle(input.Key{Code: input.Right}) {
		t.Fatal("right did not reach the selected setting")
	}
	if index != 1 || item != "wrap" || action != headless.Increase {
		t.Fatalf("change = (%d, %q, %q), want selected row and increase", index, item, action)
	}
	if !settings.Handle(input.Key{Code: input.Enter}) || action != headless.Activate {
		t.Fatalf("enter produced %q, want activate", action)
	}
	if settings.Handle(input.Key{Code: input.Esc}) {
		t.Fatal("an unbound key was consumed")
	}
}

func TestSettingsPointerSelectionUsesTheListCommittedWindow(t *testing.T) {
	settings := &headless.Settings[int]{}
	settings.SetItems([]int{0, 1, 2, 3})
	// A nil row still publishes the list window: appearance is optional, pointer
	// geometry is behaviour.
	paintWidget(8, 2, settings)
	if !settings.Handle(input.Mouse{
		Pos: image.Pt(0, 1), Action: input.MouseDown, Button: input.ButtonLeft,
	}) {
		t.Fatal("press on a visible settings row was declined")
	}
	if settings.Selected() != 1 {
		t.Fatalf("press selected %d, want row 1", settings.Selected())
	}
}
