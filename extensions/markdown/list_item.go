package markdown

import (
	"context"
	"slices"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

// The marker belongs to the item, not its first parsed child: an extension may
// produce no rows at one width and visible content at another. Keeping the children
// together also gives a retained Block the same semantics outside its original Doc.
type listItem struct {
	body   Snapshot
	prefix blockPrefix
	marker *prefixMark
}

func (item *listItem) layout(ctx context.Context, width int) (placedBody, error) {
	body, err := item.body.prepareLayout(ctx, width)
	if err != nil {
		return nil, err
	}
	return &listItemLayout{
		body: body, marker: item.marker, rails: item.prefix.rails,
		inset: min(item.prefix.indent, width-1),
	}, nil
}

// A retained parent must draw and project the same child geometry after another
// preparation replaces the source item's width memo.
type listItemLayout struct {
	body          *documentLayout
	marker, rails *prefixMark
	inset         int
}

func (item *listItemLayout) height() int { return max(1, item.body.height) }

func (item *listItemLayout) draw(view grid.View) {
	if view.Empty() {
		return
	}
	item.body.Draw(view)
	inset := view.Sub(grid.Area(0, 0, item.inset, 1))
	if item.body.height == 0 {
		item.rails.draw(inset, 0)
	}
	item.marker.draw(inset, 0)
}

func (item *listItemLayout) project(ctx context.Context) ([]text.Row, error) {
	rows, err := item.body.prepareRows(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows.values) == 0 {
		return []text.Row{{Offset: item.inset}}, nil
	}
	return slices.Clone(rows.values), nil
}
