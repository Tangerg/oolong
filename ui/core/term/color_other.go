//go:build !windows

package term

import "github.com/Tangerg/oolong/ui/core/grid"

// localDepth is the colour depth of the terminal this process opened, which its
// environment describes.
func localDepth(lookup func(string) (string, bool)) grid.Depth { return DetectDepth(lookup) }
