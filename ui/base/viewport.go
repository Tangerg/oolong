package base

import (
	"image"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// ViewportConfig supplies one scene child and a scroll key map.
type ViewportConfig struct {
	Content scene.Element
	Keys    *keymap.Map
}

// Viewport composes one scroll intention with a scene layout. The scene clips and
// transforms the child route; the viewport does not forward input or focus itself.
// Viewport must not be copied after first use; its collaborators share one live identity.
type Viewport struct {
	noCopy  noCopy
	Keys    *keymap.Map
	content scene.Element
	scroll  Scroll
	group   *scene.Group
}

// NewViewport composes a child and scroll intent. It panics if the child already belongs to another scene relation.
func NewViewport(config ViewportConfig) *Viewport {
	p := &Viewport{Keys: config.Keys}
	p.SetContent(config.Content)
	return p
}

func (p *Viewport) ensure() {
	if p.group != nil {
		return
	}
	p.group = scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{
		Measure: rowsMeasure(p.HeightForWidth), Focusable: func() bool { return !p.content.IsZero() },
		Paint: func(frame scene.Frame) {
			w, h := frame.Size()
			p.scroll.Stage(frame, p.content.Measure(layout.Down, w), h)
		},
		Handle: func(event scene.Event) scene.EventResult {
			return scene.EventResult{Consumed: !p.content.IsZero() && p.scroll.Handle(event.Input, p.Keys)}
		},
	}, Layout: func(space image.Point, items []scene.Item) []image.Rectangle {
		if len(items) == 0 {
			return nil
		}
		total := p.content.Measure(layout.Down, space.X)
		offset := p.scroll.Resolve(total, space.Y)
		return []image.Rectangle{grid.Area(0, -offset, space.X, total)}
	}})
}

// Element is the viewport group; scene alone clips and translates its child input.
func (p *Viewport) Element() scene.Element {
	p.ensure()
	p.scroll.element = p.group.Element()
	return p.group.Element()
}

// Content returns the current child identity.
func (p *Viewport) Content() scene.Element { return p.content }

// SetContent replaces the child and requests focus transfer when appropriate. It panics for an invalid scene relation or mutation during presentation or observation.
func (p *Viewport) SetContent(content scene.Element) {
	p.ensure()
	requireMutable(p.Element())
	transfer := p.content.FocusedWithin()
	var items []scene.Item
	if !content.IsZero() {
		items = []scene.Item{{Element: content, Size: layout.Flex(1)}}
	}
	if err := p.group.Set(items...); err != nil {
		panic(err)
	}
	p.content = content
	if transfer {
		_ = content.RequestFocusFirst()
	}
	p.Element().Invalidate()
}

// Scroll returns the sole scrolling intention owner used by this viewport.
func (p *Viewport) Scroll() *Scroll { return &p.scroll }

// HeightForWidth measures the child content before viewport clipping.
func (p *Viewport) HeightForWidth(width int) int { return p.content.Measure(layout.Down, width) }

// Do applies one named scrolling operation and invalidates the viewport when handled.
func (p *Viewport) Do(action keymap.Action) bool {
	handled := p.scroll.Do(action)
	if handled {
		p.Element().Invalidate()
	}
	return handled
}
