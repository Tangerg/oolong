//go:build !windows

package term

import (
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/termenv"
)

// localDepth is the colour depth of the terminal this process opened, which its
// environment describes.
func localDepth(env termenv.Lookup) grid.Depth { return DetectDepth(env) }
