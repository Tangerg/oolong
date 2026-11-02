package text

import "github.com/Tangerg/oolong/ui/core/grid"

// SGR distinguishes an absent override from an explicit attribute reset. An
// additive grid.Style cannot represent that distinction without reviving Base.
type rendition struct {
	foreground, background grid.Color
	attributes             attributeOverrides
}

func (r rendition) resolve(base grid.Style) grid.Style {
	if !r.foreground.Default() {
		base.FG = r.foreground
	}
	if !r.background.Default() {
		base.BG = r.background
	}
	base.Attr = r.attributes.resolve(base.Attr)
	return base
}

type attributeOverrides struct{ enabled, disabled grid.Attr }

func (a *attributeOverrides) set(attributes grid.Attr, enabled bool) {
	if enabled {
		a.enabled |= attributes
		a.disabled &^= attributes
		return
	}
	a.enabled &^= attributes
	a.disabled |= attributes
}

func (a *attributeOverrides) resolve(base grid.Attr) grid.Attr {
	return base&^a.disabled | a.enabled
}
