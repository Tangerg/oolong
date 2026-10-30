package base_test

import (
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func show(w, h int, draw func(grid.View)) {
	for _, row := range paint(w, h, draw) {
		fmt.Printf("|%s|\n", strings.ReplaceAll(row, ".", " "))
	}
}
func showWidget(w, h int, widget any) { show(w, h, newTestRoot(widget).Draw) }
func ExampleForm() {
	name := base.NewText(base.TextConfig{Label: "Name"})
	model := base.NewSelect(base.SelectConfig[string]{Same: base.Equal[string], Label: "Model", Options: base.Options("fast", "good")})
	sure := base.NewConfirm(base.ConfirmConfig{Label: "Sure?"})
	form := base.NewForm(base.FormConfig{Fields: []base.Field{name, model, sure}, Submit: func(id base.SubmissionID) (scene.ActionResult[base.SubmissionID], error) {
		fmt.Println("collected:", name.Text(), chosenValue(model), sure.Answer())
		return scene.ActionResult[base.SubmissionID]{State: scene.Applied, Value: id}, nil
	}})
	styleFields(form, base.Look{Taken: "x", Free: "-"})
	root := newTestRoot(form)
	for _, r := range "ada" {
		root.Handle(input.Key{Code: input.Character, Rune: r})
	}
	root.Handle(input.Key{Code: input.Tab})
	root.Handle(input.Key{Code: input.Down})
	root.Handle(input.Key{Code: input.Tab})
	root.Handle(input.Key{Code: input.Left})
	root.Handle(input.Key{Code: input.Enter})
	showWidget(16, form.HeightForWidth(16), form)
	// Output:
	// collected: ada good true
	// |Name            |
	// |ada             |
	// |Model           |
	// |- fast          |
	// |x good          |
	// |Sure?           |
	// |x yes  - no     |
}

func ExampleViewport() {
	window := base.NewViewport(base.ViewportConfig{Content: testElement(&tall{rows: 8})})
	showWidget(8, 3, window)
	window.Scroll().By(4)
	showWidget(8, 3, window)
	// Output:
	// |row 0   |
	// |row 1   |
	// |row 2   |
	// |row 4   |
	// |row 5   |
	// |row 6   |
}
