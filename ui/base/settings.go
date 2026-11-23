package base

import (
	"context"
	"errors"
	"strconv"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// Settings is a browsable list whose selected item answers value actions.
//
// Its rows own selection, scrolling, navigation and row drawing, exactly as a
// [List]'s do. Change owns the one behaviour a plain list does not have: left,
// right or activation changes the selected value. What an item is and how a change is stored
// remain the caller's domain; this controller only routes an action to it.
//
// The zero value is an empty read-only list. A Settings value must not be copied after
// first use: its list navigation and value-action matcher are one mutable owner.
type Settings[T any] struct {
	rows[T]
	noCopy noCopy

	// Value reports input acceptance; State reports the domain owner's outcome.
	// Acceptance alone cannot tell semantic callers whether a value changed.
	changeValue func(context.Context, int, T, keymap.Action) (scene.ActionResult[bool], error)
	// EditKeys binds value actions. Nil reads through [DefaultSettingsKeys]. Keys
	// independently controls navigation, so rebinding a value never replaces how the
	// list is browsed.
	editBindings keyBinding

	editMatcher keymap.Matcher
	editVersion *byte
	feedback    requestFeedback
}

// SettingsConfig is the complete construction state of [Settings]. Keys browse the
// list and EditKeys change the selected value, so rebinding one never replaces the
// other.
type SettingsConfig[T any] struct {
	// Items are copied; item payloads keep their domain ownership.
	Items []T
	// Accepted effects and failures are independent; Applied or Started survives
	// an accompanying error. Value alone only reports input consumption.
	Change func(context.Context, int, T, keymap.Action) (scene.ActionResult[bool], error)
	// Keys of nil uses [DefaultListKeys]; EditKeys of nil uses [DefaultSettingsKeys].
	Keys     *keymap.Map
	EditKeys *keymap.Map
	Wrap     bool
}

func NewSettings[T any](config SettingsConfig[T]) *Settings[T] {
	s := &Settings[T]{changeValue: config.Change, editBindings: keyBinding{current: config.EditKeys}}
	s.configure(ListConfig[T]{Keys: config.Keys, Wrap: config.Wrap})
	s.SetItems(config.Items)
	return s
}

func (s *Settings[T]) focusChanged(has bool) {
	change := func() {
		s.rows.focusChanged(has)
	}
	if has {
		change()
	} else {
		s.editMatcher.Reset(change)
	}
}

func (s *Settings[T]) Handle(event input.Event) bool {
	requireMutable(s.element)
	if s.rows.Handle(event) {
		return true
	}
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	version := s.version
	if s.editVersion != version {
		if !s.editMatcher.Reset(func() { s.editVersion = version }) {
			return true
		}
	}
	if s.version != version {
		return true
	}
	_, handled := s.editMatcher.Handle(s.editBindings.resolve(settingsKeys), key, keyboardReceipt(&s.element), func(action keymap.Action) bool {
		// A delayed value action belongs to the row where its sequence began.
		return s.version == version && s.Do(action)
	})
	return handled
}

func (s *Settings[T]) Do(action keymap.Action) bool {
	requireMutable(s.element)
	if s.rows.Do(action) {
		return true
	}
	if s.changeValue == nil || s.Selected() < 0 {
		return false
	}
	result, _ := s.request(context.Background(), s.Selected(), action, true)
	return result.Value
}

// Request addresses the current item order. It returns the domain's accepted
// effects and error without replacing physical-input feedback. For an observed
// revision-qualified address, use the element's semantic collection actions.
// It panics during presentation or observation.
func (s *Settings[T]) Request(ctx context.Context, index int, action keymap.Action) (scene.ActionResult[bool], error) {
	return s.request(ctx, index, action, false)
}

func (s *Settings[T]) request(ctx context.Context, index int, action keymap.Action, recordError bool) (scene.ActionResult[bool], error) {
	requireMutable(s.element)
	var turn *byte
	if recordError {
		turn = s.feedback.begin()
	}
	result, err := s.applyRequest(ctx, index, action)
	changed := result.State != scene.Unchanged
	if recordError {
		changed = s.feedback.finish(turn, err) || changed
	}
	if changed {
		s.element.Invalidate()
	}
	return result, err
}

func (s *Settings[T]) applyRequest(ctx context.Context, index int, action keymap.Action) (scene.ActionResult[bool], error) {
	if err := ctx.Err(); err != nil {
		return scene.ActionResult[bool]{}, context.Cause(ctx)
	}
	item, ok := s.At(index)
	if !ok || s.changeValue == nil {
		return scene.ActionResult[bool]{}, scene.ErrUnavailable
	}
	result, err := s.changeValue(ctx, index, item, action)
	if stateErr := result.State.Validate(); stateErr != nil {
		return scene.ActionResult[bool]{}, errors.Join(err, stateErr)
	}
	return result, err
}

// ChangeError belongs to the latest physical value request. Request and semantic
// actions return errors to their callers without changing this feedback.
func (s *Settings[T]) ChangeError() error { return s.feedback.err }

func (s *Settings[T]) HeightForWidth(width int) int {
	return layout.Sum(s.rows.HeightForWidth(width), feedbackRows(s.ChangeError()))
}

func (s *Settings[T]) Draw(frame scene.Frame) {
	s.DrawRows(frame, Look{}, s.rowPainter)
}

// DrawRows reserves the same feedback row as Draw before staging list geometry.
// It panics unless the settings owner is mounted in the frame's scene outside
// read-only projection. The painter must be observationally pure.
func (s *Settings[T]) DrawRows(frame scene.Frame, look Look, draw func(grid.View, int, T, bool)) {
	requireProjection(s.element, frame)
	s.rows.DrawRows(feedbackFrame(frame, s.ChangeError(), look), draw)
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
			if availability := s.indexAvailability(address); availability.Disabled() {
				return availability
			}
			if s.changeValue == nil {
				return scene.Unavailable("setting is read-only")
			}
			return scene.Availability{}
		},
		Run: func(ctx context.Context, address CollectionIndex) (scene.ActionResult[bool], error) {
			return s.Request(ctx, address.Index, action)
		},
	}).Binding()
}

