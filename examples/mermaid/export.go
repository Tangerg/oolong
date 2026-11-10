package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const viewerTimeout = 5 * time.Second

func (s *diagramScreen) export() (string, bool) {
	d := &s.current
	if d.image == nil {
		d.notice = "Image is not ready"
		return "", false
	}
	if d.exported == "" {
		path, err := exportPNG(d.image.PNG())
		if err != nil {
			d.notice = err.Error()
			return "", false
		}
		d.exported = path
	}
	return d.exported, true
}

// openExported hands the file to the system viewer off the UI goroutine, so
// drawing is not blocked while the viewer starts. The viewer runs in the
// diagram's scope: a replacement revokes its notice, which would otherwise
// describe a diagram no longer on screen.
func (s *diagramScreen) openExported(path string) {
	open := s.open
	err := s.start(func(ctx context.Context) (struct{}, error) {
		ctx, cancel := context.WithTimeout(ctx, viewerTimeout)
		defer cancel()
		return struct{}{}, open(ctx, path)
	}, func(_ struct{}, err error) {
		if err != nil {
			s.current.notice = err.Error()
		} else {
			s.current.notice = "Opened image: " + path
		}
	})
	if err != nil {
		s.current.notice = err.Error()
	}
}

// Exports are created only for an explicit open/copy-path action. They survive
// replacement and shutdown so a copied path continues to name the same image.
func exportPNG(data []byte) (_ string, err error) {
	file, err := os.CreateTemp("", "oolong-diagram-*.png")
	if err != nil {
		return "", err
	}
	path := file.Name()
	defer func() {
		if err != nil {
			err = errors.Join(err, os.Remove(path))
		}
	}()
	_, writeErr := file.Write(data)
	if err = errors.Join(writeErr, file.Close()); err != nil {
		return "", fmt.Errorf("export diagram: %w", err)
	}
	return filepath.Abs(path)
}
