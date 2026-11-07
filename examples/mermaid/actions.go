package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
)

type diagramAction struct {
	label  string
	action keymap.Action
}

func diagramActions() [3]diagramAction {
	return [3]diagramAction{{"Open Image", openDiagram}, {"Copy Image Path", copyDiagramPath}, {"Copy Source", copyDiagramSource}}
}

const (
	openDiagram          keymap.Action = "open-image"
	copyDiagramPath      keymap.Action = "copy-image-path"
	copyDiagramSource    keymap.Action = "copy-source"
	replaceDiagramSource keymap.Action = "replace-source"
	quitDiagram          keymap.Action = "quit"
)

func diagramKeys() *keymap.Map {
	keys := new(keymap.Map)
	keys.Bind(openDiagram, input.Chord{Rune: 'o'})
	keys.Bind(copyDiagramPath, input.Chord{Rune: 'p'})
	keys.Bind(copyDiagramSource, input.Chord{Rune: 'c'})
	keys.Bind(replaceDiagramSource, input.Chord{Rune: 'r'})
	keys.Bind(quitDiagram, input.Chord{Rune: 'q'})
	keys.Bind(quitDiagram, input.Ctrl.Rune('c'))
	return keys
}

func (a diagramAction) text(keys *keymap.Map) string {
	label := a.label
	if bound := keys.Keys(a.action); len(bound) != 0 {
		label += " (" + bound[0].String() + ")"
	}
	return "[" + label + "]"
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
