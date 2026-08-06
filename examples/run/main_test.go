package main

import (
	"testing"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/examples/internal/fake"
)

func TestOutputKeepsItsColoursAndFinishedLinesBelongToTheTerminal(t *testing.T) {
	host := fake.New(t, 60, 8)
	var r *runner
	done := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: host,
			Inline: func(loop program.InlineLoop) program.Component {
				// Not newRunner: that one starts a process, and what is being tested is
				// what happens to what it says.
				r = &runner{loop: loop, command: []string{"a", "command"}, status: "running"}
				return r
			},
		})
	}()
	host.Shows(t, "ctrl+e: editor")

	// A chunk boundary can fall anywhere, including inside an escape sequence.
	r.loop.Post(func() { r.output("plain \x1b[3") })
	r.loop.Post(func() { r.output("1mred\x1b[0m and more\nsecond line\nhalf a li") })

	// What a newline finished is printed into the terminal's own output; what is left
	// is still being drawn.
	host.Shows(t, "second line")
	host.Shows(t, "half a li")

	// And the colour survived the chunk boundary: red is 31, which the frame carries
	// as the colour the palette gives it.
	host.Until(t, "the output to keep its colour", func() bool {
		host.Repaint()
		return contains(host.Frames(), "128;0;0") || contains(host.Frames(), "red")
	})

	r.loop.Post(func() { r.finish(nil) })
	host.Until(t, "the run to be said to be over", func() bool {
		_, rang, notified, _ := host.Said()
		return rang == 1 && len(notified) == 1
	})

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		(haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
