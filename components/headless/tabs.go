package headless

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
)

// Tab is one compound part of [Tabs]: a named pane.
type Tab struct {
	// Title is what the tab is called. It is here rather than beside whatever draws
	// the strip because the name belongs to the pane: an appearance built from a
	// separate list of titles is a list that can drift out of step with the panes it
	// names. Semantics reads the same value for the same reason.
	Title string
	// Of is the pane. It may be nil, which is a tab with nothing in it yet.
	Of Widget
}

// Tabs is the behavior and semantic owner of a set of named panes.
//
// It draws only the selected pane. The strip of names is appearance — where it sits,
// what marks the one selected, whether there is a rule under it — and a behavior that
// drew one would have decided all of that for everybody. Whatever draws the strip asks
// which tab is selected and calls [Tabs.Select] when one is clicked, the same division
// a list keeps between selection and rows. [SemanticNode] describes the tab list and
// panel without coupling either to those cells.
//
// Construct it with [NewTabs] so selection ownership is explicit in one configuration.
// The zero value is an empty locally owned controller and is safe, but Set is the only
// way to give it parts.
type Tabs struct {
	items     []Tab
	selection ownedValue[int]

	// Keys say which keystrokes move between panes. Nil reads through
	// [DefaultTabsKeys].
	Keys *keymap.Map
	// NoWrap stops the walk at either end. Wrapping is on by default because tabs are
	// few and walked as a ring, unlike a long list where wrapping loses the reader's
	// place.
	NoWrap bool

	holder      Widget
	holderIndex int
	settled     bool
	blurred     bool
	matcher     keymap.Matcher
}

// TabsConfig is the complete construction state of [Tabs].
//
// A nil Selection gives the controller local ownership. An accessor gives ownership
// to the caller without selecting a different constructor or maintaining a shadow
// value. The zero value constructs an empty, locally owned controller.
type TabsConfig struct {
	// Items are copied in display order.
	Items []Tab
	// Selection is optional caller-owned state. Nil keeps state local.
	Selection Accessor[int]
	// Keys maps tab actions. Nil uses [DefaultTabsKeys].
	Keys *keymap.Map
	// NoWrap stops movement at the first and last tab.
	NoWrap bool
}

// NewTabs constructs one tabs controller from config.
//
// With Selection set, selection operations write the accessor directly. When its
// owner writes it independently, it calls [Tabs.Sync] so focus moves as the same
// semantic transition.
func NewTabs(config TabsConfig) *Tabs {
	tabs := &Tabs{
		selection: newOwnedValue(0, config.Selection),
		Keys:      config.Keys,
		NoWrap:    config.NoWrap,
	}
	tabs.Set(config.Items...)
	return tabs
}

// Set replaces the tab parts and preserves a valid selected index.
func (t *Tabs) Set(items ...Tab) {
	if t == nil {
		return
	}
	t.items = own(t.items, items)
	for i := range t.items {
		t.items[i].Title = strings.Clone(t.items[i].Title)
	}
	if len(t.items) == 0 {
		t.selection.set(0)
	} else {
		t.selection.set(min(max(t.selection.get(), 0), len(t.items)-1))
	}
	t.settled = false
	t.settle()
}

// Items returns a copy of the current tab parts. Replacing the returned slice cannot
// bypass selection clamping or focus settlement; use [Tabs.Set] to change the parts.
func (t *Tabs) Items() []Tab {
	if t == nil {
		return nil
	}
	return slices.Clone(t.items)
}

// Len returns the number of tab parts.
func (t *Tabs) Len() int {
	if t == nil {
		return 0
	}
	return len(t.items)
}

// At returns one tab part and whether index exists.
func (t *Tabs) At(index int) (Tab, bool) {
	if t == nil || index < 0 || index >= len(t.items) {
		return Tab{}, false
	}
	return t.items[index], true
}

// Selected is which pane is showing, or -1 when there are none.
func (t *Tabs) Selected() int {
	if t == nil || len(t.items) == 0 {
		return -1
	}
	return min(max(t.selection.get(), 0), len(t.items)-1)
}

// Current is the pane that is showing, and whether there is one.
func (t *Tabs) Current() (Tab, bool) {
	at := t.Selected()
	if at < 0 {
		return Tab{}, false
	}
	return t.items[at], true
}

