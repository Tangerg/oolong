//go:build !unix

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
	return os.Open(path) //nolint:gosec // G304: explicitly selected local preview path; the caller validates the opened file and bounds reads.
}
