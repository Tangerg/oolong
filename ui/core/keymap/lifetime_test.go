package keymap_test

import (
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
)

func TestQueuedResolutionChecksItsInputLifetime(t *testing.T) {
	var matcher keymap.Matcher
	var resolve func()
	bindings := &keymap.Map{Resolve: func(_ time.Duration, callback func()) func() {
		resolve = callback
		return func() {}
	}}
	x := input.Chord{Rune: 'x'}
	bindings.Bind("short", x)
	bindings.Bind("long", x, x)
	current, calls := true, 0
	matcher.Handle(bindings, input.Key{Rune: 'x'}, func() bool { return current }, func(keymap.Action) bool {
		calls++
		return true
	})
	current = false
	resolve()
	if calls != 0 || len(matcher.Keys()) != 0 {
		t.Fatalf("revoked resolver dispatched %d calls and retained %v", calls, matcher.Keys())
	}
}

func TestExactPrefixCannotHandANewKeyToARevokedCaller(t *testing.T) {
	var matcher keymap.Matcher
	var bindings keymap.Map
	x := input.Chord{Rune: 'x'}
	bindings.Bind("short", x)
	bindings.Bind("long", x, x)
	current := true
	permission := func() bool { return current }
	do := func(keymap.Action) bool { current = false; return true }
	matcher.Handle(&bindings, input.Key{Rune: 'x'}, permission, do)
	matched, handled := matcher.Handle(&bindings, input.Key{Rune: 'q'}, permission, do)
	if !matched || !handled {
		t.Fatal("a focus-changing exact prefix returned the interrupted key for text insertion")
	}
}

func TestResolverCancellationCannotReauthorizeDispatch(t *testing.T) {
	var matcher keymap.Matcher
	var resolve func()
	current := true
	bindings := &keymap.Map{Resolve: func(_ time.Duration, callback func()) func() {
		resolve = callback
		return func() { current = false }
	}}
	x := input.Chord{Rune: 'x'}
	bindings.Bind("short", x)
	bindings.Bind("long", x, x)
	calls := 0
	matcher.Handle(bindings, input.Key{Rune: 'x'}, func() bool { return current }, func(keymap.Action) bool {
		calls++
		return true
	})
	resolve()
	if calls != 0 {
		t.Fatal("dispatch ignored lifetime revocation from resolver cancellation")
	}
}
