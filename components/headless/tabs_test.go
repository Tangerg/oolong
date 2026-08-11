package headless_test

import (
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

func TestOnePaneShowsAndTheRestWait(t *testing.T) {
	first, second := &tall{rows: 2}, &tall{rows: 3}
	tabs := headless.NewTabs(
		headless.Tab{Title: "one", Of: first},
		headless.Tab{Title: "two", Of: second},
	)

	if got := paintWidget(6, 2, tabs); got[0] != "row 0." {
		t.Fatalf("the first pane drew %q", got)
	}
	if !tabs.Handle(input.Key{Code: input.Right, Mods: input.Alt}) {
		t.Fatal("the keystroke that moves between panes was not answered")
	}
	if tabs.Selected() != 1 {
		t.Fatalf("the pane showing is %d", tabs.Selected())
	}
	if got := tabs.Measure(6); got != 3 {
		t.Fatalf("the tabs asked for %d rows, want what the pane showing asks for", got)
	}

	// And round, because tabs are few and are walked in a ring.
	tabs.Handle(input.Key{Code: input.Right, Mods: input.Alt})
	if tabs.Selected() != 0 {
		t.Fatalf("walking past the last pane left %d showing", tabs.Selected())
	}
}

func TestTabMovementKeepsItsDirectionAtIntegerLimits(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1
	tabs := headless.NewTabs(headless.Tab{}, headless.Tab{}, headless.Tab{})
	tabs.NoWrap = true
	tabs.Select(1)
	if !tabs.Move(maxInt) || tabs.Selected() != 2 {
		t.Fatalf("largest forward movement left tab %d, want the end", tabs.Selected())
	}
	if !tabs.Move(minInt) || tabs.Selected() != 0 {
		t.Fatalf("largest backward movement left tab %d, want the start", tabs.Selected())
	}
}

func TestThePaneShowingAnswersBeforeTheTabsDo(t *testing.T) {
	// A pane that took the arrow keys keeps them. Tabs that took them first would
	// make a list inside one impossible to move through.
	pane := &tall{rows: 4}
	tabs := headless.NewTabs(headless.Tab{Title: "one", Of: pane}, headless.Tab{Title: "two"})

	if !tabs.Handle(input.Mouse{Action: input.MouseDown}) {
		t.Fatal("the pane was not offered the event")
	}
	if tabs.Selected() != 0 {
		t.Fatal("something the pane answered moved the tabs")
	}
}

func TestTheKeyboardFollowsThePaneThatIsShowing(t *testing.T) {
	first, second := &tall{rows: 1}, &tall{rows: 1}
	tabs := headless.NewTabs(headless.Tab{Of: first}, headless.Tab{Of: second})
	tabs.Focus(true)
	if first.told != 1 {
		t.Fatal("the pane showing was not told it has the keyboard")
	}
	tabs.Select(1)
	if second.told != 1 {
		t.Fatal("the pane moved to was not told it has the keyboard")
	}
}

func TestTabsAcceptNonComparableValuePanes(t *testing.T) {
	first := valueField{target: &field{name: "first"}, payload: []byte("first")}
	second := valueField{target: &field{name: "second"}, payload: []byte("second")}
	tabs := headless.NewTabs(
		headless.Tab{Title: "first", Of: first},
		headless.Tab{Title: "second", Of: second},
	)
	tabs.Select(1)
	if got := tabs.Selected(); got != 1 {
		t.Fatalf("selected = %d, want 1", got)
	}
}

func TestControlledTabsUseOneSelectionAndSyncExternalTransitions(t *testing.T) {
	selected := 1
	first, second := &focusProbe{}, &focusProbe{}
	tabs := headless.NewControlledTabs(
		headless.Bind(&selected),
		headless.Tab{Title: "one", Of: first},
		headless.Tab{Title: "two", Of: second},
	)
	if tabs.Selected() != 1 || !second.focused {
		t.Fatalf("controlled tabs started selected=%d first=%v second=%v",
			tabs.Selected(), first.changes, second.changes)
	}
	tabs.Select(0)
	if selected != 0 || !first.focused || second.focused {
		t.Fatal("select did not write state and transfer focus")
	}

	selected = 1
	tabs.Sync()
	if !second.focused || first.focused {
		t.Fatal("sync did not transfer focus after an owner-written selection")
	}

	selected = 99
	tabs.Sync()
	if selected != 1 || tabs.Selected() != 1 {
		t.Fatalf("sync left binding=%d selected=%d, want both clamped to 1", selected, tabs.Selected())
	}
}

func TestTabsExposeStructuralSemanticsIndependentOfTheStrip(t *testing.T) {
	tabs := headless.NewTabs(
		headless.Tab{Title: "chat", Of: &focusProbe{}},
		headless.Tab{Title: "files", Of: &focusProbe{}},
	)
	tabs.Select(1)
	node := tabs.Semantics()
	if node.Role != headless.RoleTabList || !node.State.Has(headless.StateFocused) {
		t.Fatalf("tab-list semantics = %+v", node)
	}
	if len(node.Children) != 3 {
		t.Fatalf("semantic children = %d, want two tabs and one panel", len(node.Children))
	}
	if node.Children[0].State.Has(headless.StateSelected) ||
		!node.Children[1].State.Has(headless.StateSelected) ||
		node.Children[2].Role != headless.RoleTabPanel || node.Children[2].Label != "files" {
		t.Fatalf("semantic parts = %+v", node.Children)
	}
}

func TestAListCanHoldTheKeyboard(t *testing.T) {
	// Without it a list could not be one of a container's children, which is what a
	// two-pane interface is made of. It draws no differently for it — that is the
	// caller's — and this is how a row asks.
	var list headless.List[string]
	list.SetItems([]string{"one", "two"})
	if !list.Focused() {
		t.Fatal("a list nobody has said anything to does not have the keyboard")
	}
	list.Focus(false)
	if list.Focused() {
		t.Fatal("a list told it lost the keyboard still has it")
	}

	// A container can therefore hold one, which is the point.
	body := headless.NewContainer(layout.Down, headless.Item{Size: layout.Fixed(1), Of: &list})
	if !body.Give(0) {
		t.Fatal("a container would not hand the keyboard to a list")
	}
	if !list.Focused() {
		t.Fatal("the list was not told it has the keyboard")
	}
}
