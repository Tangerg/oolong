package headless

import (
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
)

// Tab is one pane of a [Tabs] and what it is called.
type Tab struct {
	// Title is what the tab is called. It is here rather than beside whatever draws
	// the bar because the name belongs to the pane: a tab strip built from a separate
	// list of titles is a list that goes out of step with the panes it names.
	Title string
	// Of is the pane. It may be nil, which is a tab with nothing in it yet.
	Of Widget
}

// Tabs is more panes than fit, with one of them showing.
//
// It draws only the pane that is showing. The strip of names is appearance — where
// it sits, what marks the one selected, whether there is a rule under it — and a
// behaviour that drew one would have decided all of that for everybody. Whatever
// draws the strip asks which tab is selected and calls [Tabs.Select] when one is
// clicked, which is the same division a list and its rows already have.
//
// The zero Tabs has no panes and answers nothing.
type Tabs struct {
	// Items are the panes, in the order they are shown and walked through.
	Items []Tab
	// Keys say which keystrokes move between panes — see [Tabs.Do]. Nil reads through
	// [DefaultTabsKeys].
	Keys *keymap.Map
	// NoWrap stops the walk at either end. Wrapping is on by default, which is the
	// other way round from a list: tabs are few and are walked in a ring, where a
	// long list wrapped is a reader who has lost their place.
	NoWrap bool

	selected int
	blurred  bool
	pending  keymap.Pending
}

// Selected is which pane is showing, or -1 when there are none.
func (t *Tabs) Selected() int {
	if len(t.Items) == 0 {
		return -1
	}
	return min(max(t.selected, 0), len(t.Items)-1)
}

// Current is the pane that is showing, and whether there is one.
func (t *Tabs) Current() (Tab, bool) {
	at := t.Selected()
	if at < 0 {
		return Tab{}, false
	}
	return t.Items[at], true
}

// Select shows a pane, clamped to the ones there are, and hands it the keyboard if
// this has it.
func (t *Tabs) Select(at int) {
	was := t.Selected()
	t.selected = min(max(at, 0), max(len(t.Items)-1, 0))
	if now := t.Selected(); now != was {
		t.tell(was, false)
		t.tell(now, !t.blurred)
	}
}

// Move steps by n panes, wrapping unless told not to.
func (t *Tabs) Move(n int) bool {
	if len(t.Items) < 2 {
		return false
	}
	next := t.Selected() + n
	if !t.NoWrap {
		size := len(t.Items)
		next = ((next % size) + size) % size
	}
	was := t.Selected()
	t.Select(next)
	return t.Selected() != was
}

// Handle gives the event to the pane that is showing, and answers the keys that
// move between panes with whatever it declined.
//
// The pane is asked first. A tab strip that took its keys before its contents did
// would be a strip that stole a key from an editor inside it, which is the same rule
// a window keeps with what is in it.
func (t *Tabs) Handle(ev input.Event) bool {
	if pane, ok := t.Current(); ok {
		if handler, can := pane.Of.(Interactive); can && handler.Handle(ev) {
			return true
		}
	}
	key, ok := ev.(input.Key)
	if !ok {
		return false
	}
	action, mine := t.keys().Lookup(key, &t.pending)
	switch {
	case !mine:
		return false
	case action == "":
		return true // the start of a binding more than one chord long
	}
	return t.Do(action)
}

// Do runs one of the actions this answers to by name, reporting whether it was one
// it knows. See [Doer].
//
// What the pane knows is tried first, for the same reason it gets an event first: a
// pane driven from a menu should answer for itself before the thing holding it does.
func (t *Tabs) Do(action keymap.Action) bool {
	if pane, ok := t.Current(); ok {
		if doer, can := pane.Of.(Doer); can && doer.Do(action) {
			return true
		}
	}
	switch action {
	case NextTab:
		return t.Move(1)
	case PrevTab:
		return t.Move(-1)
	default:
		return false
	}
}

// Focus takes the keyboard, or gives it up, and passes the news to the pane showing.
func (t *Tabs) Focus(has bool) {
	t.blurred = !has
	t.tell(t.Selected(), has)
}

// tell hands the keyboard to a pane, or takes it away, when the pane can hold it.
func (t *Tabs) tell(at int, has bool) {
	if at < 0 || at >= len(t.Items) {
		return
	}
	if focusable, ok := t.Items[at].Of.(Focusable); ok {
		focusable.Focus(has)
	}
}

// Measure is what the pane showing asks for.
func (t *Tabs) Measure(across int) int {
	pane, ok := t.Current()
	if !ok {
		return 0
	}
	if sized, can := pane.Of.(Sized); can {
		return sized.Measure(across)
	}
	return 0
}

// Draw paints the pane that is showing into the whole of v.
func (t *Tabs) Draw(v Frame) {
	if pane, ok := t.Current(); ok && pane.Of != nil {
		pane.Of.Draw(v)
	}
}

// keys is the map to read through, standing in the default for a caller who set
// none.
func (t *Tabs) keys() *keymap.Map {
	if t.Keys != nil {
		return t.Keys
	}
	return tabsKeys()
}
