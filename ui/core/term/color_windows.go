//go:build windows

package term

import "github.com/Tangerg/oolong/ui/core/grid"

// localDepth is the colour depth of the console this process opened. A Windows
// console sets no TERM, yet the virtual terminal processing this package enables
// interprets 256-colour and truecolor SGR, so an unset TERM there is not a dumb
// terminal. A TERM that is set, as under mintty or MSYS, still decides.
func localDepth(lookup func(string) (string, bool)) grid.Depth {
	if lookup == nil {
		return DetectDepth(lookup)
	}
	if value, set := lookup("NO_COLOR"); set && value != "" {
		return grid.NoColor
	}
	if termName, _ := lookup("TERM"); termName == "" {
		return grid.TrueColor
	}
	return DetectDepth(lookup)
}
