package kit

import (
	"errors"
	"fmt"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

var ErrInvalidCellGlyph = errors.New("kit: glyph must be empty or occupy one terminal cell")

// CellGlyph keeps one-column furniture from overwriting an adjacent display atom.
// The zero value omits the mark; nonempty values contain one printable grapheme.
type CellGlyph struct {
	value string
}

func NewCellGlyph(value string) (CellGlyph, error) {
	if value == "" {
		return CellGlyph{}, nil
	}
	if text.Printable(value) == value {
		for _, cluster := range text.Clusters(value) {
			if len(cluster) == len(value) && grid.ClusterWidth(cluster) == 1 {
				return CellGlyph{value: value}, nil
			}
			break
		}
	}
	return CellGlyph{}, fmt.Errorf("%w: %q", ErrInvalidCellGlyph, value)
}

func (g CellGlyph) String() string { return g.value }

func mustCellGlyph(value string) CellGlyph {
	glyph, err := NewCellGlyph(value)
	if err != nil {
		panic(err)
	}
	return glyph
}

func mustCellGlyphs(values ...string) []CellGlyph {
	glyphs := make([]CellGlyph, len(values))
	for i, value := range values {
		glyphs[i] = mustCellGlyph(value)
	}
	return glyphs
}
