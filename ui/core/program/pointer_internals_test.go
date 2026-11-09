package program

import (
	"context"
	"errors"
	"image"
	"runtime"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type pointerBindingSource struct {
	pipeSource
	bind func(func(input.Mouse, uint64) scene.CapturedInput) (<-chan scene.CapturedInput, func(), error)
}

func (s *pointerBindingSource) BindPointerInput(capture func(input.Mouse, uint64) scene.CapturedInput) (<-chan scene.CapturedInput, func(), error) {
	return s.bind(capture)
}

func TestPointerInputRetainsItsAdmissionFrameAcrossQueueBackpressure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		activated := ""
		button := func(name string) scene.Element {
			return scene.NewElement(scene.ElementConfig{Handle: func(event scene.Event) scene.EventResult {
				if mouse, ok := event.Input().(input.Mouse); ok && mouse.Action == input.MouseDown {
					activated = name
					return scene.EventResult{Consumed: true}
				}
				return scene.EventResult{}
			}})
		}
		group := scene.NewGroup(scene.GroupConfig{
			Items: []scene.Item{{Element: button("A")}, {Element: button("B")}},
			Layout: func(image.Point, []scene.Item) []image.Rectangle {
				return []image.Rectangle{image.Rect(0, 0, 1, 1), image.Rect(1, 0, 2, 1)}
			},
		})
		root, err := scene.NewRoot(scene.RootConfig{Element: group.Element()})
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		present := func(sequence uint64) {
			t.Helper()
			prepared, err := root.Prepare(grid.NewScreen(2, 1).Frame())
			if err != nil {
				t.Fatal(err)
			}
			prepared.Commit(sequence)
			root.Wrote(sequence)
		}
		present(1)
		pointers := make(chan scene.CapturedInput, 1)
		bound := make(chan func(input.Mouse, uint64) scene.CapturedInput, 1)
		released := 0
		source := &pointerBindingSource{
			events: make(chan input.Event, 1),
			bind: func(capture func(input.Mouse, uint64) scene.CapturedInput) (<-chan scene.CapturedInput, func(), error) {
				bound <- capture
				return pointers, func() { released++ }, nil
			},
		}
		pipe := newInputPipe(ctx, root, source, source.Events(), func(time.Time) (uint64, bool) {
			t.Error("synthetic input was recaptured as physical input")
			return 0, false
		}, Config{InputEvents: 1, InputBytes: 1})
		defer func() { cancel(); <-pipe.done }()
		capture := <-bound
		source.events <- input.Key{Rune: 'a'}
		synctest.Wait()
		pointers <- capture(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft}, 1)
		synctest.Wait()
		if err := group.Move(0, 1); err != nil {
			t.Fatal(err)
		}
		present(2)
		first := <-pipe.events
		if key, ok := first.event.Input().(input.Key); !ok || key.Rune != 'a' {
			t.Fatalf("input before pointer = %v", first.event.Input())
		}
		pipe.consumed <- first.bytes
		pointer := <-pipe.events
		if !root.HandleInput(pointer.event) || activated != "A" {
			t.Fatalf("buffered pointer activated %q, want admission frame's A", activated)
		}
		pipe.consumed <- pointer.bytes
		close(pointers)
		source.events <- input.Key{Rune: 'b'}
		close(source.events)
		last := <-pipe.events
		if key, ok := last.event.Input().(input.Key); !ok || key.Rune != 'b' {
			t.Fatalf("input after pointer = %v", last.event.Input())
		}
		<-pipe.done
		if err := pipe.takeResult(); err != nil || released != 1 {
			t.Fatalf("pipe ended with %v; binding released %d times", err, released)
		}
	})
}

