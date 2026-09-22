package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

type diagramAction struct {
	label string
	key   rune
}

func diagramActions() [3]diagramAction {
	return [3]diagramAction{{"[Open Image (o)]", 'o'}, {"[Copy Image Path (p)]", 'p'}, {"[Copy Source (c)]", 'c'}}
}

func (s *diagramScreen) drawActions(view grid.View) {
	x := 0
	for _, action := range diagramActions() {
		kit.Label{Text: action.label, Style: s.theme.Accent}.Draw(view.Sub(grid.Rect(x, 0, text.Width(action.label), 1)))
		x += text.Width(action.label) + 2
	}
}

func (s *diagramScreen) clickAction(x int) bool {
	offset := 0
	for _, action := range diagramActions() {
		width := text.Width(action.label)
		if x >= offset && x < offset+width {
			return s.act(action.key)
		}
		offset += width + 2
	}
	return false
}

func (s *diagramScreen) act(key rune) bool {
	switch key {
	case 'c':
		if !s.runtime.Clipboard().Copy(s.source) {
			s.notice = "Clipboard unavailable"
		} else {
			s.notice = "Source copied"
		}
	case 'o', 'p':
		if s.prepared == nil {
			s.notice = "Image is not ready"
			return true
		}
		if s.exported == "" {
			path, err := exportPNG(s.prepared.PNG())
			if err != nil {
				s.notice = err.Error()
				return true
			}
			s.exported = path
		}
		if key == 'p' {
			if !s.runtime.Clipboard().Copy(s.exported) {
				s.notice = "Clipboard unavailable: " + s.exported
			} else {
				s.notice = "Image path copied: " + s.exported
			}
			return true
		}
		// Opening is an explicit user action. It does not block drawing or hold the
		// terminal open while the external viewer runs.
		dispatch, path, generation, open := s.runtime.Dispatcher(), s.exported, s.generation, s.open
		s.workers.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := open(ctx, path)
			dispatch.Post(func() {
				if s.closed || generation != s.generation {
					return
				}
				if err != nil {
					s.notice = err.Error()
				} else {
					s.notice = "Opened image: " + path
				}
			})
		})
	default:
		return false
	}
	return true
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
