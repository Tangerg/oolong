package headless

import (
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/keymap"
)

// TextConfig supplies a Text's initial binding and presentation settings. Nil Value
// keeps its answer local. Construction does not write the accessor.
type TextConfig struct {
	Value       Accessor[string]
	Label       string
	Check       func(string) error
	Placeholder string
	Keys        *keymap.Map
	Clipboard   Clipboard
	Gutter      RowGutter
	CursorStyle grid.CursorStyle
}

// NewText constructs a field from config. Replace its binding through SetBinding.
func NewText(config TextConfig) *Text {
	return &Text{
		value: config.Value, owner: new(byte), Label: config.Label,
		Check: config.Check, Placeholder: config.Placeholder, Keys: config.Keys,
		Clipboard: config.Clipboard, Gutter: config.Gutter, CursorStyle: config.CursorStyle,
	}
}

// SelectConfig supplies a Select's initial binding and settings. Same is required
// before non-empty options are installed; nil Value keeps the choice local.
type SelectConfig[T any] struct {
	Value Accessor[T]
	Label string
	Same  func(T, T) bool
	Check func(T) error
	Rows  int
	Keys  *keymap.Map
	Row   func(grid.View, Option[T], bool, bool, Look)
}

// NewSelect constructs a choice field without writing its accessor. SetOptions
// installs choices and SetBinding replaces their value owner.
func NewSelect[T any](config SelectConfig[T]) *Select[T] {
	return &Select[T]{
		value: config.Value, Label: config.Label, Same: config.Same,
		Check: config.Check, Rows: config.Rows, Keys: config.Keys, Row: config.Row,
	}
}

// MultiSelectConfig supplies a MultiSelect's initial binding and settings. Same is
// required before non-empty options are installed; nil Value keeps choices local.
type MultiSelectConfig[T any] struct {
	Value Accessor[[]T]
	Label string
	Same  func(T, T) bool
	Check func([]T) error
	Rows  int
	Keys  *keymap.Map
	Row   func(grid.View, Option[T], bool, bool, Look)
}

// NewMultiSelect constructs a choice field without writing its accessor. SetOptions
// installs choices and SetBinding replaces their value owner.
func NewMultiSelect[T any](config MultiSelectConfig[T]) *MultiSelect[T] {
	return &MultiSelect[T]{
		value: config.Value, owner: new(byte), Label: config.Label,
		Same: config.Same, Check: config.Check, Rows: config.Rows, Keys: config.Keys, Row: config.Row,
	}
}

// ConfirmConfig supplies a Confirm's initial binding and settings. Nil Value keeps
// the answer local; empty Yes and No use the field's default words.
type ConfirmConfig struct {
	Value   Accessor[bool]
	Label   string
	Yes, No string
	Check   func(bool) error
	Keys    *keymap.Map
}

// NewConfirm constructs a confirmation without writing its accessor. Replace the
// value owner through SetBinding.
func NewConfirm(config ConfirmConfig) *Confirm {
	return &Confirm{
		value: config.Value, Label: config.Label, Yes: config.Yes,
		No: config.No, Check: config.Check, Keys: config.Keys,
	}
}
