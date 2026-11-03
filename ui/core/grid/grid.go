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

const (
	maxInt = int(^uint(0) >> 1)
)

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
	if extent > uint(maxInt) {
		return maxInt
	}
	return int(extent)
}

type RGB struct{ R, G, B uint8 }

// Blend resolves opacity before output because terminal cells have no alpha
// channel. Opacity is clamped to [0,1].
func (c RGB) Blend(over RGB, opacity float64) RGB {
	opacity = min(max(opacity, 0), 1)
	lerp := func(a, b uint8) uint8 {
		return uint8(math.Round(float64(a) + (float64(b)-float64(a))*opacity))
	}
	return RGB{lerp(c.R, over.R), lerp(c.G, over.G), lerp(c.B, over.B)}
}

// Color is a cell colour: either the terminal's own default, or a truecolor
// value. The zero Color is the default, which is what an unstyled cell wants.
type Color struct {
	set bool
	rgb RGB
}

func RGBColor(r, g, b uint8) Color { return Color{set: true, rgb: RGB{r, g, b}} }

func (c Color) Default() bool { return !c.set }

// RGB returns the colour's components. They are meaningless when the colour is
// the terminal default.
func (c Color) RGB() RGB { return c.rgb }

// Blend cannot resolve a terminal-default color to a number, so it leaves c
// unchanged if either input is unresolved. Ground supplies queried RGB values
// when available; guessing a background could make text disappear.
func (c Color) Blend(over Color, opacity float64) Color {
	if c.Default() || over.Default() {
		return c
	}
	blended := c.rgb.Blend(over.rgb, opacity)
	return RGBColor(blended.R, blended.G, blended.B)
}

type Attr uint8

const (
	Bold Attr = 1 << iota
	Dim
	Italic
	Underline
	Reverse
	Strike
)

func (a Attr) Has(want Attr) bool { return a&want == want }

// Style is how a cell looks. The zero Style is the terminal's own appearance.
type Style struct {
	FG, BG Color
	Attr   Attr
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

// Merge lays over on top of s: whatever over states wins, whatever it leaves at
// its default is inherited. Attributes accumulate, because an overlay that adds
// emphasis should not silently drop the emphasis underneath it.
func (s Style) Merge(over Style) Style {
	out := s
	if !over.FG.Default() {
		out.FG = over.FG
	}
	if !over.BG.Default() {
		out.BG = over.BG
	}
	out.Attr |= over.Attr
	return out
}

// Ground holds queried terminal colors so default cells can follow terminal
// recoloring while blending can resolve actual RGB values. The zero value leaves
// unknown colors unresolved; blending cannot guess a missing background.
type Ground struct{ FG, BG Color }

// Resolve fills in whatever a style left to the terminal, so a caller that needs
// numbers has them. What the terminal never said stays default.
//
// [Reverse] is deliberately not applied. It swaps the two colours on the way to the
// screen, and swapping them here would mean a caller that resolved a style and drew
// it back would reverse it twice.
func (g Ground) Resolve(s Style) Style {
	if s.FG.Default() {
		s.FG = g.FG
	}
	if s.BG.Default() {
		s.BG = g.BG
	}
	return s
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

// Index256 compares the color cube and grayscale ramp so near-gray colors
// keep their luminance. Themeable ANSI entries are excluded: their nominal RGB
// values do not predict what the destination will display.
func (c RGB) Index256() uint8 {
	// Palette arithmetic fits uint8: cube entries end at 231 and the ramp
	// ends at 255, so no narrowing can wrap a selected index.
	r, g, b := nearestCube(c.R), nearestCube(c.G), nearestCube(c.B)
	best := 16 + 36*r + 6*g + b
	bestDist := distance(c, RGB{cube[r], cube[g], cube[b]})

	// The ramp runs 8, 18, 28 … 238. Rounding the luminance to the nearest step
	// finds the candidate without walking all twenty-four.
	lum := (int(c.R) + int(c.G) + int(c.B)) / 3
	step := clampStep((lum - 8 + 5) / 10)

	grey := 8 + step*10
	if d := distance(c, RGB{grey, grey, grey}); d < bestDist {
		return 232 + step
	}
	return best
}

// Dark weights perceived channel brightness so theme contrast does not treat
// saturated blue and saturated green as equally bright.
func (c RGB) Dark() bool {
	// The weights sum to a thousand, so the sum is a thousand times a value in
	// 0–255 and the middle of that range is 128 thousand. Kept in integers because
	// the answer is a threshold, and a threshold does not need the fraction.
	return 299*int(c.R)+587*int(c.G)+114*int(c.B) < 128_000
}

func (c RGB) Index16() uint8 {
	best, bestDist := uint8(0), distance(c, ansi16[0])
	for i := 1; i < len(ansi16); i++ {
		if d := distance(c, ansi16[i]); d < bestDist {
			best, bestDist = uint8(i), d
		}
	}
	return best
}
