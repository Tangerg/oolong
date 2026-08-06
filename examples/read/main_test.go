package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/examples/internal/fake"
)

func TestWhatIsFinishedIsPrintedAndWhatIsNotIsDrawn(t *testing.T) {
	host := fake.New(72, 10)
	done := make(chan error, 1)
	go func() {
		done <- program.Run(context.Background(), program.Config{
			Host: host,
			// The same reader, with the answer arriving as fast as the loop will take
			// it: what is being tested is where the pieces end up, not how long they
			// take.
			Inline: func(loop program.InlineLoop) program.Component {
				return read(loop, 64, time.Millisecond)
			},
		})
	}()

	// The heading is a finished block: it is printed into the terminal's own output
	// and never drawn again.
	host.Until(t, "the first block to be published", func() bool {
		return strings.Contains(host.Frames(), "What is happening here")
	})
	// And the whole answer arrives, ending with the row that says so.
	host.Until(t, "the answer to finish", func() bool {
		title, _, notified, _ := host.Said()
		return title == "" && len(notified) == 1
	})
	host.Shows(t, "that is the whole answer")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}
