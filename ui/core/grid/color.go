package grid

// Depth is applied only when truecolor cells cross the wire so themes and
// drawing operations remain independent of terminal palettes. Auto delegates
// selection to the host; encoding treats unresolved Auto as TrueColor.
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

// cube holds the six channel values of the 6×6×6 colour cube that occupies
// indices 16–231 of the xterm palette.
var cube = [6]uint8{0, 95, 135, 175, 215, 255}

// ANSI palette entries are themeable; these xterm defaults support estimates,
// not a promise about the terminal's actual colors.
var ansi16 = [16]RGB{
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
	const lastStep = 23
	switch {
	case n <= 0:
		return 0
	case n >= lastStep:
		return lastStep
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
