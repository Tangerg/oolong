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

// NewText constructs one single-line text owner from initial configuration.
func NewText(config TextConfig) *Text {
	t := &Text{
		Label: strings.Clone(config.Label), Check: config.Check, Placeholder: strings.Clone(config.Placeholder), Keys: config.Keys,
		Clipboard: config.Clipboard, Gutter: config.Gutter, CursorStyle: config.CursorStyle,
	}
	t.key = strings.Clone(config.Key)
	t.configureEditor()
	t.editor.SetText(config.Initial)
	t.editor.ForgetHistory()
	return t
}

// SelectConfig supplies available options, the initial value and pure comparison, validation and row projections.
type SelectConfig[T any] struct {
	Key     string
	Initial T
	Options []Option[T]
	Label   string
	Same    func(T, T) bool
	Check   func(T) error
	Rows    int
	Keys    *keymap.Map
	Row     func(grid.View, Option[T], bool, bool, Look)
}

// NewSelect owns the option collection and its selected index. An unavailable Initial leaves the first available option selected.
func NewSelect[T any](config SelectConfig[T]) *Select[T] {
	s := &Select[T]{Label: strings.Clone(config.Label), Same: config.Same, Check: config.Check, Rows: config.Rows, Keys: config.Keys, Row: config.Row}
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
	Same    func(T, T) bool
	Check   func([]T) error
	Rows    int
	Keys    *keymap.Map
	Row     func(grid.View, Option[T], bool, bool, Look)
}

// NewMultiSelect constructs one canonical chosen-set owner; returned slices are detached observations.
func NewMultiSelect[T any](config MultiSelectConfig[T]) *MultiSelect[T] {
	m := &MultiSelect[T]{Label: strings.Clone(config.Label), Same: config.Same, Check: config.Check, Rows: config.Rows, Keys: config.Keys, Row: config.Row}
	m.key = strings.Clone(config.Key)
	m.SetOptions(config.Options)
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
	c := &Confirm{answer: newOwnedValue(config.Initial), Label: strings.Clone(config.Label), Yes: strings.Clone(config.Yes), No: strings.Clone(config.No), Check: config.Check, Keys: config.Keys, control: config.Control}
	c.key = strings.Clone(config.Key)
	if c.control != nil {
		ref := weak.Make(c)
		c.unsubscribe = c.control.Subscribe(func() {
			if owner := ref.Value(); owner != nil && !owner.closed {
				requireMutable(owner.element)
				owner.element.Invalidate()
			}
		})
		if c.unsubscribe == nil {
			panic("base: ConfirmControl.Subscribe returned nil")
		}
	}
	return c
}
