package term

import (
	"os"
	"strings"

	"github.com/Tangerg/oolong/core/graphics"
	"github.com/Tangerg/oolong/core/grid"
)

// DetectDepth works out how much colour this terminal can show, from the
// environment it was started in.
//
// This is the one place the library detects rather than asks. Everything else a
// terminal might not support is requested and ignored if unimplemented, which
// costs nothing when the guess is wrong. Colour is not like that: a truecolor
// sequence sent to a terminal that cannot read it does not degrade, it prints
// wrong, and there is no request that fails safely.
//
// What it reads, in the order it reads it:
//
//   - NO_COLOR, set to anything at all, means no colour. That is the whole of the
//     convention, including the part where an empty value still counts.
//   - COLORTERM naming truecolor or 24-bit means truecolor. Terminals that mean it
//     say so here.
//   - TERM of "dumb", or no TERM at all, means no colour.
//   - TERM mentioning 256 means the 256-colour palette.
//   - Anything else is truecolor.
//
// That last line is a decision worth stating. Plenty of terminals handle 24-bit
// colour and describe themselves as plain "xterm", so treating an unrecognised
// TERM as sixteen colours would make the common case worse to fix the rare one. A
// caller who knows better says so, and [program.Config] carries that through.
func DetectDepth() grid.Depth {
	if _, set := os.LookupEnv("NO_COLOR"); set {
		return grid.NoColor
	}
	switch strings.ToLower(os.Getenv("COLORTERM")) {
	case "truecolor", "24bit":
		return grid.TrueColor
	}
	term := strings.ToLower(os.Getenv("TERM"))
	switch {
	case term == "" || term == "dumb":
		return grid.NoColor
	case strings.Contains(term, "256"):
		return grid.Depth256
	default:
		return grid.TrueColor
	}
}

// DetectGraphics works out whether this terminal can show inline images.
//
// It is the same bargain as [DetectDepth] and the same reason for living here:
// [graphics.DetectIn] is a function of an environment, and this is the package
// allowed to have one.
func DetectGraphics() graphics.Protocol { return graphics.DetectIn(os.Getenv) }
