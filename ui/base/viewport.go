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
	noCopy noCopy
	keys   keyBinding
	scroll Scroll
	group  *scene.Group
}

// NewViewport composes a child and scroll intent. It panics if the child already belongs to another scene relation.
func NewViewport(config ViewportConfig) *Viewport {
	p := &Viewport{keys: keyBinding{current: config.Keys}}
	p.SetContent(config.Content)
	return p
}

func (p *Viewport) ensure() {
	if p.group != nil {
		return
	}
	p.group = scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{
		Measure: rowsMeasure(p.HeightForWidth), Focusable: func() bool { return !p.Content().IsZero() },
		Semantics: func() scene.Semantic { return scene.Semantic{Role: scene.RoleDocument} },
		Actions: []scene.ActionBinding{p.scroll.action("Scroll content", func(ScrollRequest) scene.Availability {
			if p.Content().IsZero() {
				return scene.Availability{Disabled: true, Reason: "viewport has no content"}
			}
			return scene.Availability{}
		})},
		Paint: func(frame scene.Frame) {
			w, h := frame.Size()
			p.scroll.Stage(frame, p.Content().Measure(layout.Down, w), h)
		},
		Handle: func(event scene.Event) scene.EventResult {
			return scene.EventResult{Consumed: !p.Content().IsZero() && p.scroll.Handle(event.Input(), p.keys.current)}
		},
	}, Layout: func(space image.Point, items []scene.Item) []image.Rectangle {
		if len(items) == 0 {
			return nil
		}
		total := items[0].Element.Measure(layout.Down, space.X)
		offset := p.scroll.Resolve(total, space.Y)
		return []image.Rectangle{grid.Area(0, -offset, space.X, total)}
	}})
}

func (p *Viewport) Element() scene.Element {
	p.ensure()
	p.scroll.element = p.group.Element()
	p.keys.attach(p.group.Element(), &p.scroll.matcher)
	return p.group.Element()
}

func (p *Viewport) Content() scene.Element { item, _ := p.group.At(0); return item.Element }

// SetContent replaces the child and requests focus transfer when appropriate. It panics for an invalid scene relation or mutation during presentation or observation.
func (p *Viewport) SetContent(content scene.Element) {
	p.ensure()
	requireMutable(p.Element())
	previous := p.Content()
	if previous == content {
		return
	}
	transfer := previous.FocusedWithin()
	var items []scene.Item
	if !content.IsZero() {
		items = []scene.Item{{Element: content, Size: layout.Flex(1)}}
	}
	if err := p.group.Set(items...); err != nil {
		panic(err)
	}
	if transfer {
		_ = content.RequestFocusFirst()
	}
}

// Scroll returns the sole scrolling intention owner used by this viewport.
func (p *Viewport) Scroll() *Scroll { return &p.scroll }

func (p *Viewport) HeightForWidth(width int) int { return p.Content().Measure(layout.Down, width) }

func (p *Viewport) Do(action keymap.Action) bool {
	return p.scroll.Do(action)
}

func (p *Viewport) Keys() *keymap.Map { return p.keys.current }

func (p *Viewport) SetKeys(keys *keymap.Map) {
	p.keys.set(p.Element(), &p.scroll.matcher, keys)
}
