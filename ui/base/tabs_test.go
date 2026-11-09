package base_test

import (
	"image"
	"math"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

func TestTabsMountOnlyTheSelectedPaneAndPreserveKeyedSelection(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	first := base.NewEditor(base.EditorConfig{Initial: "first"})
	second := base.NewEditor(base.EditorConfig{Initial: "second"})
	tabs := base.NewTabs(base.TabsConfig{Items: []base.Tab{{Key: "a", Title: "one", Content: first.Element()}, {Key: "b", Title: "two", Content: second.Element()}}, Initial: 1})
	root := scenes.Mount(tabs)
	root.Draw(grid.NewSurface(20, 4).View())
	if first.Element().Alive() || !second.Element().Alive() {
		t.Fatal("inactive pane is mounted")
	}
	tabs.Set(tabs.Items()[1], tabs.Items()[0])
	root.FlushNotifications()
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
	tabs.SetNoWrap(false)
	tabs.Move(-1)
	if tabs.Selected() != 2 {
		t.Fatal("tab wrap did not reach end")
	}
}

func TestTabStripUsesDistinctScenePartsForPointerAndKeyboard(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	tabs := base.NewTabs(base.TabsConfig{Gap: 1, Items: []base.Tab{{Title: "one"}, {Title: "two"}}})
	root := scenes.Mount(tabs)
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
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	first := base.NewEditor(base.EditorConfig{Initial: "a"})
	second := base.NewEditor(base.EditorConfig{Initial: "b"})
	tabs := base.NewTabs(base.TabsConfig{Items: []base.Tab{{Title: "one", Content: first.Element()}, {Title: "two", Content: second.Element()}}})
	root := scenes.Mount(tabs)
	root.Draw(grid.NewSurface(12, 3).View())
	if err := first.Element().RequestFocus(); err != nil {
		t.Fatal(err)
	}
	root.FlushNotifications()
	tabs.Select(1)
	root.FlushNotifications()
	if first.Element().Focused() || !second.Element().Focused() {
		t.Fatal("focused pane transition did not settle on new pane")
	}
	if err := tabs.Strip().RequestFocusFirst(); err != nil {
		t.Fatal(err)
	}
	root.FlushNotifications()
	before := root.Focus()
	tabs.Select(0)
	root.FlushNotifications()
	if root.Focus() != before {
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

func TestTabsPublishBothRelationsBeforeFocusProjection(t *testing.T) {
	a, b := scene.NewElement(scene.ElementConfig{}), scene.NewElement(scene.ElementConfig{})
	tabs := base.NewTabs(base.TabsConfig{Items: []base.Tab{{Key: "a", Content: a}}})
	armed := false
	calls := 0
	if err := tabs.Element().SetFocusable(func() bool {
		if armed {
			calls++
			current, _ := tabs.Current()
			if current.Key != "b" || a.Alive() || !b.Alive() {
				t.Errorf("focus saw selected=%s old attached=%t new attached=%t", current.Key, a.Alive(), b.Alive())
			}
			panic("projection interrupted")
		}
		return true
	}); err != nil {
		t.Fatal(err)
	}
	root, err := scene.NewRoot(scene.RootConfig{Element: tabs.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	armed = true
	func() {
		defer func() {
			if got := recover(); got != "projection interrupted" {
				t.Fatalf("panic=%v", got)
			}
		}()
		tabs.Set(base.Tab{Key: "b", Content: b})
	}()
	armed = false
	if current, _ := tabs.Current(); calls != 1 || current.Content != b || !current.Content.Alive() || a.Alive() {
		t.Fatalf("partial tab publication after focus callback: current=%v old=%t new=%t calls=%d", current.Key, a.Alive(), b.Alive(), calls)
	}
}

func TestRefreshingAContainerPreservesFocusInsideItsRetainedPane(t *testing.T) {
	scenes := scenetest.New()
	t.Cleanup(scenes.Close)

	for _, name := range []string{"tabs", "viewport"} {
		t.Run(name, func(t *testing.T) {
			first, second := new(base.Editor), new(base.Editor)
			pane := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Element: first.Element()}, {Element: second.Element()}}}).Element()
			var container scene.Element
			var refresh func()
			if name == "tabs" {
				selected := base.Tab{Key: "selected", Content: pane}
				tabs := base.NewTabs(base.TabsConfig{Items: []base.Tab{selected, {Key: "other"}}})
				container = tabs.Element()
				refresh = func() { tabs.Set(tabs.Items()[1], selected) }
			} else {
				viewport := base.NewViewport(base.ViewportConfig{Content: pane})
				container = viewport.Element()
				refresh = func() { viewport.SetContent(pane) }
			}
			root := scenes.Mount(container)
			defer root.Root.Close()
			if err := second.Element().RequestFocus(); err != nil {
				t.Fatal(err)
			}
			refresh()
			if !second.Element().Focused() {
				t.Fatal("refresh moved focus away from the retained field")
			}
		})
	}
}
