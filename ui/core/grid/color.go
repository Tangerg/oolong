package grid

import (
	"errors"
	"fmt"
	"math"
)

type RGB struct{ R, G, B uint8 }

// Blend resolves opacity before output because terminal cells have no alpha
// channel. Ordered opacity values clamp to [0,1]; NaN panics before drawing.
func (c RGB) Blend(over RGB, opacity float64) RGB {
	opacity = normalizedOpacity(opacity)
	lerp := func(a, b uint8) uint8 {
		return uint8(math.Round(float64(a) + (float64(b)-float64(a))*opacity))
	}
	return RGB{lerp(c.R, over.R), lerp(c.G, over.G), lerp(c.B, over.B)}
}

func normalizedOpacity(opacity float64) float64 {
	if math.IsNaN(opacity) {
		panic("grid: opacity is NaN")
	}
	return min(max(opacity, 0), 1)
}

// Color is a cell colour: the terminal's own default, an entry of the terminal's
// palette, or a truecolor value. A palette entry crosses the wire as its index, so
// the user's terminal theme decides how it looks. The zero Color is the default,
// which is what an unstyled cell wants.
type Color struct {
	kind  colorKind
	index uint8
	rgb   RGB
}

type colorKind uint8

const (
	defaultColor colorKind = iota
	paletteColor
	rgbColor
)

func RGBColor(r, g, b uint8) Color { return Color{kind: rgbColor, rgb: RGB{r, g, b}} }

// PaletteColor is the terminal's palette entry at index, numbered as in the xterm
// 256-colour palette: the sixteen ANSI colours first.
func PaletteColor(index uint8) Color { return Color{kind: paletteColor, index: index} }

func (c Color) Default() bool { return c.kind == defaultColor }

// Palette reports the palette index of a palette colour.
func (c Color) Palette() (index uint8, ok bool) { return c.index, c.kind == paletteColor }

// RGB returns the colour's components: exact for a truecolor value, and for a
// palette entry only xterm's estimate of it, since the terminal's theme decides.
// They are meaningless when the colour is the terminal default.
func (c Color) RGB() RGB {
	if c.kind == paletteColor {
		return PaletteRGB(c.index)
	}
	return c.rgb
}

// Blend cannot resolve a terminal-default color to a number, so it leaves c
// unchanged if either input is unresolved. Ground supplies queried RGB values
// when available; guessing a background could make text disappear.
// NaN opacity panics even when an input color is unresolved.
func (c Color) Blend(over Color, opacity float64) Color {
	opacity = normalizedOpacity(opacity)
	if c.Default() || over.Default() {
		return c
	}
	blended := c.RGB().Blend(over.RGB(), opacity)
	return RGBColor(blended.R, blended.G, blended.B)
}

// Depth is applied only when cells cross the wire, so themes and drawing
// operations remain independent of terminal capabilities. A truecolor value is
// approximated to fit it; a palette entry keeps its index wherever the depth has
// one. Auto delegates selection to the host; encoding treats unresolved Auto as
// TrueColor.
type Depth uint8

const (
	// Auto is the zero value: whatever the caller decides, and truecolor to
	// anything that has to draw before they have.
	Auto Depth = iota
	TrueColor
	Depth256
	Depth16
	// NoColor retains text attributes so emphasis survives without color.
	NoColor
)

var ErrInvalidDepth = errors.New("grid: invalid color depth")

func (d Depth) Validate() error {
	if d > NoColor {
		return fmt.Errorf("%w: %d", ErrInvalidDepth, d)
	}
	return nil
}

// cube holds the six channel values of the 6×6×6 colour cube that occupies
// indices 16–231 of the xterm palette.
var cube = [...]uint8{0, 95, 135, 175, 215, 255}

// ANSI palette entries are themeable; these xterm defaults support estimates,
// not a promise about the terminal's actual colors.
var ansi16 = [...]RGB{
	{0, 0, 0},
	{128, 0, 0},
	{0, 128, 0},
	{128, 128, 0},
	{0, 0, 128},
	{128, 0, 128},
	{0, 128, 128},
	{192, 192, 192},
	{128, 128, 128},
	{255, 0, 0},
	{0, 255, 0},
	{255, 255, 0},
	{0, 0, 255},
	{255, 0, 255},
	{0, 255, 255},
	{255, 255, 255},
}

