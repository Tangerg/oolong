//go:build unix

package main

import (
	"os"
	"syscall"
)

func openPreview(path string) (*os.File, error) {
	// The selected path is intentionally user-controlled; only regular files are read.
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0) // #nosec G304 -- local file browser selection
}
