package program_test

import (
	"context"
	"errors"
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type pointerLifetimeHost struct {
	*host
	bound   chan struct{}
	release func()
}

func (h *pointerLifetimeHost) Input() program.EventSource { return h }

func (h *pointerLifetimeHost) BindPointerInput(func(input.Mouse, uint64) scene.CapturedInput) (<-chan scene.CapturedInput, func(), error) {
	close(h.bound)
	return make(chan scene.CapturedInput), h.release, nil
}

func TestPointerReleaseFailureUnwindsRunAfterQuitOrCancellation(t *testing.T) {
	for _, ending := range []string{"quit", "cancellation"} {
		for _, exit := range []string{"panic", "Goexit"} {
			t.Run(ending+"/"+exit, func(t *testing.T) {
				cause := errors.New("pointer release failed")
				releases, detaches, finalPaints := 0, 0, 0
				host := &pointerLifetimeHost{host: newHost(t), bound: make(chan struct{}), release: func() {
					releases++
					if exit == "Goexit" {
						runtime.Goexit()
					}
					panic(cause)
				}}
				built := make(chan *program.Runtime, 1)
				inline, err := program.NewInline(program.InlineConfig{
					Host: host,
					Build: func(runtime *program.InlineRuntime) (scene.Element, error) {
						built <- runtime.Runtime
						return scene.NewElement(scene.ElementConfig{
							Paint: func(scene.Frame) {
								select {
								case <-runtime.Dispatcher().Done():
									finalPaints++
								default:
								}
							},
							Detached: func() { detaches++ },
						}), nil
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				var recovered any
				var runErr error
				returned := false
				done := make(chan struct{})
				go func() {
					defer close(done)
					defer func() { recovered = recover() }()
					runErr = inline.Run(ctx)
					returned = true
				}()
				retained := <-built
				<-host.bound
				if postErr := retained.Dispatcher().Post(func() {
					if ending == "quit" {
						retained.Quit()
					} else {
						cancel()
					}
				}); postErr != nil {
					t.Fatal(postErr)
				}
				<-done
				failure, _ := recovered.(error)
				if returned || runErr != nil || exit == "panic" && !errors.Is(failure, cause) || exit == "Goexit" && recovered != nil {
					t.Fatalf("returned=%t error=%v recovered=%v", returned, runErr, recovered)
				}
				if releases != 1 || detaches != 1 || finalPaints != 1 {
					t.Fatalf("releases=%d detaches=%d final paints=%d, want one complete cleanup", releases, detaches, finalPaints)
				}
				if width, height := retained.Size(); width != 0 || height != 0 {
					t.Fatalf("abnormal release retained canvas size %dx%d", width, height)
				}
				if postErr := retained.Dispatcher().Post(func() { t.Error("stopped dispatcher accepted work") }); !errors.Is(postErr, program.ErrStopped) {
					t.Fatalf("dispatcher survived release failure: %v", postErr)
				}
			})
		}
	}
}
