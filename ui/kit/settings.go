package kit

import (
	"context"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// Settings dresses a browsable list of application-owned values.
//
// [base.Settings] owns selection, scrolling and action routing. Label and Value
// are projections supplied by the application, so this component can display any
// setting without naming a product schema or storing a second copy of it. The value
// column is fitted to its widest cell and the label receives the remaining width.
// Settings must not be copied after first use: its controller retains this appearance.
type Settings[T any] struct {
	noCopy     noCopy
	appearance appearance
	glyphs     Glyphs
	label      func(T) string
	value      func(T) string
	valueWidth int

	controller *base.Settings[T]
}

// SettingsConfig is the complete construction state of [Settings].
//
// Keys browse the list and EditKeys change the selected value. Keeping both in this
// value makes their different responsibilities explicit without hiding either in a
// positional function argument.
type SettingsConfig[T any] struct {
	Scope      *ThemeScope
	Appearance ThemeOverride
	Theme      Theme
	Glyphs     Glyphs
	// Items are copied into the constructed controller.
	Items []T
	// Label and Value project the two visible columns. Nil produces an empty column.
	// They run during measurement and drawing and must be observationally pure.
	Label func(T) string
	Value func(T) string
	// Change is the event-side mutation boundary: its Value reports whether the
	// input was consumed and its State the application owner's outcome. An error
	// does not erase Applied or Started effects.
	Change func(context.Context, int, T, keymap.Action) (scene.ActionResult[bool], error)
	// Keys browse rows; EditKeys change the selected value. Nil uses the respective defaults.
	Keys     *keymap.Map
	EditKeys *keymap.Map
	Wrap     bool
	// ValueWidth caps the right column. Zero leaves it uncapped.
	ValueWidth int
}

// NewSettings panics for negative ValueWidth or competing Theme and Scope
// palette sources. Its controller owns an item-slice snapshot; item payloads
// retain their domain ownership.
func NewSettings[T any](config SettingsConfig[T]) *Settings[T] {
	look, appearanceErr := newAppearance(config.Theme, config.Scope, config.Appearance)
	if appearanceErr != nil {
		panic(appearanceErr)
	}
	controller := base.NewSettings(base.SettingsConfig[T]{Change: config.Change, EditKeys: config.EditKeys})
	controller.List().SetKeys(config.Keys)
	controller.List().SetWrap(config.Wrap)
	controller.List().SetItems(config.Items)
	settings := &Settings[T]{
		appearance: look, glyphs: config.Glyphs.clone(), controller: controller, label: config.Label,
		value: config.Value,
	}
	settings.SetValueWidth(config.ValueWidth)
	if err := settings.controller.Element().SetPaint(settings.Draw); err != nil {
		panic(err)
	}
	settings.appearance.bind(settings.Element())
	return settings
}

func (s *Settings[T]) Controller() *base.Settings[T] {
	if s == nil {
		return nil
	}
	return s.controller
}

func (s *Settings[T]) HeightForWidth(width int) int {
	if s == nil || s.controller == nil {
		return 0
	}
	return s.controller.HeightForWidth(width)
}

func (s *Settings[T]) Draw(frame scene.Frame) {
	if s == nil || s.controller == nil {
		return
	}
	width, _ := frame.Size()
	table := s.table()
	columns := table.Layout(width)
	s.controller.DrawRows(frame, s.appearance.palette().Look(s.glyphs), func(view grid.View, row int, _ T, selected bool) {
		columns.Cells(view, row, s.appearance.rowStyle(view, selected))
	})
}

func (s *Settings[T]) table() Table {
	return Table{
		Theme: s.appearance.palette(),
		Columns: []Column{
			{Size: layout.Flex(1).AtLeast(1)},
			{Align: layout.End, Size: layout.Measured(0, s.valueWidth)},
		},
		Rows: s.controller.List().Len(),
		Cell: func(row, column int) Cell {
			item, ok := s.controller.List().At(row)
			if !ok {
				return Cell{}
			}
			if column == 0 {
				return LabelCell(Label{Text: project(s.label, item), Ellipsis: s.glyphs.Ellipsis})
			}
			return LabelCell(Label{
				Text: project(s.value, item), Style: s.appearance.palette().Accent,
				Align: layout.End, Ellipsis: s.glyphs.Ellipsis,
			})
		},
	}
}

func project[T any](f func(T) string, item T) string {
	if f == nil {
		return ""
	}
	return f(item)
}

func (s *Settings[T]) Element() scene.Element {
	if s == nil || s.controller == nil {
		return scene.Element{}
	}
	return s.controller.Element()
}

func (s *Settings[T]) Glyphs() Glyphs { return s.glyphs.clone() }

// SetGlyphs panics with scene.ErrBusy outside owner mutation: retained
// presentation must stay stable during observation and frame preparation.
func (s *Settings[T]) SetGlyphs(value Glyphs) { replaceGlyphs(s.Element(), &s.glyphs, value) }

func (s *Settings[T]) Label() func(T) string { return s.label }

// SetLabel panics with scene.ErrBusy outside owner mutation: retained
// presentation must stay stable during observation and frame preparation.
func (s *Settings[T]) SetLabel(value func(T) string) {
	element := s.Element()
	if err := element.CheckMutation(); err != nil {
		panic(err)
	}
	if s.label == nil && value == nil {
		return
	}
	s.label = value
	element.Invalidate()
}

func (s *Settings[T]) Value() func(T) string { return s.value }

// SetValue panics with scene.ErrBusy outside owner mutation: retained
// presentation must stay stable during observation and frame preparation.
func (s *Settings[T]) SetValue(value func(T) string) {
	element := s.Element()
	if err := element.CheckMutation(); err != nil {
		panic(err)
	}
	if s.value == nil && value == nil {
		return
	}
	s.value = value
	element.Invalidate()
}

func (s *Settings[T]) ValueWidth() int { return s.valueWidth }

// SetValueWidth panics for a negative width or with scene.ErrBusy outside
// owner mutation, before the retained presentation can change.
func (s *Settings[T]) SetValueWidth(value int) {
	element := s.Element()
	if err := element.CheckMutation(); err != nil {
		panic(err)
	}
	if value < 0 {
		panic("kit: ValueWidth must be non-negative")
	}
	if s.valueWidth == value {
		return
	}
	s.valueWidth = value
	element.Invalidate()
}
