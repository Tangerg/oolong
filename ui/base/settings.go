package base

import (
	"context"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// Settings is a browsable list whose selected item answers value actions.
//
// The composed [List] owns selection, scrolling, navigation and row drawing.
// Change owns the one behaviour a plain list does not have: left, right or
// activation changes the selected value. What an item is and how a change is stored
// remain the caller's domain; this controller only routes an action to it.
//
// The zero value is an empty read-only list. A Settings value must not be copied after
// first use: its list navigation and value-action matcher are one mutable owner.
type Settings[T any] struct {
	noCopy noCopy

	list List[T]
	// Value reports input acceptance; State reports the domain owner's outcome.
	// Acceptance alone cannot tell semantic callers whether a value changed.
	Change func(index int, item T, action keymap.Action) scene.ActionResult[bool]
	// EditKeys binds value actions. Nil reads through [DefaultSettingsKeys]. List.Keys
	// independently controls navigation, so rebinding a value never replaces how the
	// list is browsed.
	EditKeys *keymap.Map

	matcher      keymap.Matcher
	editRevision uint64
}

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

func (s *Settings[T]) Handle(event input.Event) bool {
	requireMutable(s.list.element)
	if s.list.Handle(event) {
		return true
	}
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	revision := s.list.revision
	if s.editRevision != revision {
		if !s.matcher.Reset(func() { s.editRevision = revision }) {
			return true
		}
	}
	if s.list.revision != revision {
		return true
	}
	_, handled := s.matcher.Handle(s.editKeys(), key, func(action keymap.Action) bool {
		// A delayed value action belongs to the row where its sequence began.
		return s.list.revision == revision && s.Do(action)
	})
	return handled
}

// Do navigates the list or applies a value action to its selected item.
func (s *Settings[T]) Do(action keymap.Action) bool {
	requireMutable(s.list.element)
	if s.list.Do(action) {
		return true
	}
	return s.change(s.list.Selected(), action).Value
}

func (s *Settings[T]) change(index int, action keymap.Action) scene.ActionResult[bool] {
	if item, ok := s.list.At(index); ok && s.Change != nil {
		result := s.Change(index, item, action)
		if result.State != scene.Unchanged {
			s.list.element.Invalidate()
		}
		return result
	}
	return scene.ActionResult[bool]{}
}

func (s *Settings[T]) valueActions() []scene.ActionBinding {
	return []scene.ActionBinding{
		s.valueAction(scene.ActionIncrement, "Increase value", Increase),
		s.valueAction(scene.ActionDecrement, "Decrease value", Decrease),
		s.valueAction(scene.ActionActivate, "Activate value", Activate),
	}
}

func (s *Settings[T]) valueAction(id scene.ActionID, label string, action keymap.Action) scene.ActionBinding {
	return scene.NewAction(scene.ActionConfig[CollectionIndex, bool]{
		ID: id, Label: label,
		Availability: func(address CollectionIndex) scene.Availability {
			if availability := s.list.indexAvailability(address); availability.Disabled {
				return availability
			}
			if s.Change == nil {
				return scene.Availability{Disabled: true, Reason: "setting is read-only"}
			}
			return scene.Availability{}
		},
		Run: func(_ context.Context, address CollectionIndex) (scene.ActionResult[bool], error) {
			return s.change(address.Index, action), nil
		},
	}).Binding()
}

func (s *Settings[T]) editKeys() *keymap.Map {
	if s.EditKeys != nil {
		return s.EditKeys
	}
	return settingsKeys()
}

// List is the navigation collaborator; item values remain application-owned.
func (s *Settings[T]) List() *List[T] {
	s.Element()
	return &s.list
}
