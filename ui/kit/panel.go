package kit

import (
	"image"

	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// PanelConfig supplies box appearance and one scene child.
type PanelConfig struct {
	Box     Box
	Content scene.Element
}

// Panel combines box geometry with one scene child. It owns appearance; the scene
// performs every coordinate transform, focus change and pointer route.
// Panel must not be copied after first use; its collaborators share one live identity.
type Panel struct {
	noCopy noCopy
	box    Box
	group  *scene.Group
}

// NewPanel composes box geometry with a child. It panics for invalid box alignment or a child already owned by an incompatible scene relation.
func NewPanel(config PanelConfig) *Panel {
	if err := config.Box.validate(); err != nil {
		panic(err)
	}
	p := &Panel{box: config.Box.clone()}
	p.SetContent(config.Content)
	return p
}

func (p *Panel) ensure() {
	if p.group != nil {
		return
	}
	p.group = scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{
		Paint:   func(frame scene.Frame) { p.box.paint(frame.View) },
		Measure: p.measure,
	}, Layout: func(space image.Point, items []scene.Item) []image.Rectangle {
		if len(items) == 0 {
			return nil
		}
		return []image.Rectangle{p.box.InnerRect(space)}
	}})
}

func (p *Panel) Element() scene.Element { p.ensure(); return p.group.Element() }

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

func (p *Panel) Box() Box { return p.box.clone() }

// SetBox panics for invalid title or footer alignment or with scene.ErrBusy outside
// owner mutation, before the retained presentation can change.
func (p *Panel) SetBox(value Box) {
	element := p.Element()
	if err := element.CheckMutation(); err != nil {
		panic(err)
	}
	if err := value.validate(); err != nil {
		panic(err)
	}
	p.box = value.clone()
	element.Invalidate()
}
