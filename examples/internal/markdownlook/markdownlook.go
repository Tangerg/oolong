// Package markdownlook keeps Markdown appearance with the examples because the
// application owns its palette and the host owns its terminal glyph repertoire.
package markdownlook

import (
	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/kit"
)

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
