package main

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

// observedHost adds only the two session capabilities this example exercises.
// Embedding the minimal public test host keeps capability absence testable.
type observedHost struct {
	*uitest.Host
	mu       sync.Mutex
	title    string
	notified []string
}

func (h *observedHost) SetTitle(title string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.title = title
	return nil
}

func (h *observedHost) Notify(text string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.notified = append(h.notified, text)
	return nil
}

func (h *observedHost) state() (string, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.title, len(h.notified)
}

func TestWhatIsFinishedIsPrintedAndWhatIsNotIsDrawn(t *testing.T) {
	host := &observedHost{Host: uitest.New(t, uitest.Config{Width: 72, Height: 10})}
	done := make(chan error, 1)
	go func() {
		app, err := program.NewInline(program.InlineConfig{Host: host, Build: func(runtime *program.InlineRuntime) (scene.Element, error) {
			return read(runtime, 64, time.Millisecond).Element(), nil
		}})
		if err == nil {
			err = app.Run(t.Context())
		}
		done <- err
	}()

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
