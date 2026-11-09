package keymap_test

import (
	"errors"
	"runtime"
	"slices"
	"testing"
	"time"
	"weak"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
)

func TestEveryMapMutationChecksAllConsumersBeforeCommit(t *testing.T) {
	x := input.Chord{Rune: 'x'}
	for _, test := range []struct {
		name   string
		change func(*keymap.Map)
	}{
		{"bind", func(m *keymap.Map) { m.Bind("new", x) }},
		{"unbind", func(m *keymap.Map) { m.Unbind(x) }},
		{"timeout", func(m *keymap.Map) { m.SetTimeout(2 * time.Second) }},
		{"resolver", func(m *keymap.Map) { m.SetResolver(nil) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var bindings keymap.Map
			bindings.Bind("old", x)
			bindings.Bind("long", x, x)
			var resolve func()
			bindings.SetResolver(func(_ time.Duration, callback func()) func() { resolve = callback; return func() {} })
			checks, notifications := 0, 0
			first := bindings.Subscribe(keymap.SubscriptionConfig{Check: func() error { checks++; return nil }, Changed: func() { notifications++ }})
			defer first.Close()
			refusal := errors.New("consumer has a pending frame")
			second := bindings.Subscribe(keymap.SubscriptionConfig{Check: func() error { checks++; return refusal }, Changed: func() { notifications++ }})
			defer second.Close()
			var matcher keymap.Matcher
			var actions []keymap.Action
			matcher.Handle(&bindings, input.Key{Rune: 'x'}, nil, func(action keymap.Action) bool { actions = append(actions, action); return true })
			func() {
				defer func() {
					failure, ok := recover().(error)
					if !ok || !errors.Is(failure, refusal) {
						t.Errorf("mutation refusal = %v", failure)
					}
				}()
				test.change(&bindings)
				t.Error("mutation bypassed consumer refusal")
			}()
			resolve()
			action, ok := bindings.Action(x)
			if checks != 2 || notifications != 0 || !ok || action != "old" || bindings.Timeout() != keymap.DefaultTimeout || bindings.Resolver() == nil || !slices.Equal(actions, []keymap.Action{"old"}) {
				t.Fatalf("refusal changed owner: checks=%d notifications=%d action=%q timeout=%v resolver=%v dispatched=%v", checks, notifications, action, bindings.Timeout(), bindings.Resolver() != nil, actions)
			}
		})
	}
}

func TestMapNotificationOwnsCommittedDeliveryAcrossReentry(t *testing.T) {
	var bindings keymap.Map
	var first *keymap.Subscription
	var order []string
	first = bindings.Subscribe(keymap.SubscriptionConfig{
		Check: func() error { return nil },
		Changed: func() {
			if action, _ := bindings.Action(input.Chord{Rune: 'x'}); action != "old" {
				t.Errorf("notification preceded commit: %q", action)
			}
			order = append(order, "first")
			first.Close()
			bindings.Bind("new", input.Chord{Rune: 'y'})
		},
	})
	defer first.Close()
	second := bindings.Subscribe(keymap.SubscriptionConfig{Check: func() error { return nil }, Changed: func() { order = append(order, "second") }})
	defer second.Close()
	bindings.Bind("old", input.Chord{Rune: 'x'})
	if !slices.Equal(order, []string{"first", "second", "second"}) {
		t.Fatalf("reentrant delivery = %v", order)
	}
	first.Close()
	second.Close()
	bindings.Bind("later", input.Chord{Rune: 'z'})
	if len(order) != 3 {
		t.Fatalf("closed consumer notified: %v", order)
	}
}

func TestCommittedMapChangeReachesAllConsumersDespiteInterruption(t *testing.T) {
	for _, interruption := range []struct {
		name    string
		run     func()
		failure any
	}{
		{"panic", func() { panic("notification failed") }, "notification failed"},
		{"Goexit", runtime.Goexit, nil},
	} {
		t.Run(interruption.name, func(t *testing.T) {
			var bindings keymap.Map
			delivered := 0
			first := bindings.Subscribe(keymap.SubscriptionConfig{Check: func() error { return nil }, Changed: interruption.run})
			defer first.Close()
			second := bindings.Subscribe(keymap.SubscriptionConfig{Check: func() error { return nil }, Changed: func() { delivered++ }})
			defer second.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() {
					if failure := recover(); failure != interruption.failure {
						t.Errorf("interruption = %v", failure)
					}
				}()
				bindings.Bind("committed", input.Chord{Rune: 'x'})
			}()
			<-done
			action, ok := bindings.Action(input.Chord{Rune: 'x'})
			if !ok || action != "committed" || delivered != 1 {
				t.Fatalf("committed delivery = %q, %d", action, delivered)
			}
		})
	}
}

func TestMapPoliciesRevokeAlreadyScheduledSequences(t *testing.T) {
	for _, change := range []struct {
		name  string
		apply func(*keymap.Map)
	}{
		{"timeout", func(m *keymap.Map) { m.SetTimeout(2 * time.Second) }},
		{"resolver", func(m *keymap.Map) { m.SetResolver(nil) }},
	} {
		t.Run(change.name, func(t *testing.T) {
			var bindings keymap.Map
			x := input.Chord{Rune: 'x'}
			bindings.Bind("short", x)
			bindings.Bind("long", x, x)
			var resolve func()
			bindings.SetResolver(func(_ time.Duration, callback func()) func() { resolve = callback; return func() {} })
			var matcher keymap.Matcher
			calls := 0
			matcher.Handle(&bindings, input.Key{Rune: 'x'}, nil, func(keymap.Action) bool { calls++; return true })
			change.apply(&bindings)
			resolve()
			if calls != 0 || len(matcher.Keys()) != 0 {
				t.Fatalf("old policy dispatched %d actions, retained %v", calls, matcher.Keys())
			}
		})
	}
}