const (
	paletteSize     = 256
	cubeStart       = uint8(len(ansi16))
	cubeRowStride   = uint8(len(cube))
	cubePlaneStride = cubeRowStride * cubeRowStride
	grayRampStart   = cubeStart + cubePlaneStride*cubeRowStride
	grayRampFirst   = 8
	grayRampStep    = 10
	grayRampLast    = paletteSize - int(grayRampStart) - 1
)

// PaletteRGB is what the xterm 256-colour palette holds at an index.
//
// The three regions are the sixteen ANSI colours, the 6×6×6 cube, and a 24-step
// grey ramp. Terminals may render the first sixteen however they like, so those
// values are what xterm uses and not a promise.
func PaletteRGB(index uint8) RGB {
	switch {
	case index < cubeStart:
		return ansi16[index]
	case index < grayRampStart:
		n := index - cubeStart
		return RGB{cube[n/cubePlaneStride], cube[(n%cubePlaneStride)/cubeRowStride], cube[n%cubeRowStride]}
	default:
		v := grayRampFirst + (index-grayRampStart)*grayRampStep
		return RGB{v, v, v}
	}
}

func nearestCube(v uint8) uint8 {
	best, bestDist := 0, diff(v, cube[0])
	for i := 1; i < len(cube); i++ {
		if d := diff(v, cube[i]); d < bestDist {
			best, bestDist = i, d
		}
	}
	return uint8(best)
}

// Guard narrowing explicitly so readers and static analyzers can prove the ramp index fits.
func clampStep(n int) uint8 {
	switch {
	case n <= 0:
		return 0
	case n >= grayRampLast:
		return uint8(grayRampLast)
	default:
		return uint8(n)
	}
}

func diff(a, b uint8) int {
	if a > b {
		return int(a) - int(b)
	}
	return int(b) - int(a)
}

// distance is the squared euclidean distance between two colours. Squared because
// nothing here needs the real distance, only which of two is smaller.
func distance(a, b RGB) int {
	dr, dg, db := diff(a.R, b.R), diff(a.G, b.G), diff(a.B, b.B)
	return dr*dr + dg*dg + db*db
}

// Index256 compares the color cube and grayscale ramp so near-gray colors
// keep their luminance. Themeable ANSI entries are excluded: their nominal RGB
// values do not predict what the destination will display.
func (c RGB) Index256() uint8 {
	// Palette arithmetic fits uint8: cube entries end at 231 and the ramp
	// ends at 255, so no narrowing can wrap a selected index.
	r, g, b := nearestCube(c.R), nearestCube(c.G), nearestCube(c.B)
	best := cubeStart + cubePlaneStride*r + cubeRowStride*g + b
	bestDist := distance(c, RGB{cube[r], cube[g], cube[b]})

	// The ramp runs 8, 18, 28 … 238. Rounding the luminance to the nearest step
	// finds the candidate without walking all twenty-four.
	lum := (int(c.R) + int(c.G) + int(c.B)) / 3
	step := clampStep((lum - grayRampFirst + grayRampStep/2) / grayRampStep)

	grey := grayRampFirst + step*grayRampStep
	if d := distance(c, RGB{grey, grey, grey}); d < bestDist {
		return grayRampStart + step
	}
	return best
}

// Dark weights perceived channel brightness so theme contrast does not treat
// saturated blue and saturated green as equally bright.
func (c RGB) Dark() bool {
	// Kept in integers because the answer is a threshold, and a threshold does not
	// need the fraction.
	return lumaRed*int(c.R)+lumaGreen*int(c.G)+lumaBlue*int(c.B) < midBrightness*(lumaRed+lumaGreen+lumaBlue)
}

// Rec. 601 luma weights, in thousandths, and the middle of a channel's range.
const (
	lumaRed       = 299
	lumaGreen     = 587
	lumaBlue      = 114
	midBrightness = (math.MaxUint8 + 1) / 2
)

func (c RGB) Index16() uint8 {
	best, bestDist := uint8(0), distance(c, ansi16[0])
	for i := 1; i < len(ansi16); i++ {
		if d := distance(c, ansi16[i]); d < bestDist {
			best, bestDist = uint8(i), d
		}
	}
	return best
}
