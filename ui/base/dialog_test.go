package base_test

import (
	"context"
	"errors"
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

func dialogFixture(t *testing.T, scenes *scenetest.Scope, dismissOutside bool) (*base.Dialog, *base.Button, *base.Editor, *scenetest.Scene) {
	t.Helper()
	body := base.NewEditor(base.EditorConfig{Initial: "body"})
	dialog := base.NewDialog(base.DialogConfig{Title: "Dialog", Description: "description", Content: body.Element(), Place: func(image.Point) image.Rectangle { return image.Rect(2, 1, 10, 4) }, DismissOutside: dismissOutside})
	trigger := dialog.Trigger("Open")
	group := scene.NewGroup(scene.GroupConfig{Overlay: true, Items: []scene.Item{{Element: trigger.Element(), Size: layout.Flex(1)}, {Element: dialog.Element(), Size: layout.Flex(1)}}})
	root := scenes.Mount(group.Element())
	root.Draw(grid.NewSurface(12, 5).View())
	return dialog, trigger, body, root
}

func TestDialogOwnsOpenPolicyAndSceneRestoresFocus(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	dialog, trigger, body, root := dialogFixture(t, scenes, true)
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
	root.FlushNotifications()
	if dialog.Open() || !trigger.Element().Focused() || body.Element().Alive() {
		t.Fatal("dismiss did not revoke body and restore trigger")
	}
}

func TestDialogCanCloseUnderANewerModalWithoutDismissingIt(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	firstBody := base.NewEditor(base.EditorConfig{})
	secondBody := base.NewEditor(base.EditorConfig{})
	first := base.NewDialog(base.DialogConfig{Content: firstBody.Element()})
	second := base.NewDialog(base.DialogConfig{Content: secondBody.Element()})
	group := scene.NewGroup(scene.GroupConfig{Overlay: true, Items: []scene.Item{{Element: first.Element()}, {Element: second.Element()}}})
	root := scenes.Mount(group.Element())
	first.Show()
	second.Show()
	root.Draw(grid.NewSurface(10, 3).View())
	first.Dismiss()
	root.FlushNotifications()
	if first.Open() || !second.Open() || !secondBody.Element().Focused() {
		t.Fatal("older modal dismissed newer scope")
	}
}

func TestDialogTriggerOnlyActivatesOnMatchingRelease(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	dialog, trigger, _, root := dialogFixture(t, scenes, true)
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
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	body := scene.NewElement(scene.ElementConfig{})
	var dialog *base.Dialog
	dialog = base.NewDialog(base.DialogConfig{Initial: true, Content: body, Backdrop: func(scene.Frame) { dialog.Dismiss() }})
	root := scenes.Mount(dialog)
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

func TestDialogTraversesFocusOncePerKeyGoingDown(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	for _, tc := range []struct {
		name  string
		mods  input.Mods
		steps []int
	}{
		{"tab", 0, []int{1, 2, 0}},
		{"shift_tab", input.Shift, []int{2, 1, 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fields := []*base.Text{base.NewText(base.TextConfig{Key: "a"}), base.NewText(base.TextConfig{Key: "b"}), base.NewText(base.TextConfig{Key: "c"})}
			form := base.NewForm(base.FormConfig{Fields: []base.Field{fields[0], fields[1], fields[2]}})
			dialog := base.NewDialog(base.DialogConfig{Initial: true, Content: form.Element(), Place: func(image.Point) image.Rectangle { return image.Rect(0, 0, 20, 8) }})
			root := scenes.Mount(dialog)
			root.Draw(grid.NewSurface(20, 8).View())
			focused := func() int {
				for i, field := range fields {
					if field.Element().Focused() {
						return i
					}
				}
				return -1
			}
			if focused() != 0 {
				t.Fatalf("initial focus = %d", focused())
			}
			press := func(transition input.Transition) {
				root.Handle(input.Key{Code: input.Tab, Mods: tc.mods, Transition: transition})
				root.FlushNotifications()
			}
			for step, transition := range []input.Transition{input.Press, input.Repeat, input.Repeat} {
				press(transition)
				if got, want := focused(), tc.steps[step]; got != want {
					t.Fatalf("transition %d moved focus to %d, want %d", transition, got, want)
				}
			}
			press(input.Release)
			if got, want := focused(), tc.steps[2]; got != want {
				t.Fatalf("release moved focus to %d, want it to stay on %d", got, want)
			}
		})
	}
}

func TestDialogReopenWaitsForDetachedContentCleanup(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	dialog, _, body, root := dialogFixture(t, scenes, true)
	dialog.Show()
	root.Draw(grid.NewSurface(12, 5).View())
	dialog.Dismiss()
	func() {
		defer func() {
			if err, _ := recover().(error); !errors.Is(err, scene.ErrCleanupPending) {
				t.Fatalf("same-turn reopen recovered %v, want ErrCleanupPending", err)
			}
		}()
		dialog.Show()
	}()
	if dialog.Open() {
		t.Fatal("rejected reopen changed open state")
	}
	root.FlushNotifications()
	dialog.Show()
	root.FlushNotifications()
	if !dialog.Open() || !body.Element().Focused() {
		t.Fatal("reopen after cleanup did not restore content")
	}
}
