//go:build !darwin && !linux

package term_test

import "errors"

// ptyName has no answer where there is no pty; the tests that need one skip.
func ptyName(int) (string, error) { return "", errors.New("no pty on this platform") }
