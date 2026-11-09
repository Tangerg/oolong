package ssh

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
)

type repaintSSHSource struct {
	*eventSource
	requests chan func(int, int, error)
}

func (s *repaintSSHSource) Repaints() <-chan func(int, int, error) { return s.requests }

type repaintSSHHost struct {
	source *repaintSSHSource
	writer *term.Writer
}

func (h repaintSSHHost) Input() program.EventSource  { return h.source }
func (h repaintSSHHost) Writer() program.FrameWriter { return h.writer }
func (repaintSSHHost) Size() (int, int, error)       { return 20, 4, nil }

func TestRepaintBackpressureDrainsSSHInputThroughItsTerminalResult(t *testing.T) {
	for _, sourceErr := range []error{nil, errors.New("SSH input failed")} {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			reader, inputWriter := io.Pipe()
			defer func() { _ = reader.Close(); _ = inputWriter.Close() }()
			source := &repaintSSHSource{
				eventSource: newEventSource(ctx.Done(), reader, nil, nil),
				requests:    make(chan func(int, int, error)),
			}
			defer source.Close()
			writer := term.NewWriter(term.WriterConfig{Destination: io.Discard})
			defer func() {
				if err := writer.Close(); err != nil {
					t.Error(err)
				}
			}()
			want := strings.Repeat("ab", 64)
			var delivered strings.Builder
			replies := 0
			screen, err := program.NewScreen(program.ScreenConfig{
				Host: repaintSSHHost{source: source, writer: writer}, InputEvents: 1, InputBytes: 1,
				Build: func(runtime *program.Runtime) (scene.Element, error) {
					if err := runtime.SetFramePreparation(func(int, int) (bool, error) { return false, nil }); err != nil {
						return scene.Element{}, err
					}
					return scene.NewElement(scene.ElementConfig{Handle: func(event scene.Event) scene.EventResult {
						if replies != 1 {
							t.Error("SSH input overtook its repaint result")
						}
						delivered.WriteRune(event.Input().(input.Key).Rune)
						return scene.EventResult{Consumed: true}
					}}), nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			finished := make(chan error, 1)
			go func() { finished <- screen.Run(ctx) }()
			source.requests <- func(width, height int, err error) {
				replies++
				if width != 0 || height != 0 || !errors.Is(err, program.ErrRepaintBackpressure) {
					t.Errorf("repaint result=%dx%d, %v", width, height, err)
				}
			}
			if _, err := io.WriteString(inputWriter, want); err != nil {
				t.Fatal(err)
			}
			if err := inputWriter.CloseWithError(sourceErr); err != nil {
				t.Fatal(err)
			}
			synctest.Wait()
			select {
			case err := <-finished:
				if !errors.Is(err, sourceErr) || replies != 1 || delivered.String() != want {
					t.Fatalf("termination: err=%v replies=%d input=%q", err, replies, delivered.String())
				}
			default:
				cancel()
				stoppedErr := <-finished
				t.Fatalf("repaint barrier stranded the SSH source before its terminal result; cancellation returned %v", stoppedErr)
			}
		})
	}
}
