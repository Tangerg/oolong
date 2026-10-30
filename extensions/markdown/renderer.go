package markdown

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

// RendererConfig freezes the appearance used by each independent whole-source preparation.
type RendererConfig struct{ Look Look }

// NewRenderer captures an owned appearance configuration. The returned renderer
// prepares a complete source snapshot; StreamDocument decides which version is
// current and when it is final. Each result has its own private layout cache and
// exposes no semantic mutation API.
func NewRenderer(cfg RendererConfig) content.Renderer {
	look := cloneLook(cfg.Look)
	return func(ctx context.Context, source string) (grid.Drawable, error) {
		if err := context.Cause(ctx); err != nil {
			return nil, err
		}
		blocks, err := Render(source, look)
		doc := &documentView{}
		doc.doc.SetBlocks(blocks)
		return doc, err
	}
}

type documentView struct{ doc Doc }

func (d *documentView) HeightForWidth(width int) int { return d.doc.HeightForWidth(width) }
func (d *documentView) Draw(view grid.View)          { d.doc.Draw(view) }
func (d *documentView) Rows(width int) []text.Row    { return d.doc.Rows(width) }

func cloneLook(look Look) Look {
	look.Headings = slices.Clone(look.Headings)
	look.extensions = maps.Clone(look.extensions)
	look.Glyphs.Bullet = strings.Clone(look.Glyphs.Bullet)
	look.Glyphs.Bar = strings.Clone(look.Glyphs.Bar)
	look.Glyphs.Divider = strings.Clone(look.Glyphs.Divider)
	look.Glyphs.Checked = strings.Clone(look.Glyphs.Checked)
	look.Glyphs.Unchecked = strings.Clone(look.Glyphs.Unchecked)
	return look
}
