package program

import (
	"context"
	"strconv"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/oolong/ui/core/deadline"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestInputPipeReturnsCreditsWhileTheOwnerQueueIsFull(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		source := &pipeSource{events: make(chan input.Event)}
		pipe := newInputPipe(ctx, nil, source, source.Events(), Config{InputEvents: 1, InputBytes: 8})
		defer func() { cancel(); <-pipe.done }()

		// Delay two valid acknowledgements until the producer is waiting to send
		// another event. They must still be consumed while the event queue is full.
		for range 2 {
			source.events <- input.Key{Rune: 'a'}
			<-pipe.events
		}
		source.events <- input.Key{Rune: 'b'}
		synctest.Wait()
		source.events <- input.Key{Rune: 'c'}
		synctest.Wait()
		pipe.consumed <- 1
		pipe.consumed <- 1
		synctest.Wait()
		if len(pipe.events) != 1 || len(pipe.consumed) != 0 {
			t.Fatalf("full queue prevented credit return: queued=%d credits=%d", len(pipe.events), len(pipe.consumed))
		}
	})
}

func TestSmallInputQueueDrainsAContinuousKeyStreamThroughEOF(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const count = 128
		next := 0
		element := scene.NewElement(scene.ElementConfig{Handle: func(event scene.Event) scene.EventResult {
			key, ok := event.Input.(input.Key)
			if !ok || key.Text != strconv.Itoa(next) {
				t.Errorf("event %d = %#v", next, event.Input)
			}
			next++
			return scene.EventResult{Consumed: true}
		}})
		root, err := scene.NewRoot(scene.RootConfig{Element: element})
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		source := &pipeSource{events: make(chan input.Event)}
		pipe := newInputPipe(t.Context(), root, source, source.Events(), Config{InputEvents: 1, InputBytes: 8})
		owner := &program{root: root, inputPipe: pipe, tasks: newTaskQueue()}
		go func() {
			defer close(source.events)
			for at := range count {
				source.events <- input.Key{Text: strconv.Itoa(at)}
			}
		}()
		due := deadline.NewTimer()
		defer due.Stop()
		for {
			last, err := owner.wait(t.Context(), due)
			if err != nil {
				t.Fatal(err)
			}
			if last {
				break
			}
		}
		if next != count {
			t.Fatalf("EOF discarded input: handled %d of %d", next, count)
		}
	})
}

func TestInputEOFDoesNotWaitForAnAbsentCreditReader(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		handled := 0
		root, err := scene.NewRoot(scene.RootConfig{Element: scene.NewElement(scene.ElementConfig{
			Handle: func(scene.Event) scene.EventResult { handled++; return scene.EventResult{Consumed: true} },
		})})
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		pipe := &inputPipe{events: make(chan ownerInput, 1), consumed: make(chan int, 2), done: make(chan struct{})}
		pipe.events <- ownerInput{event: root.CaptureInput(input.Key{Rune: 'x'}), bytes: 1}
		pipe.consumed <- 1
		pipe.consumed <- 1
		close(pipe.events)
		close(pipe.done)
		owner := &program{root: root, inputPipe: pipe, tasks: newTaskQueue()}
		due := deadline.NewTimer()
		defer due.Stop()
		completed := make(chan bool, 1)
		go func() {
			last, waitErr := owner.wait(t.Context(), due)
			completed <- !last && waitErr == nil
		}()
		synctest.Wait()
		select {
		case ok := <-completed:
			if !ok || handled != 1 {
				t.Fatal("last buffered event was not handled")
			}
		default:
			// Release the regression's blocked goroutine before reporting failure.
			<-pipe.consumed
			<-completed
			t.Fatal("EOF left the owner waiting for a credit reader that had exited")
		}
		if last, err := owner.wait(t.Context(), due); !last || err != nil {
			t.Fatalf("EOF = %v, %v", last, err)
		}
	})
}
