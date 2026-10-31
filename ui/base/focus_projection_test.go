package base_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestListFocusFollowsSceneBeforeLifecycleCallbacks(t *testing.T) {
	var a, b base.List[int]
	part := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Element: a.Element()}, {Element: b.Element()}}})
	if a.Focused() || b.Focused() {
		t.Error("unmounted list claims keyboard focus")
	}
	root, err := scene.NewRoot(scene.RootConfig{Element: part.Element()})
	if err != nil {
		t.Fatal(err)
	}
	if !a.Focused() || b.Focused() {
		t.Error("newly mounted list claims another list's focus")
	}
	if err := b.Element().RequestFocus(); err != nil {
		t.Fatal(err)
	}
	if a.Focused() || !b.Focused() {
		t.Error("list waits for callbacks to observe the current keyboard target")
	}
	root.Close()
	if a.Focused() || b.Focused() {
		t.Error("closed list retains keyboard focus")
	}
}

func TestOnlyTheFocusedEditorPaintsTheTerminalCursor(t *testing.T) {
	for _, singleLine := range []bool{false, true} {
		a, b := base.NewEditor(base.EditorConfig{SingleLine: singleLine}), base.NewEditor(base.EditorConfig{SingleLine: singleLine})
		part := scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Items: []scene.Item{{Element: a.Element(), Size: layout.Fixed(1)}, {Element: b.Element(), Size: layout.Fixed(1)}}})
		root, err := scene.NewRoot(scene.RootConfig{Element: part.Element()})
		if err != nil {
			t.Fatal(err)
		}
		root.FlushNotifications()
		check := func(sequence uint64, y int) {
			screen := grid.NewScreen(12, 2)
			prepared, err := root.Prepare(screen.Frame())
			if err != nil {
				t.Fatal(err)
			}
			prepared.Commit(sequence)
			root.Wrote(sequence)
			if c := screen.Cursor(); !c.Visible || c.Pos.Y != y {
				t.Errorf("singleLine=%t terminal cursor=%+v want focused row %d", singleLine, c, y)
			}
		}
		check(1, 0)
		if err := b.Element().RequestFocus(); err != nil {
			t.Fatal(err)
		}
		check(2, 1)
		root.Close()
	}
}
