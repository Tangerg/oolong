package markdown

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
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
		return &documentView{blocks: blocks}, err
	}
}

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
