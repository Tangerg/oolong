package program

import (
	"context"
	"errors"
	"runtime"
	"strings"
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
	pipe := newInputPipe(ctx, root, source, source.Events(), nil, Config{InputEvents: 1, InputBytes: 2})
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
	if err := pipe.takeResult(); err != nil {
		t.Fatal(err)
	}
}

func TestOversizedNormalizedInputEndsWithAnExplicitCause(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	source := &pipeSource{events: make(chan input.Event, 1)}
	pipe := newInputPipe(ctx, nil, source, source.Events(), nil, Config{InputBytes: 2})
	source.events <- input.Paste{Text: "too large"}
	if _, open := <-pipe.events; open {
		t.Fatal("oversized input retained")
	}
	if err := pipe.takeResult(); !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("cause = %v", err)
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
		pipe := newInputPipe(t.Context(), nil, source, source.Events(), nil, Config{InputBytes: 2})
		if item, open := <-pipe.events; open {
			t.Errorf("%T bypassed budget with a charge of %d", tc.event, item.bytes)
		}
		<-pipe.done
		if err := pipe.takeResult(); !errors.Is(err, tc.want) {
			t.Errorf("%T ended with %v, want %v", tc.event, err, tc.want)
		}
	}
}

func TestSceneCannotRetainAMutableInputAlias(t *testing.T) {
	p, _ := taskOwner(t)
	mouse := &input.Mouse{}
	arrival := p.root.CaptureInput(mouse, nil)
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
	p := newInputPipe(t.Context(), nil, s, s.Events(), nil, Config{})
	<-p.done
	if called {
		t.Fatal("input worker invoked external status callback")
	}
	defer func() {
		if got, ok := recover().(error); !ok || !errors.Is(got, cause) {
			t.Error("owner did not receive the original panic")
		}
	}()
	_ = p.takeResult()
}

func TestDefaultInputBudgetDeliversALargeBracketedPaste(t *testing.T) {
	const payload = "abcd\xff"
	text := strings.Repeat(payload, 1<<20)
	var parser input.Parser
	events := parser.Feed([]byte("\x1b[200~" + text + "\x1b[201~"))
	source := &pipeSource{events: make(chan input.Event, len(events))}
	for _, ev := range events {
		source.events <- ev
	}
	close(source.events)
	pipe := newInputPipe(t.Context(), nil, source, source.Events(), nil, Config{})
	var delivered strings.Builder
	for item := range pipe.events {
		paste, ok := item.event.Input().(input.Paste)
		if !ok {
			t.Fatalf("paste became %T", item.event.Input())
		}
		delivered.WriteString(paste.Text)
		pipe.consumed <- item.bytes
	}
	if err := pipe.takeResult(); err != nil || delivered.String() != strings.ToValidUTF8(text, "�") {
		t.Fatalf("delivered %d bytes; input ended with %v", delivered.Len(), err)
	}
}

func TestInputCompletionFailureUnwindsOnTheProgramOwner(t *testing.T) {
	for _, exit := range []string{"panic", "Goexit"} {
		t.Run(exit, func(t *testing.T) {
			cause := errors.New("completion query failed")
			root, err := scene.NewRoot(scene.RootConfig{Element: scene.NewElement(scene.ElementConfig{})})
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			source := &pipeSource{events: make(chan input.Event, 1)}
			source.events <- input.Mouse{Action: input.MouseDown}
			close(source.events)
			pipe := newInputPipe(t.Context(), root, source, source.events, func(time.Time) (uint64, bool) {
				if exit == "Goexit" {
					runtime.Goexit()
				}
				panic(cause)
			}, Config{})
			<-pipe.done
			if _, ok := <-pipe.events; ok {
				t.Fatal("failed proof admitted input")
			}
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
