package base_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestDialogOutsideDismissPolicyAndCoordinateTranslation(t *testing.T) {
	for _, outside := range []bool{false, true} {
		t.Run(map[bool]string{false: "retain", true: "dismiss"}[outside], func(t *testing.T) {
			dialog, _, body, root := dialogFixture(t, outside)
			dialog.Show()
			root.Draw(grid.NewSurface(12, 5).View())
			root.Handle(regionPress(3, 1))
			_, col := body.Cursor()
			if col != 1 {
				t.Fatalf("local coordinate=%d", col)
			}
			root.Handle(regionRelease(3, 1))
			root.Handle(regionPress(0, 0))
			if dialog.Open() == outside {
				t.Fatal("outside policy was not honored")
			}
		})
	}
}

func TestModalContentMayConsumeEscapeAndKeysCanBeRebound(t *testing.T) {
	consumed := 0
	body := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }, Handle: func(event scene.Event) scene.EventResult {
		if key, ok := event.Input.(input.Key); ok && key.Code == input.Esc {
			consumed++
			return scene.EventResult{Consumed: true}
		}
		return scene.EventResult{}
	}})
	var keys keymap.Map
	keys.Bind(base.Close, input.Chord{Rune: 'q'})
	dialog := base.NewDialog(base.DialogConfig{Initial: true, Content: body, Keys: &keys})
	root := newTestRoot(dialog)
	root.Draw(grid.NewSurface(10, 2).View())
	root.Handle(input.Key{Code: input.Esc})
	if consumed != 1 || !dialog.Open() {
		t.Fatal("dialog intercepted child's escape")
	}
	root.Handle(input.Key{Rune: 'q'})
	if dialog.Open() {
		t.Fatal("rebound dismiss did not close")
	}
}

func TestDialogPlacementAndBackdropShareOneSceneFrame(t *testing.T) {
	var backdrop, body image.Rectangle
	child := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { body = frame.Bounds(); frame.Text(0, 0, "x", grid.Style{}) }})
	dialog := base.NewDialog(base.DialogConfig{Initial: true, Content: child, Place: func(image.Point) image.Rectangle { return image.Rect(3, 2, 7, 4) }, Backdrop: func(frame scene.Frame) { backdrop = frame.Bounds() }})
	rows := paintWidget(10, 5, dialog)
	if backdrop != image.Rect(0, 0, 10, 5) || body != image.Rect(0, 0, 4, 2) || rows[2] != "...x......" {
		t.Fatalf("placement backdrop=%v body=%v rows=%v", backdrop, body, rows)
	}
}
