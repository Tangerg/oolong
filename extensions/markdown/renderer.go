package markdown

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
)

// RendererConfig is snapshotted so later caller edits cannot change retained
// source projections.
type RendererConfig struct{ Look Look }

// NewRenderer requires complete snapshots to keep parse semantics independent of chunk boundaries.
// StreamDocument owns revision and finalization; each result owns its layout cache.
func NewRenderer(cfg RendererConfig) content.Renderer {
	look := cloneLook(cfg.Look)
	return func(ctx context.Context, source string) (grid.Drawable, error) {
		if err := context.Cause(ctx); err != nil {
			return nil, err
		}
		blocks, err := Render(source, look)
		if err != nil && blocks == nil {
			return nil, err
		}
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
