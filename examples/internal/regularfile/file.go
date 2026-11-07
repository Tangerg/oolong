// Package regularfile prevents local previews from following a selected link or
// waiting for a pipe writer before the regular-file boundary can reject it.
package regularfile

import (
	"errors"
	"os"
)

// Open rejects a linked or nonregular final path component before exposing contents.
// Unix opens are nonblocking so a FIFO cannot wait for a writer at admission.
func Open(path string) (*os.File, error) {
	file, err := open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err == nil && info.Mode().IsRegular() {
		return file, nil
	}
	if err == nil {
		err = errors.New("selected path requires a regular file")
	}
	return nil, errors.Join(err, file.Close())
}
