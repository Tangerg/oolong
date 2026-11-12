package program_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type startupHost struct {
	program.Host
	queries int
}

func (h *startupHost) Size() (int, int, error) {
	h.queries++
	return h.Host.Size()
}

func (h *startupHost) Input() program.EventSource {
	h.queries++
	return h.Host.Input()
}

func (h *startupHost) Writer() program.FrameWriter {
	h.queries++
	return h.Host.Writer()
}

func TestCancelledSessionDoesNotStart(t *testing.T) {
	for _, inline := range []bool{false, true} {
		name := "screen"
		if inline {
			name = "inline"
		}
		t.Run(name, func(t *testing.T) {
			h := &startupHost{Host: newHost(t)}
			built := false
			build := func(*program.Runtime) (scene.Element, error) {
				built = true
				return scene.NewElement(scene.ElementConfig{}), errors.New("cancelled session reached Build")
			}
			var session interface {
				Run(ctx context.Context) error
			}
			var err error
			cfg := program.Config{Host: h}
			if inline {
				session, err = program.NewInline(program.InlineConfig{Config: cfg, Build: func(rt *program.InlineRuntime) (scene.Element, error) {
					return build(rt.Runtime)
				}})
			} else {
				session, err = program.NewScreen(program.ScreenConfig{Config: cfg, Build: build})
			}
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancelCause(t.Context())
			cancel(errors.New("session withdrawn"))
			if err := session.Run(ctx); err != nil {
				t.Fatalf("cancelled Run = %v", err)
			}
			if h.queries != 0 || built {
				t.Errorf("cancelled session started: host queries=%d, built=%t", h.queries, built)
			}
			if err := session.Run(t.Context()); !errors.Is(err, program.ErrAlreadyRun) {
				t.Errorf("second Run = %v, want ErrAlreadyRun", err)
			}
		})
	}
}
