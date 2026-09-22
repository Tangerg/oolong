package text

import "github.com/Tangerg/oolong/core/grid"

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
}

// NewBlock copies the source, including strings retained by spans.
func NewBlock(cfg BlockConfig) *Block {
	return &Block{lines: CloneLines(cfg.Lines), wrap: cfg.Wrap}
}

func (b *Block) physical(width int) []Wrapped {
	if b == nil || width <= 0 {
		return nil
	}
	var rows []Wrapped
	for _, line := range b.lines {
		if b.wrap {
			rows = append(rows, line.Wrap(width)...)
		} else {
			rows = append(rows, Wrapped{Line: line.Truncate(width, "")})
		}
	}
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
	var rows []Row
	for index, line := range b.lines {
		if !b.wrap {
			rows = append(rows, Row{Text: line.Truncate(width, "").String(), Line: index + 1})
			continue
		}
		source, previous := line.String(), 0
		for _, wrapped := range line.Wrap(width) {
			gap := ""
			if wrapped.Joined {
				gap = source[previous:wrapped.From]
			}
			rows = append(rows, Row{Text: wrapped.Line.String(), Line: index + 1, Joined: wrapped.Joined, Gap: gap})
			previous = wrapped.To
		}
	}
	return rows
}
