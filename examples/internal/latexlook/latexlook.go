// Package latexlook keeps formula appearance with the examples because palettes
// belong to the application and glyph repertoire comes from the host environment.
package latexlook

import (
	"github.com/Tangerg/oolong/extensions/latex"
	"github.com/Tangerg/oolong/ui/kit"
)

func New(theme kit.Theme, locale string) latex.Look {
	return latex.Look{
		Text: theme.Text, Rule: theme.Subtle, Error: theme.Danger,
		Glyphs: latex.GlyphsFor(locale),
	}
}
