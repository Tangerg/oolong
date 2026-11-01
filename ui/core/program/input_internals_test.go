package program

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type pipeSource struct{ events chan input.Event }

func (s *pipeSource) Events() <-chan input.Event { return s.events }
func (*pipeSource) Err() error                   { return nil }

func TestNormalizedInputBudgetPreservesEdges(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	root, err := scene.NewRoot(scene.RootConfig{Element: scene.NewElement(scene.ElementConfig{})})
	if err != nil {
		t.Fatal(err)
	}
	source := &pipeSource{events: make(chan input.Event, 2)}
	pipe := newInputPipe(ctx, root, source, source.Events(), Config{InputEvents: 1, InputBytes: 2})
	source.events <- input.Key{Text: "a"}
	source.events <- input.Key{Transition: input.Release}
	close(source.events)
	first := <-pipe.events
	if key := first.event.Input().(input.Key); key.Text != "a" {
		t.Fatalf("first key = %+v", key)
	}
	pipe.consumed <- first.bytes
	select {
	case next := <-pipe.events:
		if key := next.event.Input().(input.Key); key.Transition != input.Release {
			t.Fatalf("release lost: %+v", key)
		}
		pipe.consumed <- next.bytes
	case <-time.After(5 * time.Second):
		t.Fatal("budget was not returned to the reader")
	}
	<-pipe.done
	if pipe.err() != nil {
		t.Fatal(pipe.err())
	}
}

func TestOversizedNormalizedInputEndsWithAnExplicitCause(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	source := &pipeSource{events: make(chan input.Event, 1)}
	pipe := newInputPipe(ctx, nil, source, source.Events(), Config{InputBytes: 2})
	source.events <- input.Paste{Text: "too large"}
	if _, open := <-pipe.events; open {
		t.Fatal("oversized input retained")
	}
	if !errors.Is(pipe.err(), ErrInputTooLarge) {
		t.Fatalf("cause = %v", pipe.err())
	}
}

func TestNormalizedInputCannotBypassTheBudgetThroughItsShape(t *testing.T) {
	for _, tc := range []struct {
		event input.Event
		want  error
	}{{&input.Paste{Text: "long"}, ErrInvalidInput}, {input.Attributes(62, 123456789), ErrInputTooLarge}} {
		source := &pipeSource{events: make(chan input.Event, 1)}
		source.events <- tc.event
		close(source.events)
		pipe := newInputPipe(t.Context(), nil, source, source.Events(), Config{InputBytes: 2})
		if item, open := <-pipe.events; open {
			t.Errorf("%T bypassed budget with a charge of %d", tc.event, item.bytes)
		}
		<-pipe.done
		if !errors.Is(pipe.err(), tc.want) {
			t.Errorf("%T ended with %v, want %v", tc.event, pipe.err(), tc.want)
		}
	}
}

func TestSceneCannotRetainAMutableInputAlias(t *testing.T) {
	p, _ := taskOwner(t)
	mouse := &input.Mouse{}
	arrival := p.root.CaptureInput(mouse)
	mouse.Action = input.MouseDown
	if arrival.Input() != nil || p.root.HandleInput(arrival) {
		t.Fatal("mutable input escaped the capture boundary")
	}
}

type sourceWithResult struct {
	pipeSource
	result func() error
}

func (s *sourceWithResult) Err() error { return s.result() }

func TestInputSourceFailureUnwindsOnTheProgramOwner(t *testing.T) {
	cause := errors.New("source failed while obtaining final status")
	var called bool
	s := &sourceWithResult{result: func() error { called = true; panic(cause) }}
	s.events = make(chan input.Event)
	close(s.events)
	p := newInputPipe(t.Context(), nil, s, s.Events(), Config{})
	<-p.done
	if called {
		t.Fatal("input worker invoked external status callback")
	}
	defer func() {
		if got, ok := recover().(error); !ok || !errors.Is(got, cause) {
			t.Error("owner did not receive the original panic")
		}
	}()
	_ = p.err()
}
