package main

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/examples/internal/fake"
)

func TestTheFormCollectsWhatItAsksFor(t *testing.T) {
	var got answers
	form := ask(&got)

	host := fake.New(t, 50, 20)
	done := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: host,
			Root: func(loop program.Loop) program.Component { return dress(loop, form) },
		})
	}()

	host.Shows(t, "New session")
	host.Type("nightly")
	host.Press(input.Tab)
	host.Press(input.Down) // balanced
	host.Press(input.Tab)
	host.Type(" ") // read
	host.Press(input.Tab)
	host.Press(input.Left) // yes
	host.Press(input.Enter)

	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
	if got.name != "nightly" || got.model != "balanced" || !got.stream {
		t.Fatalf("collected %+v", got)
	}
	if len(got.tools) != 1 || got.tools[0] != "read" {
		t.Fatalf("collected tools %v", got.tools)
	}
}

func TestTheSameFormCanBeAnsweredInWords(t *testing.T) {
	// What happens when the output is a pipe. The questions are the fields' own, so
	// there is no second description of the form anywhere.
	var got answers
	var out strings.Builder
	said := strings.NewReader("nightly\ncareful\n1,3\nn\n")
	if err := askInWords(ask(&got), said, &out); err != nil {
		t.Fatalf("answering in words: %v", err)
	}
	if got.name != "nightly" || got.model != "careful" || got.stream {
		t.Fatalf("collected %+v", got)
	}
	if len(got.tools) != 2 {
		t.Fatalf("collected tools %v", got.tools)
	}
	if !strings.Contains(out.String(), "Which model?") {
		t.Fatalf("the conversation was %q", out.String())
	}
}
