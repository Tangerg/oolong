package main

import (
	"os"
	"syscall"
)

func openPreview(path string) (*os.File, error) {
	// Inspect the reparse point itself so a replacement link cannot redirect the read.
	return os.OpenFile(path, os.O_RDONLY|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0) //nolint:gosec // The file browser opens the selected path and checks the resulting handle.
}
