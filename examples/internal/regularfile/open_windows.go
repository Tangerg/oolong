package regularfile

import (
	"os"
	"syscall"
)

func open(path string) (*os.File, error) {
	// Inspect the reparse point itself so a replacement link cannot redirect the read.
	return os.OpenFile(path, os.O_RDONLY|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0) //nolint:gosec // Open checks the selected handle before reading its contents.
}
