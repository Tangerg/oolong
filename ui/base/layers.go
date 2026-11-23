package base

import (
	"slices"

	"github.com/Tangerg/oolong/ui/core/scene"
)

// Layers is content with an ordered stack of overlays above it: dialogs, popups,
// anything drawn over the interface. The most recently shown overlay is on top, so
// it draws over the others and, when modal, holds the keyboard; dismissing it
// returns both to the one beneath. Overlays keep their own placement and open
// state; Layers owns only which ones are present and in what order. The scene
// still owns attachment, focus and modal routing.
//
// The zero value has no content and no overlays. Layers must not be copied after
// first use; its group is the one relation its overlays are attached through.
type Layers struct {
	noCopy noCopy
	group  *scene.Group
}

// LayersConfig supplies the content beneath every overlay.
type LayersConfig struct {
	Content scene.Element
}

// NewLayers panics if the content already belongs to another scene relation.
func NewLayers(config LayersConfig) *Layers {
	l := &Layers{}
	l.SetContent(config.Content)
	return l
}

func (l *Layers) Element() scene.Element { return l.ensure().Element() }

func (l *Layers) ensure() *scene.Group {
	if l.group == nil {
		l.group = scene.NewGroup(scene.GroupConfig{Overlay: true})
	}
	return l.group
}

// Content is the element beneath every overlay, or the zero element.
func (l *Layers) Content() scene.Element {
	item, ok := l.ensure().At(0)
	if !ok || item.Key != layersContentKey {
		return scene.Element{}
	}
	return item.Element
}

// SetContent replaces the element beneath the overlays without changing them. It
// panics for an invalid scene relation or mutation during presentation or
// observation.
func (l *Layers) SetContent(content scene.Element) {
	overlays := l.Overlays()
	items := make([]scene.Item, 0, len(overlays)+1)
	if !content.IsZero() {
		items = append(items, scene.Item{Key: layersContentKey, Element: content})
	}
	for _, overlay := range overlays {
		items = append(items, scene.Item{Element: overlay})
	}
	l.set(items)
}

// Overlays returns the shown overlays, bottom first.
func (l *Layers) Overlays() []scene.Element {
	var overlays []scene.Element
	for _, item := range l.ensure().Items() {
		if item.Key != layersContentKey {
			overlays = append(overlays, item.Element)
		}
	}
	return overlays
}

// Top is the overlay drawn above every other, and whether any is shown.
func (l *Layers) Top() (scene.Element, bool) {
	overlays := l.Overlays()
	if len(overlays) == 0 {
		return scene.Element{}, false
	}
	return overlays[len(overlays)-1], true
}

// Show puts overlay on top, adding it or raising one already shown without
// remounting it. It panics for the zero element, an invalid scene relation or
// mutation during presentation or observation.
func (l *Layers) Show(overlay scene.Element) {
	if overlay.IsZero() {
		panic("base: Layers cannot show the zero element")
	}
	group := l.ensure()
	items := group.Items()
	if at := slices.IndexFunc(items, func(item scene.Item) bool {
		return item.Key != layersContentKey && item.Element == overlay
	}); at >= 0 {
		if err := group.Move(at, len(items)-1); err != nil {
			panic(err)
		}
		return
	}
	l.set(append(items, scene.Item{Element: overlay}))
}

// Dismiss removes overlay and reports whether it was shown. It panics for an
// invalid scene relation or mutation during presentation or observation.
func (l *Layers) Dismiss(overlay scene.Element) bool {
	items := l.ensure().Items()
	kept := slices.DeleteFunc(slices.Clone(items), func(item scene.Item) bool {
		return item.Key != layersContentKey && item.Element == overlay
	})
	if len(kept) == len(items) {
		return false
	}
	l.set(kept)
	return true
}

func (l *Layers) set(items []scene.Item) {
	if err := l.ensure().Set(items...); err != nil {
		panic(err)
	}
}

// layersContentKey marks the content's item, so the content is never mistaken for
// an overlay, whichever element it is.
const layersContentKey = "layers-content"
