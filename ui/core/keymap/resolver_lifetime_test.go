package keymap_test

import (
	"runtime"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
)

func TestInterruptedResolverRevokesItsUnfinishedSequence(t *testing.T) {
	for _, interruption := range []struct {
		name      string
		run       func()
		wantPanic any
	}{
		{name: "panic", run: func() { panic("scheduler failed") }, wantPanic: "scheduler failed"},
		{name: "goroutine exit", run: runtime.Goexit},
	} {
		t.Run(interruption.name, func(t *testing.T) {
			var matcher keymap.Matcher
			var resolve func()
			bindings := &keymap.Map{Resolve: func(_ time.Duration, callback func()) func() {
				resolve = callback
				interruption.run()
				return nil
			}}
			x := input.Chord{Rune: 'x'}
			bindings.Bind("short", x)
			bindings.Bind("long", x, x)
			calls := 0
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() {
					if failure := recover(); failure != interruption.wantPanic {
						t.Errorf("scheduler failure = %v, want %v", failure, interruption.wantPanic)
					}
					if resolve == nil {
						t.Error("scheduler did not receive a callback")
						return
					}
					if len(matcher.Keys()) != 0 {
						t.Errorf("interrupted scheduler retained input: %v", matcher.Keys())
					}
					resolve()
					if calls != 0 {
						t.Errorf("interrupted scheduler dispatched %d actions", calls)
					}
				}()
				matcher.Handle(bindings, input.Key{Rune: 'x'}, nil, func(keymap.Action) bool {
					calls++
					return true
				})
				t.Error("interrupted scheduler returned normally")
			}()
			<-done
		})
	}
}

func TestInterruptedResolverPreservesANewerInputSequence(t *testing.T) {
	var matcher keymap.Matcher
	var bindings keymap.Map
	x, y := input.Chord{Rune: 'x'}, input.Chord{Rune: 'y'}
	bindings.Bind("short", x)
	bindings.Bind("long", x, x)
	bindings.Bind("new", y, y)
	var actions []keymap.Action
	dispatch := func(action keymap.Action) bool { actions = append(actions, action); return true }
	var resolve func()
	bindings.Resolve = func(_ time.Duration, callback func()) func() {
		resolve = callback
		matcher.Clear()
		matcher.Handle(&bindings, input.Key{Rune: 'y'}, nil, dispatch)
		panic("scheduler failed after reentry")
	}
	func() {
		defer func() {
			if failure := recover(); failure != "scheduler failed after reentry" {
				t.Errorf("scheduler failure = %v", failure)
			}
		}()
		matcher.Handle(&bindings, input.Key{Rune: 'x'}, nil, dispatch)
	}()
	resolve()
	if len(actions) != 0 || matcher.Keys().String() != "y" {
		t.Fatalf("interrupted scheduler overwrote newer input: actions=%v keys=%v", actions, matcher.Keys())
	}
	matcher.Handle(&bindings, input.Key{Rune: 'y'}, nil, dispatch)
	if len(actions) != 1 || actions[0] != "new" {
		t.Fatalf("newer sequence dispatched %v", actions)
	}
}
