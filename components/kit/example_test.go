package kit_test

import (
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/diff"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
)

// show draws something into a surface and prints it, so an example can state what
// appears rather than what was called.
func show(w, h int, draw func(grid.View)) {
	s := grid.NewSurface(w, h)
	draw(s.View())
	for y := range h {
		var line strings.Builder
		for x := range w {
			c := cellAt(s, x, y)
			switch {
			case c.Width() == 0:
			case c.Content == "":
				line.WriteString(" ")
			default:
				line.WriteString(c.Content)
			}
		}
		fmt.Printf("|%s|\n", line.String())
	}
}

func showWidget(w, h int, widget headless.Widget) {
	show(w, h, headless.NewRoot(widget).Draw)
}

func ExampleComposer() {
	// One table for what the field does and what the program does. The hint row reads
	// the keystroke back out of it rather than being told it a second time.
	keys := headless.DefaultEditorKeys()
	keys.Bind("send", input.Chord{Code: input.Enter})

	c := kit.Composer{
		Prompt: "› ",
		Hints:  []keymap.Action{"send"},
	}
	c.Editor().Placeholder = "Ask something"
	c.Editor().Keys = keys
	showWidget(28, c.Measure(28), &c)

	// Output:
	// |› Ask something             |
	// |enter send                  |
}

func ExampleComposer_typing() {
	var c kit.Composer
	c.Prompt = "› "
	for _, r := range "hello" {
		c.Handle(input.Key{Code: input.Character, Rune: r})
	}
	fmt.Println(c.Editor().Text())
	showWidget(12, 1, &c)

	// Output:
	// hello
	// |› hello     |
}

func ExampleEntry() {
	m := kit.Entry{Label: "build", Body: "what changed"}
	show(20, m.Measure(20), m.Draw)

	// Output:
	// |build               |
	// |  what changed      |
	// |                    |
}

func ExampleBox() {
	inner := kit.Box{Glyphs: kit.Unicode(), Title: "Plan"}
	show(14, 3, func(v grid.View) {
		kit.Label{Text: "step one"}.Draw(inner.Draw(v))
	})

	// Output:
	// |╭─Plan───────╮|
	// |│step one    │|
	// |╰────────────╯|
}

func ExampleForm() {
	// A theme becomes the handful of roles a field draws itself in, and a glyph set
	// becomes the marks beside a choice. That is the whole of dressing a form.
	var name, model string
	modelField := &headless.Select[string]{Label: "Model", Value: headless.Bind(&model)}
	modelField.SetOptions(headless.Options("fast", "good"))
	form := headless.NewForm(
		&headless.Text{Label: "Name", Value: headless.Bind(&name), Placeholder: "who?"},
		modelField,
	)
	keys := headless.DefaultFormKeys()
	form.Keys = keys
	view := kit.NewForm(kit.FormConfig{
		Theme: kit.Dark(), Glyphs: kit.ASCII(), Controller: form, Title: "New session",
		Hints: []keymap.Action{headless.Submit},
	})
	showWidget(22, view.Measure(22), view)

	// Output:
	// |New session           |
	// |Name                  |
	// |who?                  |
	// |Model                 |
	// |x fast                |
	// |- good                |
	// |enter submit          |
}

func ExampleProgress() {
	// Work with a total gets a bar; work without one gets a Spinner. The cell the bar
	// ends in is drawn as a fraction of itself, so it moves by eighths of a column
	// rather than in jumps of a whole one.
	p := kit.Progress{
		Glyphs:  kit.Unicode(),
		Label:   "fetching",
		Done:    3,
		Total:   8,
		Percent: true,
	}
	show(28, p.Measure(28), p.Draw)

	// Output:
	// |fetching █████▎░░░░░░░░  38%|
}

func ExampleNewTabs_controlled() {
	selected := 1
	tabs := kit.NewTabs(kit.TabsConfig{
		Glyphs: kit.ASCII(), Selection: headless.Bind(&selected),
		Items: []headless.Tab{{Title: "one"}, {Title: "two"}},
	})
	fmt.Println(tabs.Controller().Selected())
	tabs.Controller().Select(0)
	fmt.Println(selected)
	selected = 1
	tabs.Controller().Sync()
	fmt.Println(tabs.Controller().Selected())

	// Output:
	// 1
	// 0
	// 1
}

func ExampleNewSlider_controlled() {
	value := 2
	slider := kit.NewSlider(kit.SliderConfig{
		Glyphs: kit.ASCII(), Value: headless.Bind(&value),
		Minimum: 1, Maximum: 8, Label: "workers",
	})
	slider.Controller().Move(2)
	fmt.Println(value)

	value = 8
	slider.Controller().Sync()
	fmt.Println(slider.Controller().Value())

	// Output:
	// 4
	// 8
}

func ExampleNewDialog_controlled() {
	var open bool
	dialog := kit.NewDialog(kit.DialogConfig{
		Stack: &headless.Stack{}, Open: headless.Bind(&open), Glyphs: kit.ASCII(),
		Title: "Confirm", Body: headless.Static{Of: kit.Label{Text: "Continue?"}},
	})
	dialog.Controller().Show()
	fmt.Println(open)

	open = false
	dialog.Controller().Sync()
	fmt.Println(dialog.Controller().Open())

	// Output:
	// true
	// false
}

func ExampleDiff() {
	before := strings.Split("keep\nold\nkeep", "\n")
	after := strings.Split("keep\nnew\nkeep", "\n")
	view := kit.NewDiff(kit.DiffConfig{
		Theme: kit.Dark(), Glyphs: kit.ASCII(),
		Hunks: diff.Between(before, after).Hunks(1), Numbers: true,
	})
	show(16, view.Measure(16), view.Draw)

	// Output:
	// |1 1  keep       |
	// |2   -old        |
	// |  2 +new        |
	// |3 3  keep       |
}
