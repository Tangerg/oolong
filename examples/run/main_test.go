package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/uitest"
)

// observedHost adds exactly the session capabilities this example exercises.
type observedHost struct {
	*uitest.Host
	mu       sync.Mutex
	title    string
	rang     int
	notified []string
}

func (h *observedHost) SetTitle(title string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.title = title
	return nil
}

func (h *observedHost) Bell() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rang++
	return nil
}

func (h *observedHost) Notify(text string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.notified = append(h.notified, text)
	return nil
}

func (h *observedHost) attention() (int, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.rang, len(h.notified)
}

func TestOutputKeepsItsColoursAndFinishedLinesBelongToTheTerminal(t *testing.T) {
	host := &observedHost{Host: uitest.New(t, uitest.Config{Width: 60, Height: 8})}
	var r *runner
	done := make(chan error, 1)
	ready := make(chan struct{})
	go func() {
		app, err := program.NewInline(program.InlineConfig{
			Host: host,
			// The depth is named rather than detected: a host that reports no colour
			// makes the assertion below unfalsifiable, which is how the colour claim
			// came to be written as "or the word red".
			Color: grid.TrueColor, Build: func(runtime *program.InlineRuntime) (scene.Element, error) {
				// Not newRunner: that one starts a process, and what is being tested is
				// what happens to what it says.
				r = &runner{
					runtime: runtime,
					command: []string{"a", "command"},
				}
				close(ready)
				return r.Element(), nil
			},
		})
		if err == nil {
			err = app.Run(t.Context())
		}
		done <- err
	}()
	<-ready
	host.Shows(t, "ctrl+e: editor")

	// A chunk boundary can fall anywhere, including inside an escape sequence.
	if err := r.publish(t.Context(), nil, func() []text.Line { return r.out.Feed("plain \x1b[3") }); err != nil {
		t.Fatal(err)
	}
	if err := r.publish(t.Context(), nil, func() []text.Line { return r.out.Feed("1mred\x1b[0m and more\nsecond line\nhalf a li") }); err != nil {
		t.Fatal(err)
	}

	// What a newline finished is printed into the terminal's own output; what is left
	// is still being drawn.
	host.Until(t, "the terminal to receive the finished lines", func() bool {
		return strings.Contains(host.Frames(), "second line")
	})
	host.Shows(t, "half a li")

	// And the colour survived the chunk boundary: red is 31, which the frame carries
	// as the colour the palette gives it.
	host.Until(t, "the output to keep its colour", func() bool {
		host.Repaint()
		return strings.Contains(host.Frames(), "128;0;0")
	})

	if err := r.publish(t.Context(), nil, r.out.Flush); err != nil {
		t.Fatal(err)
	}
	if err := r.runtime.Dispatcher().Invoke(t.Context(), func() error { r.finish(nil); return nil }); err != nil {
		t.Fatal(err)
	}
	host.Until(t, "the run to be said to be over", func() bool {
		rang, notified := host.attention()
		return rang == 1 && notified == 1
	})

	host.Send(input.Key{Code: input.Character, Rune: 'e', Mods: input.Ctrl})
	host.Shows(t, errors.ErrUnsupported.Error())
	host.Shows(t, "done   ctrl+c: leave")
	host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl})
	if err := <-done; err != nil {
		t.Fatalf("the program ended with %v", err)
	}
}

func TestCommandBurstHonorsPublicationCapacity(t *testing.T) {
	if len(os.Args) > 1 && os.Args[len(os.Args)-1] == "burst-fixture" {
		fmt.Print("burst-first\nburst-middle\nburst-last")
		os.Exit(0)
	}
	host := &observedHost{Host: uitest.New(t, uitest.Config{Width: 60, Height: 6})}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		app, err := program.NewInline(program.InlineConfig{
			Host: host, Publication: publication.Limits{MaxItems: 1},
			Build: func(rt *program.InlineRuntime) (scene.Element, error) {
				return newRunner(rt, []string{os.Args[0], "-test.run=^TestCommandBurstHonorsPublicationCapacity$", "--", "burst-fixture"}).Element(), nil
			},
		})
		if err == nil {
			err = app.Run(ctx)
		}
		done <- err
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("run: %v", err)
		}
	})
	host.Until(t, "the command to finish", func() bool {
		rang, notified := host.attention()
		return rang > 0 && notified > 0
	})
	host.Shows(t, "done   ctrl+c: leave")
	for _, line := range []string{"burst-first", "burst-middle", "burst-last"} {
		if !strings.Contains(host.Frames(), line) {
			t.Errorf("command completed without publishing %q", line)
		}
	}
	if rang, notified := host.attention(); rang != 1 || notified != 1 {
		t.Fatalf("command announced %d bells and %d notifications", rang, notified)
	}
}
