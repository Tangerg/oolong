package base

import (
	"context"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Tab identifies one logical pane. Key preserves trigger identity across reordering; empty keys use positional identity.
type Tab struct {
	Key     string
	Title   string
	Content scene.Element
}
type tabPart struct {
	matcher keymap.Matcher
	tab     Tab
	element scene.Element
}

// TabsConfig supplies the initial pane relation and navigation policy. PaintTab is a pure appearance slot on each owned trigger.
type TabsConfig struct {
	Items     []Tab
	Initial   int
	Keys      *keymap.Map
	NoWrap    bool
	Gap       int
	StripRows int
	PaintTab  func(scene.Frame, Tab, bool)
}

// Tabs owns the selected logical part. The strip and panel are scene groups;
// their children receive focus and input from the one scene router.
// Tabs must not be copied after first use; its collaborators share one live identity.
type Tabs struct {
	noCopy              noCopy
	parts               []*tabPart
	selected            int
	Keys                *keymap.Map
	NoWrap              bool
	gap, stripRows      int
	paintTab            func(scene.Frame, Tab, bool)
	strip, panel, group *scene.Group
	matcher             keymap.Matcher
}

// NewTabs constructs one selection owner. It panics for duplicate nonempty keys or an invalid initially selected content relation.
func NewTabs(config TabsConfig) *Tabs {
	t := &Tabs{selected: config.Initial, Keys: config.Keys, NoWrap: config.NoWrap, gap: config.Gap, stripRows: config.StripRows, paintTab: config.PaintTab}
	t.Set(config.Items...)
	return t
}

func (t *Tabs) ensure() {
	if t.group != nil {
		return
	}
	t.strip = scene.NewGroup(scene.GroupConfig{Axis: layout.Across, Gap: t.gap, Element: scene.ElementConfig{Semantics: func() scene.Semantic { return scene.Semantic{Role: scene.RoleTabList} }}})
	t.panel = scene.NewGroup(scene.GroupConfig{Overlay: true, Element: scene.ElementConfig{Semantics: func() scene.Semantic {
		tab, _ := t.Current()
		return scene.Semantic{Role: scene.RoleTabPanel, Name: tab.Title}
	}}})
	t.group = scene.NewGroup(scene.GroupConfig{Axis: layout.Down, Element: scene.ElementConfig{Handle: t.handleEvent}, Items: []scene.Item{
		{Element: t.strip.Element(), Size: layout.Fixed(max(1, t.stripRows))},
		{Element: t.panel.Element(), Size: layout.Flex(1)},
	}})
}

// Element is the scene composition of this controller's strip and selected panel.
func (t *Tabs) Element() scene.Element { t.ensure(); return t.group.Element() }

// Strip is the owned trigger group; it shares the same selection state as Panel.
func (t *Tabs) Strip() scene.Element { t.ensure(); return t.strip.Element() }

// Panel is the group containing only the selected pane's content.
func (t *Tabs) Panel() scene.Element { t.ensure(); return t.panel.Element() }

// Set reconciles stable trigger identities and preserves the selected key when possible. It panics for duplicate keys, an invalid selected content relation, or mutation during presentation or observation.
func (t *Tabs) Set(items ...Tab) {
	t.ensure()
	requireMutable(t.Element())
	keys := make(map[string]bool, len(items))
	for _, item := range items {
		if item.Key != "" && keys[item.Key] {
			panic("base: duplicate tab key")
		}
		keys[item.Key] = true
	}
	parts := t.reconcileParts(items)
	selected := t.selectionFor(items)
	children := make([]scene.Item, len(parts))
	for i, part := range parts {
		children[i] = scene.Item{Key: items[i].Key, Size: layout.Measured(0, 0), Element: part.element}
	}
	var panel []scene.Item
	if len(items) > 0 && !items[selected].Content.IsZero() {
		panel = []scene.Item{{Element: items[selected].Content, Size: layout.Flex(1)}}
	}
	if err := t.strip.CheckSet(children...); err != nil {
		panic(err)
	}
	if err := t.panel.CheckSet(panel...); err != nil {
		panic(err)
	}
	focused := t.Panel().FocusedWithin()
	t.matcher.Reset(func() {
		for i, part := range parts {
			item := items[i]
			item.Key, item.Title = strings.Clone(item.Key), strings.Clone(item.Title)
			part.tab = item
		}
		t.parts, t.selected = parts, selected
		if err := t.strip.Set(children...); err != nil {
			panic(err)
		}
		if err := t.panel.Set(panel...); err != nil {
			panic(err)
		}
		if focused {
			_ = t.Panel().RequestFocusFirst()
		}
	})
}

// Items returns detached tab descriptors in their current order.
func (t *Tabs) Items() []Tab {
	items := make([]Tab, len(t.parts))
	for i, part := range t.parts {
		items[i] = part.tab
	}
	return items
}

// Len is the number of logical tabs.
func (t *Tabs) Len() int { return len(t.parts) }

// At returns the tab at index; invalid indices return false.
func (t *Tabs) At(index int) (Tab, bool) {
	if index < 0 || index >= len(t.parts) {
		return Tab{}, false
	}
	return t.parts[index].tab, true
}

// Selected is the selected logical index, or -1 when no tabs exist.
func (t *Tabs) Selected() int {
	if len(t.parts) == 0 {
		return -1
	}
	return t.selected
}

// Current returns the selected descriptor, if any.
func (t *Tabs) Current() (Tab, bool) { return t.At(t.Selected()) }

func (t *Tabs) selectedPart() *tabPart {
	if t.Selected() < 0 {
		return nil
	}
	return t.parts[t.selected]
}

// Select clamps the requested index and mounts that pane. It panics if the new content belongs to another scene relation, or during presentation or observation.
func (t *Tabs) Select(index int) {
	t.ensure()
	requireMutable(t.Element())
	next := min(max(index, 0), max(len(t.parts)-1, 0))
	if next == t.selected {
		return
	}
	var panel []scene.Item
	if len(t.parts) > 0 && !t.parts[next].tab.Content.IsZero() {
		panel = []scene.Item{{Element: t.parts[next].tab.Content, Size: layout.Flex(1)}}
	}
	if err := t.panel.CheckSet(panel...); err != nil {
		panic(err)
	}
	focused := t.Panel().FocusedWithin()
	t.selected = next
	if err := t.panel.Set(panel...); err != nil {
		panic(err)
	}
	if focused {
		_ = t.Panel().RequestFocusFirst()
	}
	t.Element().Invalidate()
}

func (t *Tabs) selectPart(part *tabPart) {
	at := slices.Index(t.parts, part)
	if at >= 0 {
		t.Select(at)
	}
}

// Move advances selection by delta using the configured wrap policy.
func (t *Tabs) Move(delta int) bool {
	if len(t.parts) < 2 {
		return false
	}
	before := t.Selected()
	t.Select(moveIndex(before, delta, len(t.parts), !t.NoWrap))
	return before != t.Selected()
}

// Do applies one named tab-navigation action through Select.
func (t *Tabs) Do(action keymap.Action) bool {
	switch action {
	case NextTab:
		t.Move(1)
	case PrevTab:
		t.Move(-1)
	default:
		return false
	}
	return true
}

func (t *Tabs) handleEvent(event scene.Event) scene.EventResult {
	key, ok := event.Input.(input.Key)
	if !ok {
		return scene.EventResult{}
	}
	_, handled := t.matcher.Handle(t.keys(), key, func(action keymap.Action) bool {
		if action == FocusNext {
			return event.FocusNext(false)
		}
		if action == FocusPrev {
			return event.FocusNext(true)
		}
		return t.Do(action)
	})
	return scene.EventResult{Consumed: handled}
}

// HeightForWidth measures the trigger strip and selected pane without changing selection.
func (t *Tabs) HeightForWidth(width int) int {
	rows := max(1, t.stripRows)
	if tab, ok := t.Current(); ok {
		return layout.Sum(rows, tab.Content.Measure(layout.Down, width))
	}
	return rows
}

func (t *Tabs) keys() *keymap.Map {
	if t.Keys != nil {
		return t.Keys
	}
	return tabsKeys()
}

func (t *Tabs) reconcileParts(items []Tab) []*tabPart {
	parts := make([]*tabPart, len(items))
	for i, item := range items {
		old := t.partIndex(item.Key, i)
		var part *tabPart
		if old >= 0 && old < len(t.parts) && t.parts[old].tab.Key == item.Key {
			part = t.parts[old]
		} else {
			part = t.newPart(item.Key)
		}

		parts[i] = part
	}

	return parts
}

func (t *Tabs) newPart(key string) *tabPart {
	part := &tabPart{}
	choose := scene.NewAction(scene.ActionConfig[struct{}, int]{ID: scene.ActionSelect, Label: "Select tab", Run: func(context.Context, struct{}) (scene.ActionResult[int], error) {
		before := t.Selected()
		t.selectPart(part)
		state := scene.Unchanged
		if before != t.Selected() {
			state = scene.Applied
		}
		return scene.ActionResult[int]{State: state, Value: t.Selected()}, nil
	}})
	part.element = scene.NewElement(scene.ElementConfig{
		Key: key, Part: "tab", Focusable: func() bool { return true },
		FocusChanged: func(has bool) {
			if !has {
				part.matcher.Clear()
			}
		},
		Measure: func(axis layout.Axis, _ int) int {
			if axis == layout.Down {
				return max(1, t.stripRows)
			}
			return text.Width(part.tab.Title)
		},
		Paint: func(frame scene.Frame) {
			if t.paintTab != nil {
				t.paintTab(frame, part.tab, t.selectedPart() == part)
			} else {
				frame.Text(0, 0, part.tab.Title, Look{}.Text)
			}
		},
		Handle: func(event scene.Event) scene.EventResult {
			if mouse, ok := event.Input.(input.Mouse); ok && mouse.Action == input.MouseDown && mouse.Button == input.ButtonLeft {
				t.selectPart(part)
				return scene.EventResult{Consumed: true, Focus: true}
			}
			key, ok := event.Input.(input.Key)
			if !ok {
				return scene.EventResult{}
			}
			_, handled := part.matcher.Handle(activationKeys(), key, func(action keymap.Action) bool {
				if action != Activate {
					return false
				}
				t.selectPart(part)
				return true
			})
			return scene.EventResult{Consumed: handled}
		}, Semantics: func() scene.Semantic {
			state := scene.SemanticState(0)
			if t.selectedPart() == part {
				state = scene.StateSelected
			}
			return scene.Semantic{Role: scene.RoleTab, Name: part.tab.Title, State: state}
		}, Actions: []scene.ActionBinding{choose.Binding()},
	})

	return part
}

func (t *Tabs) partIndex(key string, fallback int) int {
	if key == "" {
		return fallback
	}
	for index, part := range t.parts {
		if part.tab.Key == key {
			return index
		}
	}
	return -1
}

func (t *Tabs) selectionFor(items []Tab) int {
	selected := min(max(t.selected, 0), max(len(items)-1, 0))
	previous, ok := t.Current()
	if !ok || previous.Key == "" {
		return selected
	}
	for index, item := range items {
		if item.Key == previous.Key {
			return index
		}
	}
	return selected
}
