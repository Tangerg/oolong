package base_test

import (
	"image"
	"math"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestTabsMountOnlyTheSelectedPaneAndPreserveKeyedSelection(t *testing.T) {
	first := base.NewEditor(base.EditorConfig{Initial: "first"})
	second := base.NewEditor(base.EditorConfig{Initial: "second"})
	tabs := base.NewTabs(base.TabsConfig{Items: []base.Tab{{Key: "a", Title: "one", Content: first.Element()}, {Key: "b", Title: "two", Content: second.Element()}}, Initial: 1})
	root := newTestRoot(tabs)
	root.Draw(grid.NewSurface(20, 4).View())
	if first.Element().Alive() || !second.Element().Alive() {
		t.Fatal("inactive pane is mounted")
	}
	tabs.Set(tabs.Items()[1], tabs.Items()[0])
	root.root.FlushNotifications()
	if tabs.Selected() != 0 || !second.Element().Alive() {
		t.Fatal("keyed reorder changed selected pane")
	}
	snapshot := tabs.Items()
	snapshot[0].Title = "external mutation"
	current, _ := tabs.Current()
	if current.Title != "two" {
		t.Fatal("Items exposed owned tab structure")
	}
}

func TestTabMovementKeepsItsDirectionAtIntegerLimits(t *testing.T) {
	tabs := base.NewTabs(base.TabsConfig{Items: []base.Tab{{Title: "a"}, {Title: "b"}, {Title: "c"}}, NoWrap: true})
	tabs.Select(1)
	if !tabs.Move(math.MaxInt) || tabs.Selected() != 2 {
		t.Fatal("forward overflow changed direction")
	}
	if !tabs.Move(math.MinInt) || tabs.Selected() != 0 {
		t.Fatal("backward overflow changed direction")
	}
	tabs.NoWrap = false
	tabs.Move(-1)
	if tabs.Selected() != 2 {
		t.Fatal("tab wrap did not reach end")
	}
}

func TestTabStripUsesDistinctScenePartsForPointerAndKeyboard(t *testing.T) {
	tabs := base.NewTabs(base.TabsConfig{Gap: 1, Items: []base.Tab{{Title: "one"}, {Title: "two"}}})
	root := newTestRoot(tabs)
	root.Draw(grid.NewSurface(12, 2).View())
	if !root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(5, 0)}) || tabs.Selected() != 1 {
		t.Fatal("second tab was not hit through its scene part")
	}
	if !root.Handle(input.Key{Code: input.Left, Mods: input.Alt}) || tabs.Selected() != 0 {
		t.Fatal("tab command did not reach group")
	}
	if root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonRight, Pos: image.Pt(5, 0)}) {
		t.Fatal("secondary click selected a tab")
	}
}

func TestTabChangeMovesPaneFocusWithoutStealingStripFocus(t *testing.T) {
	first := base.NewEditor(base.EditorConfig{Initial: "a"})
	second := base.NewEditor(base.EditorConfig{Initial: "b"})
	tabs := base.NewTabs(base.TabsConfig{Items: []base.Tab{{Title: "one", Content: first.Element()}, {Title: "two", Content: second.Element()}}})
	root := newTestRoot(tabs)
	root.Draw(grid.NewSurface(12, 3).View())
	if err := first.Element().RequestFocus(); err != nil {
		t.Fatal(err)
	}
	root.root.FlushNotifications()
	tabs.Select(1)
	root.root.FlushNotifications()
	if first.Element().Focused() || !second.Element().Focused() {
		t.Fatal("focused pane transition did not settle on new pane")
	}
	if err := tabs.Strip().RequestFocusFirst(); err != nil {
		t.Fatal(err)
	}
	root.root.FlushNotifications()
	before := root.root.Focus()
	tabs.Select(0)
	root.root.FlushNotifications()
	if root.root.Focus() != before {
		t.Fatal("selection stole strip focus")
	}
}

func TestTabsRejectFailedStructureBeforeChangingSelection(t *testing.T) {
	tabs := base.NewTabs(base.TabsConfig{Items: []base.Tab{{Key: "a", Title: "one"}}})
	before := tabs.Items()
	func() {
		defer func() {
			if recover() == nil {
				t.Error("duplicate tab key accepted")
			}
		}()
		tabs.Set(base.Tab{Key: "dup"}, base.Tab{Key: "dup"})
	}()
	if len(tabs.Items()) != 1 || tabs.Items()[0] != before[0] {
		t.Fatal("failed tab transition changed owned structure")
	}
	foreign := scene.NewElement(scene.ElementConfig{})
	_ = scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Element: foreign}}})
	func() {
		defer func() {
			if recover() == nil {
				t.Error("already owned pane accepted")
			}
		}()
		tabs.Set(base.Tab{Key: "new", Content: foreign})
	}()
	if tabs.Items()[0] != before[0] {
		t.Fatal("failed mounting partially published tab metadata")
	}
}
