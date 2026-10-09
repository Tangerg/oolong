package headless_test

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

type keyboardTransition struct {
	reentrant
	handle func()
	do     func()
	rows   int
}

func (w *keyboardTransition) HeightForWidth(int) int { return max(w.rows, 1) }

func (*keyboardTransition) Place(image.Point) layout.Placement {
	return layout.Placement{Anchor: layout.TopLeft, Width: 4, Height: 2}
}

func (w *keyboardTransition) Handle(input.Event) bool {
	if w.handle != nil {
		w.handle()
	}
	return false
}

func (w *keyboardTransition) Do(keymap.Action) bool {
	if w.do != nil {
		w.do()
	}
	return false
}

func TestChildFocusTransferEndsContainerKeyPropagation(t *testing.T) {
	a, b, c := &keyboardTransition{}, &keyboardTransition{}, &keyboardTransition{}
	container := headless.NewContainer(layout.Down, headless.Item{Of: a}, headless.Item{Of: b}, headless.Item{Of: c})
	a.handle = func() { container.FocusIndex(1) }
	if !container.Handle(input.Key{Code: input.Tab}) || container.Focused() != b {
		t.Fatal("an old key traversed again after its child transferred focus")
	}
}

func TestFocusRoundTripEndsContainerKeyPropagation(t *testing.T) {
	a, b := &keyboardTransition{}, &keyboardTransition{}
	container := headless.NewContainer(layout.Down, headless.Item{Of: a}, headless.Item{Of: b})
	a.handle = func() { container.Focus(false); container.Focus(true) }
	if !container.Handle(input.Key{Code: input.Tab}) || container.Focused() != a {
		t.Fatal("a focus round trip restored authority to propagate the old key")
	}
}

func TestSelectionRoundTripEndsTabsKeyPropagation(t *testing.T) {
	a, b := &keyboardTransition{}, &keyboardTransition{}
	tabs := headless.NewTabs(headless.TabsConfig{Items: []headless.Tab{{Of: a}, {Of: b}}})
	a.handle = func() { tabs.Select(1); tabs.Select(0) }
	if !tabs.Handle(input.Key{Code: input.Right, Mods: input.Alt}) || tabs.Selected() != 0 {
		t.Fatal("an attachment round trip restored authority to propagate the old key")
	}
}

func TestPaneTransitionEndsTabsKeyAndActionPropagation(t *testing.T) {
	for _, method := range []string{"key", "action"} {
		t.Run(method, func(t *testing.T) {
			a, b, c := &keyboardTransition{}, &keyboardTransition{}, &keyboardTransition{}
			tabs := headless.NewTabs(headless.TabsConfig{Items: []headless.Tab{{Of: a}, {Of: b}, {Of: c}}})
			changed := func() { tabs.Select(1) }
			a.handle, a.do = changed, changed
			b.do = func() { t.Error("an old key reached the replacement pane") }
			if method == "key" {
				tabs.Handle(input.Key{Code: input.Right, Mods: input.Alt})
			} else {
				tabs.Do(headless.NextTab)
			}
			if tabs.Selected() != 1 {
				t.Fatal("an interrupted operation overwrote the pane's selection")
			}
		})
	}
}

func TestViewportReplacementEndsOldScrollPropagation(t *testing.T) {
	for _, method := range []string{"key", "action"} {
		t.Run(method, func(t *testing.T) {
			a, b := &keyboardTransition{rows: 20}, &keyboardTransition{rows: 20}
			viewport := headless.NewViewport(a)
			paintWidget(8, 1, viewport)
			changed := func() { viewport.SetContent(b) }
			a.handle, a.do = changed, changed
			if method == "key" {
				viewport.Handle(input.Key{Code: input.Down})
			} else {
				viewport.Do(headless.ScrollDown)
			}
			if viewport.Scroll().Offset() != 0 {
				t.Fatal("the old content's operation scrolled its replacement")
			}
		})
	}
}

func TestFramedChildReplacementEndsParentKeyPropagation(t *testing.T) {
	for _, kind := range []string{"panel", "dialog panel", "dialog content"} {
		t.Run(kind, func(t *testing.T) {
			a, b, next := &keyboardTransition{}, &keyboardTransition{}, &keyboardTransition{}
			var child headless.Widget
			switch kind {
			case "panel":
				panel := kit.NewPanel(kit.PanelConfig{Content: a})
				child = panel
				a.handle = func() { panel.SetContent(b) }
			case "dialog panel":
				panel := &kit.DialogPanel{}
				panel.SetBody(a)
				child = panel
				a.handle = func() { panel.SetBody(b) }
			case "dialog content":
				dialog := headless.NewDialog(headless.DialogConfig{Stack: headless.NewStack(nil), Content: a})
				child = dialog.Content()
				a.handle = func() { dialog.Content().Focus(false); dialog.Content().Focus(true) }
			}
			container := headless.NewContainer(layout.Down, headless.Item{Of: child}, headless.Item{Of: next})
			if !container.Handle(input.Key{Code: input.Tab}) || container.Focused() != child {
				t.Fatal("a changed inner routing scope authorized navigation in its outer container")
			}
		})
	}
}

func TestRootDeclineBelongsToItsCommittedTree(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "pending", true: "committed"}[commit], func(t *testing.T) {
			a, b := &keyboardTransition{}, &keyboardTransition{}
			root := headless.NewRoot(a)
			view := grid.NewSurface(8, 4).View()
			root.Draw(view)
			a.handle = func() {
				root.SetContent(b)
				if commit {
					root.Draw(view)
				}
			}
			if handled := root.Handle(input.Key{Code: input.Tab}); handled != commit {
				t.Fatalf("Handle = %v for committed replacement = %v", handled, commit)
			}
		})
	}
}

func TestModalFocusRoundTripCannotDismissTheOldScope(t *testing.T) {
	child := &keyboardTransition{}
	stack := headless.NewStack(nil)
	stack.Push(child)
	paintWidget(8, 4, stack)
	child.handle = func() { stack.Focus(false); stack.Focus(true) }
	if !stack.Handle(input.Key{Code: input.Esc}) || stack.Depth() != 1 {
		t.Fatal("a focus round trip allowed the interrupted key to dismiss its modal")
	}
}
