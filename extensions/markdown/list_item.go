package markdown

import (
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

// The marker belongs to the item, not its first parsed child: an extension may
// produce no rows at one width and visible content at another. Keeping the children
// together also gives a retained Block the same semantics outside its original Doc.
type listItem struct {
	body   documentView
	prefix blockPrefix
	marker *prefixMark
}

func (item *listItem) HeightForWidth(width int) int {
	if width <= 0 {
		return 0
	}
	return max(1, item.body.HeightForWidth(width))
}

func (item *listItem) Draw(view grid.View) {
	if view.Empty() {
		return
	}
	width, _ := view.Size()
	item.body.Draw(view)
	inset := view.Sub(grid.Area(0, 0, min(item.prefix.indent, width-1), 1))
	if item.body.HeightForWidth(width) == 0 {
		item.prefix.rails.draw(inset, 0)
	}
	item.marker.draw(inset, 0)
}

func (item *listItem) Rows(width int) []text.Row {
	if width <= 0 {
		return nil
	}
	if rows := item.body.Rows(width); len(rows) > 0 {
		return rows
	}
	return []text.Row{{Offset: min(item.prefix.indent, width-1)}}
}
