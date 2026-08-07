package headless_test

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

// show draws a widget and prints what it came to, one row per line, so an example can
// say what an interface looks like rather than describe it.
func show(w, h int, draw func(grid.View)) {
	for _, row := range paint(w, h, draw) {
		fmt.Printf("|%s|\n", strings.ReplaceAll(row, ".", " "))
	}
}

func showWidget(w, h int, widget headless.Widget) {
	show(w, h, headless.NewRoot(widget).Draw)
}

func ExampleForm() {
	// A field does not own what it collects: these three variables do, and the form
	// writes into them as the answers are given.
	var (
		name  string
		model string
		sure  bool
	)
	modelField := &headless.Select[string]{Label: "Model", Value: headless.Bind(&model)}
	modelField.SetOptions(headless.Options("fast", "good"))
	form := &headless.Form{
		Look: headless.Look{Taken: "x", Free: "-"},
		Fields: []headless.Field{
			&headless.Text{
				Label: "Name",
				Value: headless.Bind(&name),
				Check: func(s string) error {
					if s == "" {
						return errors.New("a name is needed")
					}
					return nil
				},
			},
			modelField,
			&headless.Confirm{Label: "Sure?", Value: headless.Bind(&sure)},
		},
		Done: func() { fmt.Println("collected:", name, model, sure) },
	}

	for _, r := range "ada" {
		form.Handle(input.Key{Code: input.Character, Rune: r})
	}
	form.Handle(input.Key{Code: input.Tab})   // on to the model
	form.Handle(input.Key{Code: input.Down})  // which is the second one
	form.Handle(input.Key{Code: input.Tab})   // on to the question
	form.Handle(input.Key{Code: input.Left})  // which is yes
	form.Handle(input.Key{Code: input.Enter}) // done

	showWidget(16, form.Measure(16), form)

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
	// Content is drawn at its whole height into a view that begins above the box, so
	// the rows off the top fall away and nothing has to be told it is scrolled.
	window := &headless.Viewport{Content: numbered(8)}
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

// numbered is content that writes its own row number.
func numbered(rows int) headless.Sized { return &tall{rows: rows} }
