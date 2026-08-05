package headless_test

import (
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
)

func TestOnePaneShowsAndTheRestWait(t *testing.T) {
	first, second := &tall{rows: 2}, &tall{rows: 3}
	tabs := &headless.Tabs{Items: []headless.Tab{
		{Title: "one", Of: first},
		{Title: "two", Of: second},
	}}

	if got := paint(6, 2, tabs.Draw); got[0] != "row 0." {
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

func TestThePaneShowingAnswersBeforeTheTabsDo(t *testing.T) {
	// A pane that took the arrow keys keeps them. Tabs that took them first would
	// make a list inside one impossible to move through.
	pane := &tall{rows: 4}
	tabs := &headless.Tabs{Items: []headless.Tab{{Title: "one", Of: pane}, {Title: "two"}}}

	if !tabs.Handle(input.Mouse{Action: input.MouseDown}) {
		t.Fatal("the pane was not offered the event")
	}
	if tabs.Selected() != 0 {
		t.Fatal("something the pane answered moved the tabs")
	}
}

func TestTheKeyboardFollowsThePaneThatIsShowing(t *testing.T) {
	first, second := &tall{rows: 1}, &tall{rows: 1}
	tabs := &headless.Tabs{Items: []headless.Tab{{Of: first}, {Of: second}}}
	tabs.Focus(true)
	if first.told != 1 {
		t.Fatal("the pane showing was not told it has the keyboard")
	}
	tabs.Select(1)
	if second.told != 1 {
		t.Fatal("the pane moved to was not told it has the keyboard")
	}
}
