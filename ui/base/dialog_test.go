package base_test

import (
	"context"
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func dialogFixture(t *testing.T, dismissOutside bool) (*base.Dialog, *base.Button, *base.Editor, *testScene) {
	t.Helper()
	body := base.NewEditor(base.EditorConfig{Initial: "body"})
	dialog := base.NewDialog(base.DialogConfig{Title: "Dialog", Description: "description", Content: body.Element(), Place: func(image.Point) image.Rectangle { return image.Rect(2, 1, 10, 4) }, DismissOutside: dismissOutside})
	trigger := dialog.Trigger("Open")
	group := scene.NewGroup(scene.GroupConfig{Overlay: true, Items: []scene.Item{{Element: trigger.Element(), Size: layout.Flex(1)}, {Element: dialog.Element(), Size: layout.Flex(1)}}})
	root := newTestRoot(group.Element())
	root.Draw(grid.NewSurface(12, 5).View())
	return dialog, trigger, body, root
}

func TestDialogOwnsOpenPolicyAndSceneRestoresFocus(t *testing.T) {
	dialog, trigger, body, root := dialogFixture(t, true)
	if dialog.Open() || !trigger.Element().Focused() {
		t.Fatal("closed dialog stole focus")
	}
	result, err := trigger.Press(t.Context())
	if err != nil || result.State != scene.Applied {
		t.Fatal(result, err)
	}
	root.Draw(grid.NewSurface(12, 5).View())
	if !dialog.Open() || !body.Element().Focused() {
		t.Fatal("opened dialog has no focused body")
	}
	root.Handle(input.Key{Code: input.Esc})
	root.root.FlushNotifications()
	if dialog.Open() || !trigger.Element().Focused() || body.Element().Alive() {
		t.Fatal("dismiss did not revoke body and restore trigger")
	}
}

func TestDialogCanCloseUnderANewerModalWithoutDismissingIt(t *testing.T) {
	firstBody := base.NewEditor(base.EditorConfig{})
	secondBody := base.NewEditor(base.EditorConfig{})
	first := base.NewDialog(base.DialogConfig{Content: firstBody.Element()})
	second := base.NewDialog(base.DialogConfig{Content: secondBody.Element()})
	group := scene.NewGroup(scene.GroupConfig{Overlay: true, Items: []scene.Item{{Element: first.Element()}, {Element: second.Element()}}})
	root := newTestRoot(group.Element())
	first.Show()
	second.Show()
	root.Draw(grid.NewSurface(10, 3).View())
	first.Dismiss()
	root.root.FlushNotifications()
	if first.Open() || !second.Open() || !secondBody.Element().Focused() {
		t.Fatal("older modal dismissed newer scope")
	}
}

func TestDialogTriggerOnlyActivatesOnMatchingRelease(t *testing.T) {
	dialog, trigger, _, root := dialogFixture(t, true)
	root.Handle(regionPress(1, 0))
	if dialog.Open() {
		t.Fatal("button activated before release")
	}
	root.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonRight, Pos: image.Pt(1, 0)})
	if dialog.Open() {
		t.Fatal("foreign button release activated trigger")
	}
	root.Handle(regionRelease(1, 0))
	if !dialog.Open() {
		t.Fatal("matching release did not activate trigger")
	}
	dialog.Dismiss()
	root.Draw(grid.NewSurface(12, 5).View())
	root.Handle(regionPress(1, 0))
	root.Handle(regionRelease(30, 0))
	if dialog.Open() {
		t.Fatal("outside release clicked trigger")
	}
	result, err := trigger.Press(context.Background())
	if err != nil || result.State != scene.Applied {
		t.Fatal("typed action diverged from button", err)
	}
}

func TestDialogRejectsSemanticMutationDuringPaint(t *testing.T) {
	body := scene.NewElement(scene.ElementConfig{})
	var dialog *base.Dialog
	dialog = base.NewDialog(base.DialogConfig{Initial: true, Content: body, Backdrop: func(scene.Frame) { dialog.Dismiss() }})
	root := newTestRoot(dialog)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("paint changed open owner")
			}
		}()
		root.Draw(grid.NewSurface(10, 3).View())
	}()
	if !dialog.Open() {
		t.Fatal("rejected phase changed open state before failure")
	}
}
