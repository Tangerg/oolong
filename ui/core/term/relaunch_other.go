//go:build !unix && !windows

package term

import "fmt"

// Refuse unsupported process replacement instead of leaving a partial relaunch
// that could compete with its parent for terminal ownership.
func relaunch(path string, _, _ []string) (int, error) {
	return 0, fmt.Errorf("%w: cannot start %s again on this platform", ErrRelaunch, path)
}
