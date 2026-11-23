package keymap_test

import (
	"reflect"
	"testing"
	"time"
	"unicode"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
)

func TestBindRejectsUnreachableChordsBeforePublication(t *testing.T) {
	for _, test := range []struct {
		name  string
		chord input.Chord
	}{
		{"no identity", input.Chord{}},
		{"functional key with rune", input.Chord{Code: input.Enter, Rune: 'x'}},
		{"control identity", input.Ctrl.Rune('\n')},
		{"unknown code", input.Chord{Code: input.KeypadBegin + 1}},
		{"negative code", input.Chord{Code: -1}},
		{"unknown modifier", input.Chord{Rune: 'x', Mods: input.Meta << 1}},
		{"invalid scalar", input.Chord{Rune: unicode.MaxRune + 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var bindings keymap.Map
			prefix, suffix := input.Chord{Rune: 'g'}, input.Chord{Rune: 'h'}
			bindings.Bind("kept", prefix, suffix)
			before := bindings.Bindings()
			checks, publications, notifications := 0, 0, 0
			subscription := bindings.Subscribe(keymap.SubscriptionConfig{
				Check:   func() error { checks++; return nil },
				Publish: func() { publications++ },
				Changed: func() { notifications++ },
			})
			defer subscription.Close()
			var matcher keymap.Matcher
			if _, matched := match(&matcher, &bindings, input.Key{Rune: prefix.Rune}); !matched {
				t.Fatal("sequence prefix was not accepted")
			}
			func() {
				defer func() {
					failure, ok := recover().(error)
					if !ok || failure.Error() != test.chord.Validate().Error() {
						t.Errorf("invalid binding panic = %v", failure)
					}
				}()
				bindings.Bind("invalid", prefix, test.chord, suffix)
				t.Error("invalid sequence was accepted")
			}()
			if !reflect.DeepEqual(bindings.Bindings(), before) || checks != 0 || publications != 0 || notifications != 0 {
				t.Fatalf("rejection changed bindings or notified consumers: bindings=%v checks=%d publications=%d notifications=%d", bindings.Bindings(), checks, publications, notifications)
			}
			if action, matched := match(&matcher, &bindings, input.Key{Rune: suffix.Rune}); !matched || action != "kept" {
				t.Fatalf("rejection revoked the prior input sequence: action=%q matched=%v", action, matched)
			}
		})
	}
}

func TestBindIgnoresNoKeysAndRefusesNoAction(t *testing.T) {
	var bindings keymap.Map
	bindings.Bind("unused")
	if len(bindings.Bindings()) != 0 {
		t.Fatal("a binding of no keys changed the map")
	}
	for name, bind := range map[string]func(){
		"Bind":   func() { bindings.Bind("", input.Chord{Rune: 'a'}) },
		"Rebind": func() { bindings.Rebind("", input.Keys{{Rune: 'a'}}) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("an empty action was bound")
				}
			}()
			bind()
		})
	}
	if len(bindings.Bindings()) != 0 {
		t.Fatal("a refused binding changed the map")
	}
}

func TestANilMapReadsEmptyAndRefusesChanges(t *testing.T) {
	var absent *keymap.Map
	if absent.Keys("any") != nil || absent.Bindings() != nil || absent.Timeout() != keymap.DefaultTimeout {
		t.Fatal("a nil map did not read as empty")
	}
	for name, change := range map[string]func(){
		"Bind":        func() { absent.Bind("action", input.Chord{Rune: 'a'}) },
		"Unbind":      func() { absent.Unbind(input.Chord{Rune: 'a'}) },
		"Rebind":      func() { absent.Rebind("action", input.Keys{{Rune: 'a'}}) },
		"SetTimeout":  func() { absent.SetTimeout(keymap.DefaultTimeout + time.Second) },
		"SetResolver": func() { absent.SetResolver(func(time.Duration, func()) func() { return func() {} }) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("a nil map accepted a change")
				}
			}()
			change()
		})
	}
}
