package kit

import (
	"image"
	"slices"
	"sync"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// FormConfig supplies the form's initial fields, semantic title, submission policy and appearance. Scope supplies the live application palette.
// Fields give this form their presentation slot; supplying a kit Input's Controller
// explicitly replaces its standalone skin with the form look. Values remain owned
// by those exact controllers.
type FormConfig struct {
	Key        string
	Scope      *ThemeScope
	Appearance ThemeOverride
	Theme      Theme
	Glyphs     Glyphs
	Title      string
	Hints      []keymap.Action
	Fields     []base.Field
	Keys       *keymap.Map
	Gap        int
	Check      func() error
	Submit     func(base.SubmissionID) (scene.ActionResult[base.SubmissionID], error)
	Cancel     func(base.SubmissionID) error
}

// Form composes one base.Form with a title, help and field appearance. It does not own a second field collection, validation state or submission attempt.
type Form struct {
	appearance appearance
	Glyphs     Glyphs
	hints      []keymap.Action
	controller *base.Form
	group      *scene.Group
}

// NewForm constructs the form and its appearance. It panics for invalid fields or scene relations, or competing Theme and Scope sources.
func NewForm(config FormConfig) *Form {
	look, appearanceErr := newAppearance(config.Theme, config.Scope, config.Appearance)
	if appearanceErr != nil {
		panic(appearanceErr)
	}
	f := &Form{appearance: look, Glyphs: config.Glyphs, hints: slices.Clone(config.Hints)}
	f.controller = base.NewForm(base.FormConfig{
		Key: config.Key, Title: config.Title, Fields: config.Fields, Keys: config.Keys, Gap: config.Gap, Check: config.Check, Submit: config.Submit, Cancel: config.Cancel,
		PaintField: func(frame scene.Frame, field base.Field) {
			if themed, ok := field.(base.ThemedField); ok {
				themed.DrawWith(frame, f.appearance.palette().Look(f.Glyphs))
			} else {
				field.Draw(frame)
			}
		},
	})
	f.group = scene.NewGroup(scene.GroupConfig{
		Items: []scene.Item{{Element: f.controller.Element(), Size: layout.Flex(1)}},
		Element: scene.ElementConfig{
			Measure: func(axis layout.Axis, width int) int {
				if axis == layout.Down {
					return f.HeightForWidth(width)
				}
				return 0
			},
			Paint: func(frame scene.Frame) {
				w, h := frame.Size()
				if f.controller.Title() != "" {
					Label{Text: f.controller.Title(), Style: f.appearance.palette().Heading, Ellipsis: f.Glyphs.Ellipsis}.Draw(frame.View)
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

// Controller is the sole owner of fields, validation and asynchronous submission settlement.
func (f *Form) Controller() *base.Form { return f.controller }

// Element is the composed title, base form and help scene subtree.
func (f *Form) Element() scene.Element {
	if f == nil || f.group == nil {
		return scene.Element{}
	}
	return f.group.Element()
}

// HeightForWidth measures the form and its title/help without changing validation.
func (f *Form) HeightForWidth(width int) int {
	return layout.Sum(f.titleRows(), f.help().HeightForWidth(width), f.controller.HeightForWidth(width))
}

func (f *Form) titleRows() int {
	if f.controller.Title() != "" {
		return 1
	}
	return 0
}

func (f *Form) help() Help { return Help{Theme: f.appearance.palette(), Keys: f.keys(), Show: f.hints} }

var defaultFormKeys = sync.OnceValue(base.DefaultFormKeys)

func (f *Form) keys() *keymap.Map {
	if f.controller.Keys != nil {
		return f.controller.Keys
	}
	return defaultFormKeys()
}
