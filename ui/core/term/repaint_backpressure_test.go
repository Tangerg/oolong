package term_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
)

type repaintTerminalSource struct {
	program.EventSource
	requests chan func(int, int, error)
}

func (s *repaintTerminalSource) Repaints() <-chan func(int, int, error) { return s.requests }

type repaintTerminalHost struct {
	program.Host
	source *repaintTerminalSource
}

func (h repaintTerminalHost) Input() program.EventSource { return h.source }

func TestRepaintBackpressureKeepsTheTerminalInputPumpMoving(t *testing.T) {
	primary, replica := pty(t)
	if err := resizePTY(replica, 20, 4); err != nil {
		t.Fatal(err)
	}
	tty, err := term.OpenOn(replica, replica, term.Config{}, os.LookupEnv)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := tty.Close(); closeErr != nil {
			t.Error(closeErr)
		}
	}()
	if _, ok := (<-tty.Events()).(input.Resize); !ok {
		t.Fatal("terminal did not announce its initial size")
	}
	base := program.TerminalHost(tty)
	source := &repaintTerminalSource{EventSource: base.Input(), requests: make(chan func(int, int, error))}
	host := repaintTerminalHost{Host: base, source: source}
	want := strings.Repeat("ab", 64)
	var delivered strings.Builder
	replies := 0
	screen, err := program.NewScreen(program.ScreenConfig{
		Host: host, InputEvents: 1, InputBytes: 1,
		Build: func(runtime *program.Runtime) (scene.Element, error) {
			if prepareErr := runtime.SetFramePreparation(func(int, int) (bool, error) { return false, nil }); prepareErr != nil {
				return scene.Element{}, prepareErr
			}
			return scene.NewElement(scene.ElementConfig{Handle: func(event scene.Event) scene.EventResult {
				if key, ok := event.Input().(input.Key); ok {
					if replies != 1 {
						t.Error("terminal input overtook its repaint result")
					}
					delivered.WriteRune(key.Rune)
					if delivered.Len() == len(want) {
						runtime.Quit()
					}
				}
				return scene.EventResult{Consumed: true}
			}}), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	finished := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		finished <- screen.Run(ctx)
	}()
	defer func() { cancel(); <-done }()
	select {
	case source.requests <- func(width, height int, err error) {
		replies++
		if width != 0 || height != 0 || !errors.Is(err, program.ErrRepaintBackpressure) {
			t.Errorf("repaint result=%dx%d, %v", width, height, err)
		}
	}:
	case err := <-finished:
		t.Fatalf("program ended before repaint admission: %v", err)
	case <-ctx.Done():
		t.Fatal("program did not admit the repaint")
	}
	if _, err := io.WriteString(primary, want); err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if replies != 1 || delivered.String() != want {
		t.Fatalf("replies=%d delivered=%q, want complete ordered terminal input", replies, delivered.String())
	}
}
