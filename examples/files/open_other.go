//go:build !unix && !windows

package main

import (
	"errors"
	"os"
)

func openPreview(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("preview requires a regular file")
	}
	file, err := os.Open(path) //nolint:gosec // The selected file is verified before its contents can be read.
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err == nil && os.SameFile(info, opened) {
		return file, nil
	}
	if err == nil {
		err = errors.New("selected file changed while opening preview")
	}
	return nil, errors.Join(err, file.Close())
}
