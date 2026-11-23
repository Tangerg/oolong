package term

import (
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/termenv"
)

// DetectDepth uses the driven terminal's environment, which may belong to a
// remote PTY rather than this process. Nonempty NO_COLOR has priority. Unknown
// terminals default to truecolor because capable peers often advertise only xterm.
func DetectDepth(env termenv.Lookup) grid.Depth {
	if env.Set("NO_COLOR") {
		return grid.NoColor
	}
	if env.Is("COLORTERM", "truecolor") || env.Is("COLORTERM", "24bit") {
		return grid.TrueColor
	}
	switch termName := env.Term(); {
	case termName == "" || termName == "dumb":
		return grid.NoColor
	case strings.Contains(termName, "256"):
		return grid.Depth256
	default:
		return grid.TrueColor
	}
}
