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
// caller that knows better can use its own answer instead.
func DetectDepth() grid.Depth {
	return DetectDepthIn(os.LookupEnv)
}

// DetectDepthIn applies the same colour decision to an explicit environment.
// It is the form for adapters whose terminal is not the process terminal — most
// notably an SSH session. lookup follows [os.LookupEnv] so an empty NO_COLOR value
// remains distinguishable from an absent one.
func DetectDepthIn(lookup func(string) (string, bool)) grid.Depth {
	if lookup == nil {
		return grid.NoColor
	}
	if _, set := lookup("NO_COLOR"); set {
		return grid.NoColor
	}
	colorTerm, _ := lookup("COLORTERM")
	switch strings.ToLower(colorTerm) {
	case "truecolor", "24bit":
		return grid.TrueColor
	}
	termName, _ := lookup("TERM")
	termName = strings.ToLower(termName)
	switch {
	case termName == "" || termName == "dumb":
		return grid.NoColor
	case strings.Contains(termName, "256"):
		return grid.Depth256
	default:
		return grid.TrueColor
	}
}

// sixelAttribute is the extension number a terminal claims when it can draw sixel
// graphics. It is claimed in the device attributes and nowhere else.
const sixelAttribute = 4

// DetectGraphics works out whether this terminal can show inline images, from the
// environment alone.
//
// It is the same bargain as [DetectDepth] and the same reason for living here:
// [graphics.DetectIn] is a function of an environment, and this is the package
// allowed to have one.
//
// A terminal that draws sixel and nothing else will come back as [graphics.None]:
// no environment variable names sixel, so the only way to learn about it is to ask,
// and asking needs a terminal. [Terminal.Graphics] is that answer; this is the one
// available to code holding no terminal at all.
func DetectGraphics() graphics.Protocol { return graphics.DetectIn(os.LookupEnv, "", false) }
