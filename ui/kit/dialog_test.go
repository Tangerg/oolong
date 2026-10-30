package kit_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestDialogComposesPanelAndOneBehaviorOwner(t *testing.T) {
	body := base.NewEditor(base.EditorConfig{Initial: "body"})
	dialog := kit.NewDialog(kit.DialogConfig{Initial: true, Glyphs: kit.ASCII(), Title: "Title", Body: body.Element(), Where: layout.Placement{Width: 16, Height: 5}})
	owner := dialog.Controller()
	if owner != dialog.Controller() || owner.Content() != dialog.Panel().Element() {
		t.Fatal("dialog appearance is not attached through its controller")
	}
	drawn := strings.Join(paintWidget(20, 7, dialog), "\n")
	if !strings.Contains(drawn, "Title") || !strings.Contains(drawn, "body") {
		t.Fatalf("missing dialog composition: %s", drawn)
	}
	owner.SetTitle("New title")
	if !strings.Contains(strings.Join(paintWidget(20, 7, dialog), "\n"), "New title") {
		t.Fatal("title paint read a stale duplicate")
	}
	root := newTestRoot(dialog)
	root.Handle(input.Key{Rune: 'x'})
	if !strings.Contains(body.Text(), "x") {
		t.Fatal("keyboard did not reach dialog body")
	}
	root.Handle(input.Key{Code: input.Esc})
	if owner.Open() {
		t.Fatal("escape did not reach owner")
	}
}

func TestDialogCanReplaceItsPanelBodyWithoutAnotherRouter(t *testing.T) {
	first := base.NewEditor(base.EditorConfig{})
	second := base.NewEditor(base.EditorConfig{})
	dialog := kit.NewDialog(kit.DialogConfig{Initial: true, Body: first.Element()})
	root := newTestRoot(dialog)
	root.Draw(grid.NewSurface(20, 6).View())
	dialog.Panel().SetContent(second.Element())
	root.root.FlushNotifications()
	if first.Element().Alive() || !second.Element().Focused() {
		t.Fatal("panel replacement did not settle scene focus")
	}
}

func TestDialogWithNoBodyAndNoSpaceIsStillDrawable(_ *testing.T) {
	dialog := kit.NewDialog(kit.DialogConfig{Initial: true, Glyphs: kit.ASCII()})
	paintWidget(1, 1, dialog)
	newTestRoot(dialog).Draw(grid.NewSurface(0, 0).View())
}

func TestDialogPlacementUsesConfiguredRectangle(t *testing.T) {
	dialog := kit.NewDialog(kit.DialogConfig{Initial: true, Glyphs: kit.ASCII(), Where: layout.Placement{Width: 4, Height: 3, Anchor: layout.Middle}})
	rows := paintWidget(10, 7, dialog)
	if !strings.Contains(rows[2], "+--+") {
		t.Fatalf("centered rectangle: %v", rows)
	}
}
