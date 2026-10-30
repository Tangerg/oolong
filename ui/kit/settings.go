package kit

import (
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
type Settings[T any] struct {
	appearance appearance
	Glyphs     Glyphs
	// Label and Value are the two columns shown for an item. Nil returns an empty
	// column. Both run during measurement and drawing and must be observationally
	// pure; Change is the event-side mutation boundary.
	Label func(T) string
	Value func(T) string
	// ValueWidth caps the fitted value column. Zero leaves it uncapped.
	ValueWidth int

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
	// Change applies a value action. Nil makes values read-only.
	Change func(index int, item T, action keymap.Action) bool
	// Keys browse rows; EditKeys change the selected value. Nil uses the respective defaults.
	Keys     *keymap.Map
	EditKeys *keymap.Map
	Wrap     bool
	// ValueWidth caps the right column. Zero leaves it uncapped.
	ValueWidth int
}

// NewSettings builds a settings list around one application-owned item slice.
// It panics when Theme and Scope both declare palette sources, or when its base configuration is invalid.
func NewSettings[T any](config SettingsConfig[T]) *Settings[T] {
	look, appearanceErr := newAppearance(config.Theme, config.Scope, config.Appearance)
	if appearanceErr != nil {
		panic(appearanceErr)
	}
	controller := &base.Settings[T]{Change: config.Change, EditKeys: config.EditKeys}
	controller.List().Keys = config.Keys
	controller.List().Wrap = config.Wrap
	controller.List().SetItems(config.Items)
	settings := &Settings[T]{
		appearance: look, Glyphs: config.Glyphs, controller: controller, Label: config.Label,
		Value: config.Value, ValueWidth: config.ValueWidth,
	}
	if err := settings.controller.Element().SetPaint(settings.Draw); err != nil {
		panic(err)
	}
	settings.appearance.bind(settings.Element())
	return settings
}

// Controller returns the base settings list that owns selection and actions.
func (s *Settings[T]) Controller() *base.Settings[T] {
	if s == nil {
		return nil
	}
	return s.controller
}

// HeightForWidth is one row per setting.
func (s *Settings[T]) HeightForWidth(int) int {
	if s == nil || s.controller == nil {
		return 0
	}
	return s.controller.List().Len()
}

// Draw paints the visible rows with one shared table layout.
func (s *Settings[T]) Draw(frame scene.Frame) {
	if s == nil || s.controller == nil {
		return
	}
	width, _ := frame.Size()
	table := s.table()
	columns := table.Layout(width)
	s.controller.List().DrawRows(frame, func(view grid.View, row int, _ T, selected bool) {
		base := s.appearance.palette().Text
		if selected && s.controller.List().Focused() {
			base = base.Merge(s.appearance.palette().Selection)
			view.Fill(view.Bounds(), s.appearance.palette().Selection)
		}
		columns.Cells(view, row, base)
	})
}

// Handle routes navigation, pointer selection and value actions to the controller.

// Do runs a navigation or value action.

// Focus takes or releases the keyboard.

func (s *Settings[T]) table() Table {
	return Table{
		Theme: s.appearance.palette(),
		Columns: []Column{
			{Size: layout.Flex(1).AtLeast(1)},
			{Align: layout.End, Size: layout.Measured(0, s.ValueWidth)},
		},
		Rows: s.controller.List().Len(),
		Cell: func(row, column int) Cell {
			item, ok := s.controller.List().At(row)
			if !ok {
				return Cell{}
			}
			if column == 0 {
				return LabelCell(Label{Text: project(s.Label, item), Ellipsis: s.Glyphs.Ellipsis})
			}
			return LabelCell(Label{
				Text: project(s.Value, item), Style: s.appearance.palette().Accent,
				Align: layout.End, Ellipsis: s.Glyphs.Ellipsis,
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

// Element returns the controller's one scene identity with its kit appearance installed.
func (s *Settings[T]) Element() scene.Element {
	if s == nil || s.controller == nil {
		return scene.Element{}
	}
	return s.controller.Element()
}
