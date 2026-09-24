// Package grid is the cell grid the whole terminal UI is drawn into: styled
// grapheme cells, a clipped drawing view over them, and the two ways a frame of
// them reaches a terminal.
//
// [Screen] takes the terminal's whole screen and emits the smallest escape stream
// that turns one frame into the next. [Inline] draws a block in the terminal's own
// screen instead, printing finished output above it into the scrollback. They share
// the cells, the view and the encoding, and differ only in what a frame is allowed
// to assume about where it is.
//
// It is the only layer that knows what a terminal is made of. Everything above
// it draws through [View] and never assembles an escape sequence.
//
// Geometry is [image.Rectangle] and [image.Point] from the standard library
// rather than a private rectangle type. Terminal rectangles are ordinary
// half-open rectangles, and intersection, insetting and containment are already
// written and already correct there.
package grid

import (
	"image"
	"math"

	"github.com/Tangerg/oolong/core/layout"
)

// Area builds a rectangle from a terminal-natural origin and size. The result is
// half-open: it covers columns [x, x+w) and rows [y, y+h). Negative sizes become
// zero and endpoints that exceed int range saturate.
//
// Not Rect, which is [image.Rect]'s name for the same four integers read as two
// corners. Two functions with one signature, one return type and two meanings is a
// mistake nothing catches: a caller reaching for the familiar one gets a rectangle
// that compiles, draws, and is the wrong shape.
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

// RGB is a 24-bit colour.
type RGB struct{ R, G, B uint8 }

// Blend mixes c toward over by opacity, clamped to [0,1].
//
// This is the whole of compositing in a terminal. There is no alpha channel on the
// wire — a cell holds one background and one foreground, and both are opaque — so a
// translucent layer has to be resolved to opaque colours before anything is written.
// Doing the mixing here, on two colours that are certainly numbers, is what keeps
// that resolution in one place.
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

// RGBColor returns a colour that overrides the terminal default.
func RGBColor(r, g, b uint8) Color { return Color{set: true, rgb: RGB{r, g, b}} }

// Default reports whether the colour defers to the terminal.
func (c Color) Default() bool { return !c.set }

// RGB returns the colour's components. They are meaningless when the colour is
// the terminal default.
func (c Color) RGB() RGB { return c.rgb }

// Blend mixes c toward over by opacity, clamped to [0,1].
//
// A colour that defers to the terminal is not a number, so a blend involving one
// cannot be computed and c is returned unchanged. That is the rule everywhere
// blending appears: what cannot be resolved is left alone, rather than guessed at.
// Guessing would tint an interface differently on every terminal, and be wrong in
// the direction that makes text vanish — a scrim assumed to be over black, painted
// over white, blacks out the screen.
//
// Turning a default into a number is [Ground]'s job, and doing it first is what
// makes a blend answerable. A frame drawn by a program has one, because the
// terminal was asked at startup.
func (c Color) Blend(over Color, opacity float64) Color {
	if c.Default() || over.Default() {
		return c
	}
	blended := c.rgb.Blend(over.rgb, opacity)
	return RGBColor(blended.R, blended.G, blended.B)
}

// Attr is a set of text attributes.
type Attr uint8

// The attributes a cell can carry. They are the ones every terminal implements
// and the ones a single SGR parameter turns on, which is why there are six.
const (
	Bold Attr = 1 << iota
	Dim
	Italic
	Underline
	Reverse
	Strike
)

// Has reports whether every attribute in want is set.
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

// Ground is what a terminal's own two colours actually are.
//
// Leaving a cell's colour at the default is the right way to store it: the user's
// own theme shows through, an unstyled cell costs nothing on the wire, and a
// terminal recoloured while a program is running follows along. The price is that
// "the terminal's own" is not a value, and anything that has to mix with what is
// underneath needs one. This is where the answer is kept, once the terminal has been
// asked through the terminal colour-query protocol.
//
// The zero value is two defaults, which is what a terminal that was not asked or
// did not answer leaves behind. Blending through it resolves nothing and changes
// nothing, which is the honest outcome: a scrim over an unknown background is a
// question with no answer, and the visible cost of skipping it — a layer that does
// not dim what it covers — is far smaller than the cost of guessing.
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

// Cell is one terminal cell.
//
// The zero Cell is a blank single-width cell in the terminal's own style, so a
// freshly allocated or cleared surface is already valid.
//
// A cell's content is read through [Cell.Content] rather than a writable field.
// Only drawing through a [View] can create content, which keeps its measured span
// and continuation cells inseparable. Style and Link remain writable on copied
// rows because changing appearance cannot invalidate that geometry.
type Cell struct {
	content string
	Style   Style
	// Link is an OSC 8 hyperlink target. It is cell metadata rather than part of
	// Style because a hyperlink has its own open/close protocol on the wire,
	// while everything in Style is one SGR parameter list.
	Link string

	span span
}

// Content returns the complete grapheme cluster stored on an atom's head. It is
// empty for a blank or continuation cell.
func (c Cell) Content() string { return c.content }

// Width is how many columns the cell occupies: the complete display width on an
// atom's head, zero on a continuation cell, and one otherwise.
func (c Cell) Width() int {
	if c.span < 0 {
		return 0
	}
	return max(int(c.span), 1)
}

// Blank reports whether the cell would print as empty space.
func (c Cell) Blank() bool { return c.content == "" && c.span >= 0 }

// Index256 is the nearest entry of the xterm 256-colour palette.
//
// Both the colour cube and the grey ramp are searched and the closer of the two
// wins. Searching only the cube would turn every near-grey into a muddy brown: the
// cube's greys are the six points where all three channels agree, and the ramp has
// twenty-four.
//
// The first sixteen indices are left out of the search on purpose. A terminal is
// free to render those as anything at all — a theme's own palette, usually — so
// choosing one because its default value happened to be close is choosing a colour
// nobody can predict.
func (c RGB) Index256() uint8 {
	// All of this is uint8 arithmetic on purpose. Each cube index is at most 5, so
	// the largest value reachable here is 16+36*5+6*5+5 = 231, and the largest grey
	// index is 232+23 = 255 — both inside the type the answer is returned in. Doing
	// the sums in a wider type and converting at the end would be the same numbers
	// with a conversion nobody can check by reading it.
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

// Dark reports whether this colour is dark enough that what goes on top of it
// should be light.
//
// That, rather than "is it dark" in the abstract, is the question a theme asks
// when it learns what the terminal draws on. The answer weights the channels by
// how much of brightness the eye takes from each — green far more than blue — and
// puts the line down the middle. An unweighted average would call a saturated
// blue light and a saturated green dark, and get both backwards.
func (c RGB) Dark() bool {
	// The weights sum to a thousand, so the sum is a thousand times a value in
	// 0–255 and the middle of that range is 128 thousand. Kept in integers because
	// the answer is a threshold, and a threshold does not need the fraction.
	return 299*int(c.R)+587*int(c.G)+114*int(c.B) < 128_000
}

// Index16 is the nearest of the sixteen colours every terminal has.
func (c RGB) Index16() uint8 {
	best, bestDist := uint8(0), distance(c, ansi16[0])
	for i := 1; i < len(ansi16); i++ {
		if d := distance(c, ansi16[i]); d < bestDist {
			best, bestDist = uint8(i), d
		}
	}
	return best
}