func TestSharedMapDoesNotRetainAbandonedConsumers(t *testing.T) {
	var bindings keymap.Map
	type consumer struct {
		subscription  *keymap.Subscription
		notifications int
	}
	attach := func() weak.Pointer[consumer] {
		owner := &consumer{}
		owner.subscription = bindings.Subscribe(keymap.SubscriptionConfig{Check: func() error { return nil }, Changed: func() { owner.notifications++ }})
		return weak.Make(owner)
	}
	reference := attach()
	runtime.GC()
	if reference.Value() != nil {
		t.Fatal("shared map retained its abandoned consumer")
	}
	bindings.Bind("still usable", input.Chord{Rune: 'x'})
	runtime.KeepAlive(&bindings)
}

func TestConsumerChecksCannotReenterMapMutationOrRegistration(t *testing.T) {
	for _, test := range []struct {
		name    string
		reenter func(*keymap.Map)
	}{
		{"bind", func(m *keymap.Map) { m.Bind("inner", input.Chord{Rune: 'y'}) }},
		{"subscribe", func(m *keymap.Map) {
			m.Subscribe(keymap.SubscriptionConfig{Check: func() error { return nil }, Changed: func() {}})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var bindings keymap.Map
			bindings.Bind("before", input.Chord{Rune: 'x'})
			subscription := bindings.Subscribe(keymap.SubscriptionConfig{Check: func() error { test.reenter(&bindings); return nil }, Changed: func() { t.Error("reentrant refusal was published") }})
			func() {
				defer func() {
					failure, ok := recover().(error)
					if !ok || !errors.Is(failure, keymap.ErrChecking) {
						t.Errorf("reentrant mutation returned %v", failure)
					}
				}()
				bindings.Bind("outer", input.Chord{Rune: 'x'})
				t.Error("consumer check changed the map it was checking")
			}()
			subscription.Close()
			if action, _ := bindings.Action(input.Chord{Rune: 'x'}); action != "before" {
				t.Errorf("outer request changed state after failed check: %q", action)
			}
			if _, ok := bindings.Action(input.Chord{Rune: 'y'}); ok {
				t.Error("nested request escaped checking gate")
			}
			bindings.Bind("after", input.Chord{Rune: 'x'})
			if action, _ := bindings.Action(input.Chord{Rune: 'x'}); action != "after" {
				t.Error("failed check left map locked")
			}
		})
	}
}

func TestAllConsumersPublishBeforeNotificationsDespiteInterruption(t *testing.T) {
	for _, interruption := range []struct {
		name    string
		run     func()
		failure any
	}{
		{"panic", func() { panic("publication failed") }, "publication failed"},
		{"Goexit", runtime.Goexit, nil},
	} {
		t.Run(interruption.name, func(t *testing.T) {
			var bindings keymap.Map
			var order []string
			first := bindings.Subscribe(keymap.SubscriptionConfig{
				Check: func() error { return nil },
				Publish: func() {
					order = append(order, "publish first")
					interruption.run()
				},
				Changed: func() { order = append(order, "notify first") },
			})
			defer first.Close()
			second := bindings.Subscribe(keymap.SubscriptionConfig{
				Check:   func() error { return nil },
				Publish: func() { order = append(order, "publish second") },
				Changed: func() { order = append(order, "notify second") },
			})
			defer second.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer func() {
					if failure := recover(); failure != interruption.failure {
						t.Errorf("interruption = %v", failure)
					}
				}()
				bindings.Bind("committed", input.Chord{Rune: 'x'})
			}()
			<-done
			if !slices.Equal(order, []string{"publish first", "publish second", "notify first", "notify second"}) {
				t.Fatalf("interrupted publication = %v", order)
			}
			first.Close()
			second.Close()
			bindings.Bind("after", input.Chord{Rune: 'x'})
			if action, _ := bindings.Action(input.Chord{Rune: 'x'}); action != "after" {
				t.Fatal("interrupted publication left the map locked")
			}
		})
	}
}

func TestConsumerPublicationCannotReenterMapMutationOrRegistration(t *testing.T) {
	for _, test := range []struct {
		name    string
		reenter func(*keymap.Map)
	}{
		{"bind", func(m *keymap.Map) { m.Bind("inner", input.Chord{Rune: 'y'}) }},
		{"subscribe", func(m *keymap.Map) {
			m.Subscribe(keymap.SubscriptionConfig{Check: func() error { return nil }, Changed: func() {}})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var bindings keymap.Map
			notifications := 0
			subscription := bindings.Subscribe(keymap.SubscriptionConfig{
				Check:   func() error { return nil },
				Publish: func() { test.reenter(&bindings) },
				Changed: func() { notifications++ },
			})
			defer subscription.Close()
			func() {
				defer func() {
					failure, ok := recover().(error)
					if !ok || !errors.Is(failure, keymap.ErrPublishing) {
						t.Errorf("reentrant publication = %v", failure)
					}
				}()
				bindings.Bind("committed", input.Chord{Rune: 'x'})
				t.Error("consumer publication reentered the map")
			}()
			if action, _ := bindings.Action(input.Chord{Rune: 'x'}); action != "committed" || notifications != 1 {
				t.Fatalf("publication interruption lost committed delivery: action=%q notifications=%d", action, notifications)
			}
			if _, ok := bindings.Action(input.Chord{Rune: 'y'}); ok {
				t.Fatal("nested mutation escaped publication gate")
			}
		})
	}
}
