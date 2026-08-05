package kit_test

import (
	"fmt"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
)

// show draws something into a surface and prints it, so an example can state what
// appears rather than what was called.
func show(w, h int, draw func(grid.View)) {
	s := grid.NewSurface(w, h)
	draw(s.View())
	for y := range h {
		line := ""
		for x := range w {
			c := s.CellAt(x, y)
			switch {
			case c.Width() == 0:
			case c.Content == "":
				line += " "
			default:
				line += c.Content
			}
		}
		fmt.Printf("|%s|\n", line)
	}
}

func ExampleComposer() {
	c := kit.Composer{
		Prompt:      "› ",
		Placeholder: "Ask something",
		Hints:       []headless.Binding{{Key: input.Key{Code: input.Enter}, Does: "send"}},
	}
	show(28, c.Measure(28), c.Draw)

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
	fmt.Println(c.Text())
	show(12, 1, c.Draw)

	// Output:
	// hello
	// |› hello     |
}

func ExampleMessage() {
	m := kit.Message{Speaker: "you", Body: "what is this"}
	show(20, m.Measure(20), m.Draw)

	// Output:
	// |you                 |
	// |  what is this      |
	// |                    |
}

func ExampleBox() {
	inner := kit.Box{Border: kit.Rounded, Title: "Plan"}
	show(14, 3, func(v grid.View) {
		kit.Label{Text: "step one"}.Draw(inner.Draw(v))
	})

	// Output:
	// |╭─Plan───────╮|
	// |│step one    │|
	// |╰────────────╯|
}
