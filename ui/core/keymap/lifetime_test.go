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
	bindings := &keymap.Map{}
	bindings.SetResolver(func(_ time.Duration, callback func()) func() {
		resolve = callback
		return func() {}
	})
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
	bindings := &keymap.Map{}
	bindings.SetResolver(func(_ time.Duration, callback func()) func() {
		resolve = callback
		return func() { current = false }
	})
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

func TestResetStaleOnlyCancelsAnObsoleteMapSequence(t *testing.T) {
	for _, test := range []struct {
		name  string
		stale bool
	}{
		{name: "current"},
		{name: "obsolete", stale: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var matcher keymap.Matcher
			var bindings keymap.Map
			x := input.Chord{Rune: 'x'}
			bindings.Bind("short", x)
			bindings.Bind("long", x, x)
			var resolve func()
			published, cancelled := false, false
			bindings.SetResolver(func(_ time.Duration, callback func()) func() {
				resolve = callback
				return func() {
					if !published {
						t.Error("resolver cancellation preceded publication")
					}
					cancelled = true
				}
			})
			calls := 0
			matcher.Handle(&bindings, input.Key{Rune: 'x'}, nil, func(keymap.Action) bool { calls++; return true })
			if test.stale {
				bindings.Bind("another", input.Chord{Rune: 'y'})
			}
			matcher.ResetStale(func() { published = true })
			if !published || cancelled != test.stale {
				t.Fatalf("publication=%v cancellation=%v, stale=%v", published, cancelled, test.stale)
			}
			resolve()
			if test.stale && calls != 0 || !test.stale && calls != 1 {
				t.Fatalf("resolved %d actions, stale=%v", calls, test.stale)
			}
		})
	}
}
