package base_test

import (
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

func show(w, h int, draw func(grid.View)) {
	for _, row := range paint(w, h, draw) {
		fmt.Printf("|%s|\n", strings.ReplaceAll(row, ".", " "))
	}
}

func showWidget(scenes *scenetest.Scope, w, h int, widget any) { show(w, h, scenes.Mount(widget).Draw) }

func ExampleForm() {
	scenes := scenetest.New()
	defer scenes.Close()

	name := base.NewText(base.TextConfig{Label: "Name"})
	model := base.NewSelect(base.SelectConfig[string]{Same: base.Equal[string], Label: "Model", Options: base.Options("fast", "good")})
	sure := base.NewConfirm(base.ConfirmConfig{Label: "Sure?"})
	form := base.NewForm(base.FormConfig{Fields: []base.Field{name, model, sure}, Submit: func(base.SubmissionID) (scene.ActionState, error) {
		fmt.Println("collected:", name.Text(), chosenValue(model), sure.Answer())
		return scene.Applied, nil
	}})
	styleFields(form, base.Look{Taken: "x", Free: "-"})
	root := scenes.Mount(form)
	for _, r := range "ada" {
		root.Handle(input.Key{Code: input.Character, Rune: r})
	}
	root.Handle(input.Key{Code: input.Tab})
	root.Handle(input.Key{Code: input.Down})
	root.Handle(input.Key{Code: input.Tab})
	root.Handle(input.Key{Code: input.Left})
	root.Handle(input.Key{Code: input.Enter})
	showWidget(scenes, 16, form.HeightForWidth(16), form)
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
	scenes := scenetest.New()
	defer scenes.Close()

	window := base.NewViewport(base.ViewportConfig{Content: scenes.Element(&tall{rows: 8})})
	showWidget(scenes, 8, 3, window)
	window.Scroll().By(4)
	showWidget(scenes, 8, 3, window)
	// Output:
	// |row 0   |
	// |row 1   |
	// |row 2   |
	// |row 4   |
	// |row 5   |
	// |row 6   |
}
