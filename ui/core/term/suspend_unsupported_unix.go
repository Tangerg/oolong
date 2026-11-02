//go:build unix && !linux && !darwin && !freebsd

package term

import "errors"

func suspendProcess() error {
	// Without a synchronous delivery boundary, Hand could reclaim the terminal
	// before a queued stop arrives. An unbounded SIGCONT wait is not a substitute.
	return errors.ErrUnsupported
}
