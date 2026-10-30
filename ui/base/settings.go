package base

import (
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// Settings is a browsable list whose selected item answers value actions.
//
// The embedded [List] owns selection, scrolling, navigation and row drawing.
// Change owns the one behaviour a plain list does not have: left, right or
// activation changes the selected value. What an item is and how a change is stored
// remain the caller's domain; this controller only routes an action to it.
//
// The zero value is an empty read-only list. A Settings value must not be copied after
// first use: its list navigation and value-action matcher are one mutable owner.
type Settings[T any] struct {
	element scene.Element
	noCopy  noCopy

	list List[T]
	// Change applies an action to the selected item. It reports whether the item
	// accepted it. Nil makes the list read-only.
	Change func(index int, item T, action keymap.Action) bool
	// EditKeys binds value actions. Nil reads through [DefaultSettingsKeys]. List.Keys
	// independently controls navigation, so rebinding a value never replaces how the
	// list is browsed.
	EditKeys *keymap.Map

	matcher   keymap.Matcher
	editItems *byte
	editIndex int
}

// Focus takes or releases the keyboard with the embedded list. Releasing it also
// cancels a partial value binding owned by the settings controller.
func (s *Settings[T]) focusChanged(has bool) {
	change := func() {
		s.list.focusChanged(has)
	}
	if has {
		change()
	} else {
		s.matcher.Reset(change)
	}
}

// Handle first lets list navigation and pointer selection act, then offers a key to
// the selected value.
func (s *Settings[T]) Handle(event input.Event) bool {
	requireMutable(s.element)
	if s.list.Handle(event) {
		return true
	}
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	items, index := s.list.itemsID, s.list.Selected()
	if s.editItems != items || s.editIndex != index {
		if !s.matcher.Reset(func() { s.editItems, s.editIndex = items, index }) {
			return true
		}
	}
	if s.list.itemsID != items || s.list.Selected() != index {
		return true
	}
	_, handled := s.matcher.Handle(s.editKeys(), key, func(action keymap.Action) bool {
		// A delayed value action belongs to the row where its sequence began.
		return s.list.itemsID == items && s.list.Selected() == index && s.Do(action)
	})
	return handled
}

// Do navigates the list or applies a value action to its selected item.
func (s *Settings[T]) Do(action keymap.Action) bool {
	requireMutable(s.element)
	if s.list.Do(action) {
		return true
	}
	if s.Change == nil {
		return false
	}
	index := s.list.Selected()
	item, ok := s.list.At(index)
	return ok && s.Change(index, item, action)
}

func (s *Settings[T]) editKeys() *keymap.Map {
	if s.EditKeys != nil {
		return s.EditKeys
	}
	return settingsKeys()
}

// List is the navigation collaborator; item values remain application-owned.
func (s *Settings[T]) List() *List[T] { return &s.list }