func (s *Settings[T]) Change() func(context.Context, int, T, keymap.Action) (scene.ActionResult[bool], error) {
	return s.changeValue
}

func (s *Settings[T]) SetChange(value func(context.Context, int, T, keymap.Action) (scene.ActionResult[bool], error)) {
	requireMutable(s.element)
	s.editMatcher.Reset(func() {
		s.changeValue = value
		s.element.Invalidate()
	})
}

func (s *Settings[T]) EditKeys() *keymap.Map { return s.editBindings.current }

func (s *Settings[T]) SetEditKeys(keys *keymap.Map) {
	s.editBindings.set(s.element, &s.editMatcher, keys)
}

func (s *Settings[T]) Element() scene.Element {
	if s.element.IsZero() {
		s.element = scene.NewElement(scene.ElementConfig{
			Measure: layout.MeasureRows(s.HeightForWidth), Paint: s.Draw,
			Handle: func(e scene.Event) scene.EventResult {
				return partEvent(e, s.Handle, &s.geometry, &s.pointer.geometry)
			}, Focusable: func() bool { return true }, FocusChanged: s.focusChanged,
			Semantics: func() scene.Semantic {
				return withFeedback(scene.Semantic{Role: scene.RoleList, ContentRevision: s.contentRevision, Value: strconv.Itoa(s.Selected())}, s.ChangeError())
			}, Actions: append(collectionActions(&s.rows, unchangedItem[T]), s.valueActions()...),
		})
	}
	s.adopt(s.element)
	s.editBindings.attach(s.element, &s.editMatcher)
	return s.element
}
