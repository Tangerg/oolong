package kit

import (
	"errors"
	"image"

	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

var errPanelBoxTheme = errors.New("kit: a panel's palette comes from its Theme or Scope, not Box.Theme")

var errPanelTitle = errors.New("kit: this panel's title belongs to its controller")

// PanelConfig supplies the palette source, box geometry and one scene child. Like
// every live skin, the palette comes from Theme or Scope; Box.Theme must be zero.
type PanelConfig struct {
	Scope      *ThemeScope
	Appearance ThemeOverride
	Theme      Theme
	Box        Box
	Content    scene.Element
}

// Panel combines box geometry with one scene child. It owns appearance; the scene
// performs every coordinate transform, focus change and pointer route.
// Panel must not be copied after first use; its collaborators share one live identity.
type Panel struct {
	noCopy     noCopy
	appearance appearance
	box        Box
	// title, when set, projects a title owned by another controller. The box then
	// carries no title of its own, so the two can never disagree.
	title func() string
	group *scene.Group
}

// NewPanel composes box geometry with a child. It panics for invalid box alignment,
// a Box.Theme, competing Theme and Scope sources or a child already owned by an
// incompatible scene relation.
func NewPanel(config PanelConfig) *Panel {
	look, err := newAppearance(config.Theme, config.Scope, config.Appearance)
	if err != nil {
		panic(err)
	}
	return newPanel(look, config.Box, config.Content, nil)
}

func newPanel(look appearance, box Box, content scene.Element, title func() string) *Panel {
	p := &Panel{appearance: look, title: title}
	if err := p.admit(box); err != nil {
		panic(err)
	}
	p.box = box.clone()
	p.group = scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{
		Paint:   func(frame scene.Frame) { p.presented().paint(frame.View) },
		Measure: p.measure,
	}, Layout: func(space image.Point, items []scene.Item) []image.Rectangle {
		if len(items) == 0 {
			return nil
		}
		return []image.Rectangle{p.box.InnerRect(space)}
	}})
	p.appearance.bind(p.group.Element())
	p.SetContent(content)
	return p
}

func (p *Panel) admit(box Box) error {
	if err := box.validate(); err != nil {
		return err
	}
	if box.Theme != (Theme{}) {
		return errPanelBoxTheme
	}
	if p.title != nil && box.Title != "" {
		return errPanelTitle
	}
	return nil
}

func (p *Panel) presented() Box {
	box := p.box
	box.Theme = p.appearance.palette()
	if p.title != nil {
		box.Title = p.title()
	}
	return box
}

func (p *Panel) Element() scene.Element { return p.group.Element() }

func (p *Panel) Content() scene.Element { item, _ := p.group.At(0); return item.Element }

// SetContent replaces the child and requests focus transfer from the old content. It panics for an invalid scene relation or mutation during presentation or observation.
func (p *Panel) SetContent(child scene.Element) {
	if err := p.Element().CheckMutation(); err != nil {
		panic(err)
	}
	previous := p.Content()
	if child == previous {
		return
	}
	transfer := previous.FocusedWithin()
	var items []scene.Item
	if !child.IsZero() {
		items = []scene.Item{{Element: child, Size: layout.Flex(1)}}
	}
	if err := p.group.Set(items...); err != nil {
		panic(err)
	}
	if transfer {
		_ = child.RequestFocusFirst()
	}
}

func (p *Panel) HeightForWidth(width int) int {
	return p.measure(layout.Down, width)
}

func (p *Panel) measure(axis layout.Axis, across int) int {
	overhead := p.box.Overhead()
	along, cross := overhead.X, overhead.Y
	if axis == layout.Down {
		along, cross = overhead.Y, overhead.X
	}
	return layout.Sum(along, p.Content().Measure(axis, layout.Remaining(across, cross)))
}

// Box returns the retained geometry. Its Theme is zero: the palette is resolved
// from the panel's scope when painting.
func (p *Panel) Box() Box { return p.box.clone() }

// SetBox panics for invalid title or footer alignment, a Box.Theme, a title on a
// panel whose title has another owner, or with scene.ErrBusy outside owner
// mutation, before the retained presentation can change.
func (p *Panel) SetBox(value Box) {
	element := p.Element()
	if err := element.CheckMutation(); err != nil {
		panic(err)
	}
	if err := p.admit(value); err != nil {
		panic(err)
	}
	p.box = value.clone()
	element.Invalidate()
}