// Select shows a pane, clamped to the parts present, and transfers focus with it.
func (t *Tabs) Select(at int) {
	if t == nil {
		return
	}
	t.selection.set(min(max(at, 0), max(len(t.items)-1, 0)))
	t.settle()
}

// Sync applies a caller-written controlled selection to pane focus. An index outside
// the current parts is clamped and written back, so the caller and controller keep
// one valid selection rather than observing different forms of the same state.
//
// Accessors are not observable. Keeping this explicit prevents Draw from performing a
// hidden semantic transition merely because external storage changed.
func (t *Tabs) Sync() {
	if t == nil {
		return
	}
	t.Select(t.selection.get())
}

// Move steps by n panes, wrapping unless told not to.
func (t *Tabs) Move(n int) bool {
	if t == nil || len(t.items) < 2 {
		return false
	}
	next := moveIndex(t.Selected(), n, len(t.items), !t.NoWrap)
	was := t.Selected()
	t.Select(next)
	return t.Selected() != was
}

// Handle offers an event to the selected pane before answering tab movement.
//
// The pane is first because a strip that took keys before its contents would steal an
// arrow from an editor or list inside it. This is the same rule a viewport keeps with
// the content it is showing.
func (t *Tabs) Handle(event input.Event) bool {
	if t == nil {
		return false
	}
	if pane, ok := t.Current(); ok {
		if handler, can := pane.Of.(Interactive); can && handler.Handle(event) {
			return true
		}
	}
	key, ok := event.(input.Key)
	if !ok {
		return false
	}
	_, handled := t.matcher.Handle(t.keys(), key, t.Do)
	return handled
}

// Do offers an action to the selected pane before answering tab movement by name.
// A pane driven from a menu keeps the same priority it has for key events.
func (t *Tabs) Do(action keymap.Action) bool {
	if t == nil {
		return false
	}
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

// Focus takes or releases keyboard ownership and settles it on the selected pane.
func (t *Tabs) Focus(has bool) {
	if t == nil {
		return
	}
	if !has {
		t.matcher.Clear()
	}
	blurred := !has
	if t.blurred == blurred && t.settled {
		return
	}
	t.blurred = blurred
	t.settled = false
	t.settle()
}

// Measure is what the selected pane asks for.
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

// Draw paints the selected pane into the whole frame.
func (t *Tabs) Draw(frame Frame) {
	if pane, ok := t.Current(); ok && pane.Of != nil {
		pane.Of.Draw(frame)
	}
}

// Semantics returns a tab list, its tab parts and the selected panel.
func (t *Tabs) Semantics() SemanticNode {
	root := SemanticNode{Role: RoleTabList}
	if t == nil {
		return root
	}
	if len(t.items) > 0 && !t.blurred {
		root.State |= StateFocused
	}
	selected := t.Selected()
	root.Children = make([]SemanticNode, 0, len(t.items)+1)
	for i, tab := range t.items {
		state := SemanticState(0)
		if i == selected {
			state |= StateSelected
		}
		root.Children = append(root.Children, SemanticNode{
			Role: RoleTab, Label: tab.Title, State: state,
		})
	}
	if selected >= 0 {
		panel := SemanticNode{Role: RoleTabPanel, Label: t.items[selected].Title, State: StateSelected}
		if semantic, ok := t.items[selected].Of.(Semantic); ok {
			panel.Children = []SemanticNode{semantic.Semantics()}
		}
		root.Children = append(root.Children, panel)
	}
	return root
}

func (t *Tabs) settle() {
	wantIndex := t.Selected()
	want := Widget(nil)
	if pane, ok := t.Current(); ok {
		want = pane.Of
	}
	if t.settled && wantIndex == t.holderIndex {
		return
	}
	from := t.holder
	t.holder, t.holderIndex, t.settled = want, wantIndex, true
	if from != nil {
		tell(from, false)
	}
	for i, tab := range t.items {
		if i != wantIndex {
			tell(tab.Of, false)
		}
	}
	tell(want, !t.blurred)
}

func (t *Tabs) keys() *keymap.Map {
	if t.Keys != nil {
		return t.Keys
	}
	return tabsKeys()
}
