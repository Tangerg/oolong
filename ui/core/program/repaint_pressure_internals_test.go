package program

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestRepaintPressureRestoresInputProgressThroughSourceTermination(t *testing.T) {
	for _, capacity := range []struct {
		name   string
		events int
		bytes  int
		reply  error
	}{
		{name: "input fits", events: 2, bytes: 8, reply: ErrStopped},
		{name: "event capacity", events: 1, bytes: 8, reply: ErrRepaintBackpressure},
		{name: "byte capacity", events: 2, bytes: 1, reply: ErrRepaintBackpressure},
	} {
		for _, sourceErr := range []error{nil, errors.New("input transport failed")} {
			t.Run(capacity.name+"/"+errorName(sourceErr), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					var delivered strings.Builder
					replies := 0
					root, err := scene.NewRoot(scene.RootConfig{Element: scene.NewElement(scene.ElementConfig{
						Handle: func(event scene.Event) scene.EventResult {
							if replies != 1 {
								t.Error("input overtook its uncompleted repaint")
							}
							delivered.WriteRune(event.Input().(input.Key).Rune)
							return scene.EventResult{Consumed: true}
						},
					})})
					if err != nil {
						t.Fatal(err)
					}
					source := &sourceWithResult{
						events: make(chan input.Event, 2),
						result: func() error { return sourceErr },
					}
					owner := &program{
						root: root, canvas: grid.NewScreen(20, 4), writer: &drainingWriter{changes: make(chan struct{}, 1)},
						ctx: ctx, cancel: cancel, tasks: newTaskQueue(),
						framePreparation: func(int, int) (bool, error) { return false, nil },
					}
					owner.inputPipe = newInputPipe(ctx, root, source, source.Events(), nil, Config{InputEvents: capacity.events, InputBytes: capacity.bytes})
					defer owner.stop()
					if err := owner.answerRepaint(&repaintRequest{reply: func(width, height int, err error) {
						replies++
						if width != 0 || height != 0 || !errors.Is(err, capacity.reply) {
							t.Errorf("repaint result=%dx%d, %v; want %v", width, height, err, capacity.reply)
						}
					}}); err != nil {
						t.Fatal(err)
					}
					source.events <- input.Key{Rune: 'a'}
					source.events <- input.Key{Rune: 'b'}
					close(source.events)
					finished := make(chan error, 1)
					go func() { finished <- owner.run(ctx) }()
					synctest.Wait()
					select {
					case err := <-finished:
						if !errors.Is(err, sourceErr) || replies != 1 || delivered.String() != "ab" {
							t.Fatalf("termination: err=%v replies=%d input=%q", err, replies, delivered.String())
						}
					default:
						cancel()
						stoppedErr := <-finished
						t.Fatalf("repaint blocked discovery of the source's terminal result; cancellation returned %v", stoppedErr)
					}
				})
			})
		}
	}
}

func errorName(err error) string {
	if err != nil {
		return "source error"
	}
	return "EOF"
}

func TestPressureQueryReturnsCreditsBeforeRefusingAdmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		source := &pipeSource{events: make(chan input.Event)}
		pipe := newInputPipe(ctx, nil, source, source.Events(), nil, Config{InputEvents: 1, InputBytes: 1})
		defer func() { cancel(); <-pipe.done }()
		source.events <- input.Key{Rune: 'a'}
		first := <-pipe.events
		source.events <- input.Key{Rune: 'b'}
		synctest.Wait()
		pipe.consumed <- first.bytes
		answer := make(chan bool, 1)
		select {
		case pipe.pressure <- answer:
			if <-answer {
				t.Fatal("previously returned credit was mistaken for blocked input")
			}
		case second := <-pipe.events:
			if key := second.event.Input().(input.Key); key.Rune != 'b' {
				t.Fatalf("next input=%v", key)
			}
			return
		}
		if key := (<-pipe.events).event.Input().(input.Key); key.Rune != 'b' {
			t.Fatalf("next input=%v", key)
		}
	})
}
