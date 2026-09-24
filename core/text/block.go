package text

import (
	"sync"

	"github.com/Tangerg/oolong/core/grid"
)

// BlockConfig configures an immutable collection of styled logical lines.
type BlockConfig struct {
	Lines []Line
	// Wrap reflows lines to the available width. False clips each logical line.
	Wrap bool
}

// Block is passive styled text. Construction owns its lines; measurement,
// drawing and text projection share the same wrap or clip rule. Its zero value
// is explicit empty content.
type Block struct {
	lines []Line
	wrap  bool
	cache *blockCache
}

// NewBlock copies the source, including strings retained by spans.
func NewBlock(cfg BlockConfig) *Block {
	return &Block{lines: CloneLines(cfg.Lines), wrap: cfg.Wrap, cache: &blockCache{}}
}

type blockRow struct {
	Wrapped
	logical int
}

// A single width projection bounds retention during resize. Published rows are
// immutable, so concurrent draws can keep using the previous projection.
type blockCache struct {
	mu    sync.Mutex
	width int
	rows  []blockRow
}

func (b *Block) physical(width int) []blockRow {
	if b == nil || width <= 0 || len(b.lines) == 0 {
		return nil
	}
	b.cache.mu.Lock()
	defer b.cache.mu.Unlock()
	if b.cache.width == width {
		return b.cache.rows
	}
	var rows []blockRow
	for index, line := range b.lines {
		if !b.wrap {
			rows = append(rows, blockRow{Line: line.Truncate(width, ""), logical: index + 1})
			continue
		}
		for _, wrapped := range line.Wrap(width) {
			rows = append(rows, blockRow{Wrapped: wrapped, logical: index + 1})
		}
	}
	b.cache.width, b.cache.rows = width, rows
	return rows
}

// HeightForWidth reports physical rows, with no rows at nonpositive widths.
func (b *Block) HeightForWidth(width int) int { return len(b.physical(width)) }

// Draw paints the visible part without changing the source.
func (b *Block) Draw(view grid.View) {
	width, _ := view.Size()
	rows := b.physical(width)
	visible := view.Visible()
	for y := max(0, visible.Min.Y); y < min(len(rows), visible.Max.Y); y++ {
		rows[y].Line.Draw(view, 0, y)
	}
}

// Rows projects the same physical lines used by Draw for selection and search.
func (b *Block) Rows(width int) []Row {
	if b == nil || width <= 0 {
		return nil
	}
	physical := b.physical(width)
	rows := make([]Row, len(physical))
	for i, row := range physical {
		rows[i] = Row{Text: row.Line.String(), Line: row.logical, Joined: row.Joined, Gap: row.Gap}
	}
	return rows
}
