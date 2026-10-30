//go:build !unix && !windows

package term

import "fmt"

// relaunch has nothing to do here. Replacing a process and starting one beside it
// are both unavailable, and inventing something that half works would hide that
// from a caller who could have said so to the user.
func relaunch(path string, _, _ []string) (int, error) {
	return 0, fmt.Errorf("%w: cannot start %s again on this platform", ErrRelaunch, path)
}
