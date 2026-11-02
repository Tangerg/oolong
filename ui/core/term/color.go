package term

import (
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
)

// DetectDepth uses the driven terminal's environment, which may belong to a
// remote PTY rather than this process. Nonempty NO_COLOR has priority. Unknown
// terminals default to truecolor because capable peers often advertise only xterm.
func DetectDepth(lookup func(string) (string, bool)) grid.Depth {
	if lookup == nil {
		return grid.NoColor
	}
	if value, set := lookup("NO_COLOR"); set && value != "" {
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

const sixelAttribute = 4
