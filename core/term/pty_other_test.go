//go:build !darwin && !linux

package term_test

import (
	"errors"
	"os"
)

// openPTY has no answer where there is no pty, and the tests that need one skip.
func openPTY() (primary, replica *os.File, err error) {
	return nil, nil, errors.New("no pty on this platform")
}

func resizePTY(*os.File, int, int) error {
	return errors.New("no pty resize source on this platform")
}
