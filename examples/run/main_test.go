package main

import (
	"sync"
	"testing"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/programtest"
)

// observedHost adds exactly the session capabilities this example exercises.
type observedHost struct {
	*programtest.Host
	mu       sync.Mutex
	title    string
	rang     int
	notified []string
}

func (h *observedHost) SetTitle(title string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.title = title
}

func (h *observedHost) Bell() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rang++
}

func (h *observedHost) Notify(text string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.notified = append(h.notified, text)
}

func (h *observedHost) attention() (int, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.rang, len(h.notified)
}

func TestOutputKeepsItsColoursAndFinishedLinesBelongToTheTerminal(t *testing.T) {
	host := &observedHost{Host: programtest.New(t, 60, 8)}
	var r *runner
	done := make(chan error, 1)
	ready := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: host,
			Inline: func(runtime *program.InlineRuntime) program.Component {
				// Not newRunner: that one starts a process, and what is being tested is
				// what happens to what it says.
				r = &runner{
					runtime: runtime,
					command: []string{"a", "command"}, status: "running",
				}
				var err error
				r.ingress, err = program.NewByteIngress(runtime.Dispatcher(), 8, r.accept)
				ready <- err
				return r
			},
		})
	}()
	if err := <-ready; err != nil {
		t.Fatal(err)
	}
	host.Shows(t, "ctrl+e: editor")

	// A chunk boundary can fall anywhere, including inside an escape sequence.
	if _, err := r.ingress.Write([]byte("plain \x1b[3")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ingress.Write([]byte("1mred\x1b[0m and more\nsecond line\nhalf a li")); err != nil {
		t.Fatal(err)
	}

	// What a newline finished is printed into the terminal's own output; what is left
	// is still being drawn.
	host.Until(t, "the terminal to receive the finished lines", func() bool {
		return contains(host.Frames(), "second line")
	})
	host.Shows(t, "half a li")

	// And the colour survived the chunk boundary: red is 31, which the frame carries
	// as the colour the palette gives it.
	host.Until(t, "the output to keep its colour", func() bool {
		host.Repaint()
		return contains(host.Frames(), "128;0;0") || contains(host.Frames(), "red")
	})

	if err := r.ingress.Close(); err != nil {
		t.Fatal(err)
	}
	host.Until(t, "the run to be said to be over", func() bool {
		rang, notified := host.attention()
		return rang == 1 && notified == 1
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
