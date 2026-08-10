// Package latexlook maps the example theme onto mathematical LaTeX.
//
// It lives with the examples because formula colour is application appearance and
// terminal glyph repertoire is a fact supplied by the host environment.
package latexlook

import (
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/latex"
)

// New returns formula appearance for locale's terminal repertoire.
func New(theme kit.Theme, locale string) latex.Look {
	return latex.Look{
		Text: theme.Text, Rule: theme.Subtle, Error: theme.Danger,
		Glyphs: latex.GlyphsFor(locale),
	}
}
