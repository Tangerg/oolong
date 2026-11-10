package main

import (
	"image"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

const (
	openDiagram          keymap.Action = "open-image"
	copyDiagramPath      keymap.Action = "copy-image-path"
	copyDiagramSource    keymap.Action = "copy-source"
	replaceDiagramSource keymap.Action = "replace-source"
	quitDiagram          keymap.Action = "quit"

	toolbarGap = 2
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

var toolbarActions = [...]struct {
	label  string
	action keymap.Action
}{
	{"Open Image", openDiagram},
	{"Copy Image Path", copyDiagramPath},
	{"Copy Source", copyDiagramSource},
}

// toolbarButton is where one action was drawn in its frame.
type toolbarButton struct {
	action keymap.Action
	area   image.Rectangle
}

// drawToolbar paints one button per action on the first row of view, each
// hinting its current first binding, and returns where the buttons landed.
func drawToolbar(view grid.View, keys *keymap.Map, style grid.Style) []toolbarButton {
	buttons := make([]toolbarButton, 0, len(toolbarActions))
	x := 0
	for _, entry := range toolbarActions {
		label := entry.label
		if bound := keys.Keys(entry.action); len(bound) != 0 {
			label += " (" + bound[0].String() + ")"
		}
		label = "[" + label + "]"
		area := grid.Area(x, 0, text.Width(label), 1)
		kit.Label{Text: label, Style: style}.Draw(view.Sub(area))
		buttons = append(buttons, toolbarButton{action: entry.action, area: area})
		x = area.Max.X + toolbarGap
	}
	return buttons
}

func buttonAt(buttons []toolbarButton, at image.Point) (keymap.Action, bool) {
	for _, button := range buttons {
		if at.In(button.area) {
			return button.action, true
		}
	}
	return "", false
}
