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
	if key := first.event.Input.(input.Key); key.Text != "a" {
		t.Fatalf("first key = %+v", key)
	}
	pipe.consumed <- first.bytes
	select {
	case next := <-pipe.events:
		if key := next.event.Input.(input.Key); key.Transition != input.Release {
			t.Fatalf("release lost: %+v", key)
		}
		pipe.consumed <- next.bytes
	case <-time.After(5 * time.Second):
		t.Fatal("budget was not returned to the reader")
	}
	<-pipe.done
	if pipe.err != nil {
		t.Fatal(pipe.err)
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
	if !errors.Is(pipe.err, ErrInputTooLarge) {
		t.Fatalf("cause = %v", pipe.err)
	}
}
