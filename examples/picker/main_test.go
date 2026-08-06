package main

import (
	"testing"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/examples/internal/fake"
)

func TestTypingNarrowsAndEnterPicks(t *testing.T) {
	chosen := ""
	host := fake.New(t, 60, 12)
	done := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: host,
			Root: func(runtime *program.Runtime) program.Component {
				return newPicker(runtime, files(), &chosen)
			},
		})
	}()

	host.Shows(t, "type to narrow")
	host.Shows(t, "25 of 25")

	host.Type("termwri")
	host.Shows(t, "1 of 25")
	// "ter.go" and not "writer.go": the characters that answered the query are drawn
	// in another style, so the row reaches the terminal in pieces. That is the
	// feature, and a test that asserted on the whole word would be asserting that it
	// is not there.
	host.Shows(t, "ter.go")

	host.Press(input.Enter)
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
	if chosen != "core/term/writer.go" {
		t.Fatalf("picked %q", chosen)
	}
}