func TestPointerBindingSharesTheInputLifetime(t *testing.T) {
	for _, ending := range []string{"input EOF", "pointer EOF", "cancellation", "invalid pointer", "oversized input"} {
		t.Run(ending, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				pointers := make(chan scene.CapturedInput, 1)
				released := 0
				source := &pointerBindingSource{
					events: make(chan input.Event, 1),
					bind: func(func(input.Mouse, uint64) scene.CapturedInput) (<-chan scene.CapturedInput, func(), error) {
						return pointers, func() { released++ }, nil
					},
				}
				pipe := newInputPipe(ctx, nil, source, source.Events(), nil, Config{InputBytes: 1})
				defer func() { cancel(); <-pipe.done }()
				var want error
				switch ending {
				case "input EOF":
					close(source.events)
				case "pointer EOF":
					close(pointers)
					source.events <- input.Key{Rune: 'x'}
					item, open := <-pipe.events
					if key, ok := item.event.Input().(input.Key); !open || !ok || key.Rune != 'x' {
						t.Fatal("pointer closure ended the main input stream")
					}
					close(source.events)
				case "cancellation":
					cancel()
				case "invalid pointer":
					pointers <- scene.CapturedInput{}
					want = ErrInvalidInput
				case "oversized input":
					source.events <- input.Paste{Text: "oversized"}
					want = ErrInputTooLarge
				}
				<-pipe.done
				if err := pipe.takeResult(); !errors.Is(err, want) || released != 1 {
					t.Fatalf("pipe ended with %v; binding released %d times", err, released)
				}
			})
		})
	}
}

func TestMalformedPointerBindingIsRejectedAndReleased(t *testing.T) {
	cause := errors.New("pointer source is already bound")
	for _, binding := range []struct {
		name    string
		stream  bool
		release bool
		err     error
	}{
		{name: "missing stream", release: true},
		{name: "missing release", stream: true},
		{name: "bind error", err: cause},
		{name: "partial bind error", release: true, err: cause},
	} {
		t.Run(binding.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				released := 0
				source := &pointerBindingSource{
					events: make(chan input.Event),
					bind: func(func(input.Mouse, uint64) scene.CapturedInput) (<-chan scene.CapturedInput, func(), error) {
						var stream <-chan scene.CapturedInput
						var release func()
						if binding.stream {
							stream = make(chan scene.CapturedInput)
						}
						if binding.release {
							release = func() { released++ }
						}
						return stream, release, binding.err
					},
				}
				pipe := newInputPipe(t.Context(), nil, source, source.Events(), nil, Config{})
				<-pipe.done
				err := pipe.takeResult()
				if err == nil || binding.err != nil && !errors.Is(err, binding.err) {
					t.Fatalf("malformed binding error = %v", err)
				}
				if binding.release && released != 1 || !binding.release && released != 0 {
					t.Fatalf("binding released %d times", released)
				}
			})
		})
	}
}

func TestPointerBindingFailureUnwindsOnTheProgramOwner(t *testing.T) {
	for _, stage := range []string{"binding", "release"} {
		for _, exit := range []string{"panic", "Goexit"} {
			t.Run(stage+"/"+exit, func(t *testing.T) {
				cause := errors.New("pointer callback failed")
				fail := func() {
					if exit == "Goexit" {
						runtime.Goexit()
					}
					panic(cause)
				}
				source := &pointerBindingSource{
					events: make(chan input.Event),
					bind: func(func(input.Mouse, uint64) scene.CapturedInput) (<-chan scene.CapturedInput, func(), error) {
						if stage == "binding" {
							fail()
						}
						return make(chan scene.CapturedInput), fail, nil
					},
				}
				close(source.events)
				pipe := newInputPipe(t.Context(), nil, source, source.Events(), nil, Config{})
				<-pipe.done
				var recovered any
				returned := false
				done := make(chan struct{})
				go func() {
					defer close(done)
					defer func() { recovered = recover() }()
					_ = pipe.takeResult()
					returned = true
				}()
				<-done
				failure, _ := recovered.(error)
				if returned || exit == "panic" && !errors.Is(failure, cause) || exit == "Goexit" && recovered != nil {
					t.Fatalf("returned=%t recovered=%v", returned, recovered)
				}
			})
		}
	}
}
