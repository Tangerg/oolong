// Package grid owns styled grapheme cells, clipped [View]s and terminal frame
// encoding. [Screen] uses absolute screen geometry; [Inline] uses a relative block
// and terminal scrollback. Higher layers draw cells rather than escape sequences.
// Geometry uses standard half-open image rectangles.
package grid

import (
	"errors"
	"fmt"
	"image"
	"math"

	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/internal/identity"
)

// MaxTerminalCells bounds a physical terminal's reported geometry before its
// cell stores are allocated. It does not limit independently owned surfaces.
const MaxTerminalCells = 1 << 18

var ErrInvalidTerminalSize = errors.New("grid: invalid terminal size")

// ValidateTerminalSize reports whether the reported dimensions can safely back a
// terminal screen. Hosts and screen assertion models share this admission rule.
func ValidateTerminalSize(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("%w: %dx%d", ErrInvalidTerminalSize, width, height)
	}
	if width > MaxTerminalCells/height {
		return fmt.Errorf("%w: %dx%d exceeds %d cells", ErrInvalidTerminalSize,
			width, height, MaxTerminalCells)
	}
	return nil
}

// Area names origin-and-size geometry separately from image.Rect's two-corner
// geometry, because their identical argument types cannot catch a mix-up. The
// rectangle is half-open; negative sizes collapse and overflowing ends saturate.
func Area(x, y, w, h int) image.Rectangle {
	w, h = max(w, 0), max(h, 0)
	return image.Rectangle{
		Min: image.Pt(x, y),
		Max: image.Pt(layout.Translate(x, w), layout.Translate(y, h)),
	}
}

func translatePoint(point, by image.Point) image.Point {
	return image.Pt(layout.Translate(point.X, by.X), layout.Translate(point.Y, by.Y))
}

func translateRect(rect image.Rectangle, by image.Point) image.Rectangle {
	return image.Rectangle{Min: translatePoint(rect.Min, by), Max: translatePoint(rect.Max, by)}
}

func untranslateRect(rect image.Rectangle, by image.Point) image.Rectangle {
	return image.Rectangle{
		Min: image.Pt(layout.Relative(rect.Min.X, by.X), layout.Relative(rect.Min.Y, by.Y)),
		Max: image.Pt(layout.Relative(rect.Max.X, by.X), layout.Relative(rect.Max.Y, by.Y)),
	}
}

func rectangleSize(rect image.Rectangle) image.Point {
	return image.Pt(coordinateExtent(rect.Min.X, rect.Max.X), coordinateExtent(rect.Min.Y, rect.Max.Y))
}

func coordinateExtent(from, to int) int {
	if to <= from {
		return 0
	}
	extent := uint(to) - uint(from)
	if extent > uint(math.MaxInt) {
		return math.MaxInt
	}
	return int(extent)
}

// Drawable is passive content that can measure its height at a width and draw into
// exactly that space. Retained content and permanent inline publication share this
// contract. Layout adapters supply the explicit axis measurement separately.
type Drawable interface {
	Draw(view View)
	HeightForWidth(width int) int
}

// IsNilDrawable recognizes absent content across an interface boundary, including
// typed nils. Each consumer decides whether absence is a no-op or a refusal.
func IsNilDrawable(body Drawable) bool {
	return identity.IsNil(body)
}

// span describes one display atom. Zero is an ordinary one-column cell, a
// positive value is the width stored on an atom's head, and a negative value is
// a continuation cell's distance back to that head.
//
// It is unexported so an atom cannot be split from outside the package. Unicode
// grapheme boundaries and terminal columns are related but not identical: a
// grapheme containing a spacing modifier can occupy more than two columns, so a
// head/trail pair is not a sufficient storage model.
type span int

// Cell content changes only through View so an atom's measured width and
// continuation cells cannot diverge. Copied rows may change Style and Link
// without changing geometry. The zero value is a valid blank single-column cell.
type Cell struct {
	content string
	Style   Style
	// Link is an OSC 8 hyperlink target. It is cell metadata rather than part of
	// Style because a hyperlink has its own open/close protocol on the wire,
	// while everything in Style is one SGR parameter list.
	Link string

	span span
}

// Content is empty for blanks and continuations. Inspect Width for coordinates:
// concatenating text alone cannot preserve independently placed atom boundaries.
func (c Cell) Content() string { return c.content }

// Width is how many columns the cell occupies: the complete display width on an
// atom's head, zero on a continuation cell, and one otherwise.
func (c Cell) Width() int {
	if c.span < 0 {
		return 0
	}
	return max(int(c.span), 1)
}

func (c Cell) Blank() bool { return c.content == "" && c.span >= 0 }
