package grid

// Depth is how much colour a terminal is being asked to show.
//
// A frame is always built in truecolor — a [Color] is either the terminal's
// default or a 24-bit value, and nothing above this package thinks about anything
// else. The depth is applied at the very last step, where a style becomes bytes,
// so a widget never has to know what it is drawing onto and a palette never has to
// be authored twice.
//
// The zero value is [Auto], which leaves the choice to whoever opened the
// terminal — this package cannot read an environment variable and has no business
// guessing. Everything here treats it as [TrueColor], which is the bet the library
// made before this type existed; the difference is that it is now a bet a caller
// can lose gracefully instead of one they cannot opt out of.
type Depth uint8

const (
	// Auto is the zero value: whatever the caller decides, and truecolor to
	// anything that has to draw before they have.
	Auto Depth = iota
	// TrueColor emits the 24-bit value unchanged.
	TrueColor
	// Depth256 maps each colour to the nearest entry of the xterm 256 palette.
	Depth256
	// Depth16 maps each colour to the nearest of the eight ANSI colours and their
	// bright forms — the only colours a terminal is really obliged to have.
	Depth16
	// NoColor drops colour entirely and keeps the attributes. It is what NO_COLOR
	// asks for, and what a terminal being logged to a file wants: bold and
	// underline still carry meaning in a transcript, and a colour does not.
	NoColor
)

// cube holds the six channel values of the 6×6×6 colour cube that occupies
// indices 16–231 of the xterm palette.
var cube = [6]uint8{0, 95, 135, 175, 215, 255}

// ansi16 are the sixteen colours every terminal has, in the order the SGR codes
// number them. The values are the common xterm defaults: a terminal is free to
// render its own, which is the point of using an index rather than a value.
var ansi16 = [16]RGB{
	{0, 0, 0}, {128, 0, 0}, {0, 128, 0}, {128, 128, 0},
	{0, 0, 128}, {128, 0, 128}, {0, 128, 128}, {192, 192, 192},
	{128, 128, 128}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0},
	{0, 0, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
}

// PaletteRGB is what the xterm 256-colour palette holds at an index.
//
// The three regions are the sixteen ANSI colours, the 6×6×6 cube, and a 24-step
// grey ramp. Terminals may render the first sixteen however they like, so those
// values are what xterm uses and not a promise.
func PaletteRGB(index uint8) RGB {
	switch {
	case index < 16:
		return ansi16[index]
	case index < 232:
		n := index - 16
		return RGB{cube[n/36], cube[(n%36)/6], cube[n%6]}
	default:
		v := 8 + (index-232)*10
		return RGB{v, v, v}
	}
}

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
	r, g, b := nearestCube(c.R), nearestCube(c.G), nearestCube(c.B)
	best := 16 + 36*uint16(r) + 6*uint16(g) + uint16(b)
	bestDist := distance(c, RGB{cube[r], cube[g], cube[b]})

	// The ramp runs 8, 18, 28 … 238. Rounding the luminance to the nearest step
	// finds the candidate without walking all twenty-four.
	lum := (int(c.R) + int(c.G) + int(c.B)) / 3
	step := clampStep((lum - 8 + 5) / 10)
	grey := uint8(8 + step*10)
	if d := distance(c, RGB{grey, grey, grey}); d < bestDist {
		return uint8(232 + step)
	}
	return uint8(best)
}

// Index16 is the nearest of the sixteen colours every terminal has.
func (c RGB) Index16() uint8 {
	best, bestDist := 0, distance(c, ansi16[0])
	for i := 1; i < len(ansi16); i++ {
		if d := distance(c, ansi16[i]); d < bestDist {
			best, bestDist = i, d
		}
	}
	return uint8(best)
}

// nearestCube is the index into [cube] whose value is closest to v.
func nearestCube(v uint8) uint8 {
	best, bestDist := 0, diff(v, cube[0])
	for i := 1; i < len(cube); i++ {
		if d := diff(v, cube[i]); d < bestDist {
			best, bestDist = i, d
		}
	}
	return uint8(best)
}

func clampStep(n int) int { return min(max(n, 0), 23) }

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
