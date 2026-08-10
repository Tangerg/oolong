// Package markdownlook maps the example theme onto Markdown.
//
// It lives with the examples because this mapping is application appearance; the
// Markdown module does not choose a component theme or terminal glyph set.
package markdownlook

import (
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/markdown"
)

// New returns the document appearance shared by the Markdown examples.
func New(theme kit.Theme, glyphs kit.Glyphs) markdown.Look {
	return markdown.Look{
		Text:     theme.Text,
		Headings: []grid.Style{theme.Heading, theme.Strong},
		Strong:   theme.Strong,
		Emphasis: grid.Style{Attr: grid.Italic},
		Struck:   grid.Style{Attr: grid.Strike},
		Code:     theme.Info,
		Block:    theme.Muted,
		Link:     theme.Accent,
		Quote:    theme.Muted,
		Rail:     theme.Subtle,
		Marker:   theme.Accent,
		Rule:     theme.Subtle,
		Glyphs: markdown.Glyphs{
			Bullet:    glyphs.Bullet,
			Bar:       glyphs.Vertical,
			Divider:   glyphs.Horizontal,
			Checked:   glyphs.Taken,
			Unchecked: glyphs.Free,
		},
	}
}
