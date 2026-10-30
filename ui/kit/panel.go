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
	Box    Box
	child  scene.Element
	group  *scene.Group
}

// NewPanel composes box geometry with a child. It panics for a child already owned by an incompatible scene relation.
func NewPanel(config PanelConfig) *Panel {
	p := &Panel{Box: config.Box}
	p.SetContent(config.Content)
	return p
}

func (p *Panel) ensure() {
	if p.group != nil {
		return
	}
	p.group = scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{
		Paint: func(frame scene.Frame) { p.Box.paint(frame.View) },
		Measure: func(axis layout.Axis, across int) int {
			if axis == layout.Down {
				return p.HeightForWidth(across)
			}
			return layout.Sum(p.Box.Overhead().X, p.child.Measure(axis, across))
		},
	}, Layout: func(space image.Point, items []scene.Item) []image.Rectangle {
		if len(items) == 0 {
			return nil
		}
		return []image.Rectangle{p.Box.InnerRect(space)}
	}})
}

// Element is the panel's one scene group.
func (p *Panel) Element() scene.Element { p.ensure(); return p.group.Element() }

// Content returns the scene child currently inside the panel.
func (p *Panel) Content() scene.Element { return p.child }

// SetContent replaces the child and requests focus transfer from the old content. It panics for an invalid scene relation or mutation during presentation or observation.
func (p *Panel) SetContent(child scene.Element) {
	p.ensure()
	transfer := p.child.FocusedWithin()
	var items []scene.Item
	if !child.IsZero() {
		items = []scene.Item{{Element: child, Size: layout.Flex(1)}}
	}
	if err := p.group.Set(items...); err != nil {
		panic(err)
	}
	p.child = child
	if transfer {
		_ = child.RequestFocusFirst()
	}
}

// HeightForWidth adds box overhead to the child's measured height.
func (p *Panel) HeightForWidth(width int) int {
	overhead := p.Box.Overhead()
	return layout.Sum(overhead.Y, p.child.Measure(layout.Down, layout.Remaining(width, overhead.X)))
}
