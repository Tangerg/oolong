package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type diagramAction struct {
	label string
	key   rune
}

func diagramActions() [3]diagramAction {
	return [3]diagramAction{{"[Open Image (o)]", 'o'}, {"[Copy Image Path (p)]", 'p'}, {"[Copy Source (c)]", 'c'}}
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
