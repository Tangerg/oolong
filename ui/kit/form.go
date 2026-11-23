package kit

import (
	"image"
	"slices"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/setting"
)

// FormConfig carries the base form configuration in Behavior and adds its title,
// help and field appearance. The skin paints fields, so Behavior.PaintField must be
// nil. Scope supplies the live application palette. Fields use the form look while
// attached. Removing a field restores its own presentation; its value and scene
// identity stay with its controller.
type FormConfig struct {
	Behavior   base.FormConfig
	Scope      *ThemeScope
	Appearance ThemeOverride
	Theme      Theme
	Glyphs     Glyphs
	Hints      []keymap.Action
}

// Form composes one base.Form with a title, help and field appearance. It does not own a second field collection, validation state or submission attempt.
// A Form must not be copied after first use: its controller and group retain this appearance.
type Form struct {
	noCopy     noCopy
	appearance appearance
	glyphs     Glyphs
	hints      []keymap.Action
	controller *base.Form
	group      *scene.Group
}

// NewForm panics for negative Gap, invalid fields or scene relations, a field
// painter in Behavior, or competing Theme and Scope palette sources.
func NewForm(config FormConfig) *Form {
	look := newAppearance(config.Theme, config.Scope, config.Appearance)
	requireSkinOwned("form Behavior.PaintField", config.Behavior.PaintField == nil)
	f := &Form{appearance: look, glyphs: config.Glyphs.clone(), hints: slices.Clone(config.Hints)}
	behavior := config.Behavior
	behavior.PaintField = func(frame scene.Frame, field base.Field) {
		if themed, ok := field.(base.ThemedField); ok {
			themed.DrawWith(frame, f.appearance.palette().Look(f.glyphs))
		} else {
			field.Draw(frame)
		}
	}
	f.controller = base.NewForm(behavior)
	f.group = scene.NewGroup(scene.GroupConfig{
		Items: []scene.Item{{Element: f.controller.Element(), Size: layout.Flex(1)}},
		Element: scene.ElementConfig{
			Measure: layout.MeasureRows(f.HeightForWidth),
			Paint: func(frame scene.Frame) {
				w, h := frame.Size()
				if f.controller.Title() != "" {
					Label{Text: f.controller.Title(), Style: f.appearance.palette().Heading, Ellipsis: f.glyphs.Ellipsis}.Draw(frame.View)
				}
				help := f.help()
				rows := help.HeightForWidth(w)
				if rows > 0 {
					help.Draw(frame.Sub(grid.Area(0, layout.Remaining(h, rows), w, rows)).View)
				}
			},
		}, Layout: func(space image.Point, _ []scene.Item) []image.Rectangle {
			top := f.titleRows()
			return []image.Rectangle{grid.Area(0, top, space.X, layout.Remaining(space.Y, top, f.help().HeightForWidth(space.X)))}
		},
	})
	f.appearance.bind(f.Element())
	return f
}

func (f *Form) Controller() *base.Form { return f.controller }

func (f *Form) Element() scene.Element {
	if f.group == nil {
		return scene.Element{}
	}
	return f.group.Element()
}

func (f *Form) HeightForWidth(width int) int {
	return layout.Sum(f.titleRows(), f.help().HeightForWidth(width), f.controller.HeightForWidth(width))
}

func (f *Form) titleRows() int {
	if f.controller.Title() != "" {
		return 1
	}
	return 0
}

func (f *Form) help() Help {
	return Help{Theme: f.appearance.palette(), Keys: KeysFunc(f.controller.KeysFor), Show: f.hints}
}

func (f *Form) Glyphs() Glyphs { return f.glyphs.clone() }

// SetGlyphs panics with scene.ErrBusy outside owner mutation: retained
// presentation must stay stable during observation and frame preparation.
func (f *Form) SetGlyphs(value Glyphs) { setting.Replace(f.Element(), &f.glyphs, value.clone()) }
