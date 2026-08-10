package main

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/programtest"
)

// observedHost adds only the two session capabilities this example exercises.
// Embedding the minimal public test host keeps capability absence testable.
type observedHost struct {
	*programtest.Host
	mu       sync.Mutex
	title    string
	notified []string
}

func (h *observedHost) SetTitle(title string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.title = title
}

func (h *observedHost) Notify(text string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.notified = append(h.notified, text)
}

func (h *observedHost) state() (string, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.title, len(h.notified)
}

func TestWhatIsFinishedIsPrintedAndWhatIsNotIsDrawn(t *testing.T) {
	host := &observedHost{Host: programtest.New(t, 72, 10)}
	done := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: host,
			// The same reader, with the answer arriving as fast as the runtime will take
			// it: what is being tested is where the pieces end up, not how long they
			// take.
			Inline: func(runtime *program.InlineRuntime) program.Component {
				return read(runtime, 64, time.Millisecond)
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
		title, notified := host.state()
		return title == "" && notified == 1
	})
	if frames := host.Frames(); !strings.Contains(frames, "√") {
		t.Fatal("the streamed display formula was not rendered through the Markdown extension")
	}
	host.Shows(t, "that is the whole answer")

	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}
