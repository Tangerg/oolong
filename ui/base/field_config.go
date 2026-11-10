package base

import (
	"strings"
	"weak"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/keymap"
)

// TextConfig seeds one answer. Later changes go through the returned owner.
type TextConfig struct {
	Key         string
	Initial     string
	Label       string
	Check       func(string) error
	Placeholder string
	Keys        *keymap.Map
	Clipboard   Clipboard
	Gutter      RowGutter
	CursorStyle grid.CursorStyle
}

// NewText panics for an unknown cursor shape; one-line policy belongs to its edit session.
func NewText(config TextConfig) *Text {
	t := &Text{label: strings.Clone(config.Label), check: config.Check}
	t.editor.placeholder = strings.Clone(config.Placeholder)
	t.editor.keys.current = config.Keys
	t.editor.clipboard, t.editor.gutter = config.Clipboard, config.Gutter
	if err := config.CursorStyle.Shape.Validate(); err != nil {
		panic(err)
	}
	t.editor.cursorStyle = config.CursorStyle
	t.key = strings.Clone(config.Key)
	t.line().SetText(config.Initial)
	t.editor.ForgetHistory()
	return t
}

// SelectConfig fixes identity matching for the field's lifetime. Validation and
// row callbacks have distinct roles: checks may request changes, rows must be pure.
type SelectConfig[T any] struct {
	Key     string
	Initial T
	Options []Option[T]
	Label   string
	// Same is required for non-empty options and must remain a pure, stable
	// identity relation. It does not compare the complete payload for validation.
	Same  func(T, T) bool
	Check func(T) error
	Rows  int
	Keys  *keymap.Map
	Row   func(grid.View, Option[T], bool, bool, Look)
}

// NewSelect panics for negative Rows or non-empty Options without Same.
// An unavailable Initial leaves the first available option selected.
func NewSelect[T any](config SelectConfig[T]) *Select[T] {
	requireNonNegative("choice Rows", config.Rows)
	s := &Select[T]{check: config.Check}
	s.initialize(config.Label, config.Same, config.Rows, config.Keys, config.Row)
	s.key = strings.Clone(config.Key)
	s.SetOptions(config.Options)
	s.Choose(config.Initial)
	return s
}

// MultiSelectConfig supplies available options and an initial chosen set. Unknown values and duplicates are removed at the owner boundary.
type MultiSelectConfig[T any] struct {
	Key     string
	Initial []T
	Options []Option[T]
	Label   string
	// Same has the same lifetime and purity contract as SelectConfig.Same.
	Same  func(T, T) bool
	Check func([]T) error
	Rows  int
	// Limit is how many choices may be taken at once; zero allows every option.
	// It applies before Initial, which must respect it.
	Limit int
	Keys  *keymap.Map
	Row   func(grid.View, Option[T], bool, bool, Look)
}

// NewMultiSelect panics for negative Rows or Limit, an Initial set larger than
// Limit, or non-empty Options without Same. Its chosen set is canonical; returned
// slices are detached observations.
func NewMultiSelect[T any](config MultiSelectConfig[T]) *MultiSelect[T] {
	requireNonNegative("choice Rows", config.Rows)
	requireNonNegative("multi-select Limit", config.Limit)
	m := &MultiSelect[T]{check: config.Check, limit: config.Limit}
	m.initialize(config.Label, config.Same, config.Rows, config.Keys, config.Row)
	m.key = strings.Clone(config.Key)
	m.SetOptions(config.Options)
	if taken, _ := m.indices(m.list.items, config.Initial); !m.admits(len(taken)) {
		panic("base: multi-select Initial exceeds its Limit")
	}
	m.SetTaken(config.Initial)
	return m
}

// ConfirmConfig chooses an owned Initial or a live domain Control. Keys and pointer input use the same version-checked request path.
type ConfirmConfig struct {
	Key     string
	Control ConfirmControl
	Initial bool
	Label   string
	Yes, No string
	Check   func(bool) error
	Keys    *keymap.Map
}

// NewConfirm constructs one owned or domain-controlled answer. Initial must be
// false when Control is present. It panics for competing initial ownership or a
// subscription that returns no cancellation function.
func NewConfirm(config ConfirmConfig) *Confirm {
	if config.Control != nil && config.Initial {
		panic("base: Confirm Initial and Control are mutually exclusive")
	}
	control := config.Control
	if control == nil {
		control = &localConfirm{answer: newOwnedValue(config.Initial)}
	}
	c := &Confirm{label: strings.Clone(config.Label), yes: strings.Clone(config.Yes), no: strings.Clone(config.No), check: config.Check, keys: keyBinding{current: config.Keys}, control: control}
	c.key = strings.Clone(config.Key)
	if config.Control != nil {
		ref := weak.Make(c)
		c.unsubscribe = c.control.Subscribe(func() {
			if owner := ref.Value(); owner != nil && !owner.closed {
				owner.invalidate()
			}
		})
		if c.unsubscribe == nil {
			panic("base: ConfirmControl.Subscribe returned nil")
		}
	}
	return c
}
