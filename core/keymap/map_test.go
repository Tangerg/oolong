package keymap_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
)

// at is a keystroke that arrived at a given moment, which is what a sequence is judged
// by. The zero time means nothing timed it.
func at(chord input.Chord, when time.Time) input.Key {
	return input.Key{Code: chord.Code, Rune: chord.Rune, Mods: chord.Mods, At: when}
}

func match(matcher *keymap.Matcher, bindings *keymap.Map, key input.Key) (keymap.Action, bool) {
	var action keymap.Action
	matched, _ := matcher.Handle(bindings, key, func(next keymap.Action) bool {
		action = next
		return true
	})
	return action, matched
}

func TestAChordSurvivesBeingWrittenDownAndReadBack(t *testing.T) {
	// A keybinding in a configuration file is a string, and the whole point of writing
	// one is that it comes back as the keystroke it names. Anything that only goes one
	// way is a binding a user can write and never press.
	for _, chord := range []input.Chord{
		{Code: input.Enter},
		{Code: input.Esc},
		{Code: input.Backspace},
		{Code: input.Tab},
		input.Shift.With(input.Tab),
		{Code: input.Up},
		{Code: input.PageDown},
		{Code: input.F1},
		{Code: input.F12},
		input.Ctrl.With(input.F5),
		input.Ctrl.Rune('w'),
		input.Alt.Rune('b'),
		input.Ctrl.Rune(' '),
		{Rune: '+'},
		input.Ctrl.Rune('+'),
		{Rune: '中'},
		(input.Ctrl | input.Alt | input.Shift | input.Super).Rune('x'),
	} {
		written := chord.String()
		read, ok := input.ParseChord(written)
		if !ok {
			t.Errorf("%q could not be read back", written)
			continue
		}
		if read != chord {
			t.Errorf("%q read back as %+v, want %+v", written, read, chord)
		}
	}
}

func TestWhatIsNotAKeystrokeIsNotReadAsOne(t *testing.T) {
	// Guessing would be worse than refusing: "contrl+a" is a typo in somebody's
	// configuration, and reading it as the letter c binds a key they never asked for.
	for _, s := range []string{"", "ctrl+", "contrl+a", "unknown", "char", "f13", "f0", "ab"} {
		if chord, ok := input.ParseChord(s); ok {
			t.Errorf("%q was read as %+v, want it refused", s, chord)
		}
	}
}

func TestASequenceIsWrittenWithSpacesBetweenTheChords(t *testing.T) {
	keys, ok := input.ParseKeys("ctrl+x ctrl+s")
	if !ok {
		t.Fatal("a two-chord sequence could not be read")
	}
	if len(keys) != 2 || keys[0] != input.Ctrl.Rune('x') || keys[1] != input.Ctrl.Rune('s') {
		t.Fatalf("read %+v", keys)
	}
	if got := keys.String(); got != "ctrl+x ctrl+s" {
		t.Fatalf("written back as %q", got)
	}
	// The space bar has a name, so there is nothing ambiguous to split on.
	spaced, ok := input.ParseKeys("ctrl+x space")
	if !ok || len(spaced) != 2 || spaced[1] != (input.Chord{Code: input.Character, Rune: ' '}) {
		t.Fatalf("a sequence ending in the space bar read as %+v (ok=%v)", spaced, ok)
	}
	if _, ok := input.ParseKeys("ctrl+x nonsense"); ok {
		t.Error("a sequence with a chord nobody can press was accepted")
	}
}

func TestAKeystrokeNamesTheActionItIsBoundTo(t *testing.T) {
	m := &keymap.Map{}
	m.Bind("delete-word-back", input.Ctrl.Rune('w'))

	action, mine := match(&keymap.Matcher{}, m, input.Key{Code: input.Character, Rune: 'w', Mods: input.Ctrl})
	if !mine || action != "delete-word-back" {
		t.Fatalf("ctrl+w = %q (mine=%v)", action, mine)
	}
	// A keystroke the map says nothing about is not the map's, which is what lets it
	// carry on to whatever else might want it.
	if _, mine := match(&keymap.Matcher{}, m, input.Key{Code: input.Character, Rune: 'q'}); mine {
		t.Error("the map claimed a keystroke nothing was bound to")
	}
	// A key coming back up is not a keystroke.
	up := input.Key{Code: input.Character, Rune: 'w', Mods: input.Ctrl, Transition: input.Release}
	if _, mine := match(&keymap.Matcher{}, m, up); mine {
		t.Error("the map answered a key being let go of")
	}
}

func TestBindingsAreACallerOwnedSnapshotOfTheWholeMap(t *testing.T) {
	m := &keymap.Map{}
	original := input.Ctrl.Rune('w')
	m.Bind("delete-word-back", original)
	m.Bind("submit", input.Chord{Code: input.Enter})

	bindings := m.Bindings()
	if len(bindings) != 2 || bindings[0].String() != "ctrl+w delete-word-back" {
		t.Fatalf("bindings = %+v, want the complete map in binding order", bindings)
	}
	bindings[0].Keys[0] = input.Chord{Code: input.Esc}
	bindings[0].Action = "changed"
	if action, ok := m.Action(original); !ok || action != "delete-word-back" {
		t.Fatalf("changing the snapshot changed the map: action=%q ok=%v", action, ok)
	}
}

func TestASequenceIsFinishedByItsLastChord(t *testing.T) {
	m := &keymap.Map{}
	m.Bind("go-to-top", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	var matcher keymap.Matcher
	now := time.Unix(1700000000, 0)

	// The first chord is the map's and names nothing yet. Consumed all the same: a
	// caller that passed it on would let half a sequence act somewhere else too.
	action, mine := match(&matcher, m, at(input.Chord{Rune: 'g'}, now))
	if !mine || action != "" {
		t.Fatalf("the first chord = %q (mine=%v), want it taken and nothing done", action, mine)
	}
	if got := matcher.Keys().String(); got != "g" {
		t.Fatalf("waiting on %q", got)
	}
	action, mine = match(&matcher, m, at(input.Chord{Rune: 'g'}, now.Add(50*time.Millisecond)))
	if !mine || action != "go-to-top" {
		t.Fatalf("the second chord = %q (mine=%v)", action, mine)
	}
	if got := matcher.Keys(); len(got) != 0 {
		t.Fatalf("still waiting on %q after the sequence finished", got.String())
	}
}

func TestASequenceLeftTooLongIsOver(t *testing.T) {
	// The pause is the whole difference between a sequence and two keystrokes that
	// happen to be adjacent, and only the arrival time can tell them apart.
	m := &keymap.Map{Timeout: 100 * time.Millisecond}
	m.Bind("go-to-top", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	m.Bind("quit", input.Chord{Rune: 'q'})
	var matcher keymap.Matcher
	now := time.Unix(1700000000, 0)

	match(&matcher, m, at(input.Chord{Rune: 'g'}, now))
	action, mine := match(&matcher, m, at(input.Chord{Rune: 'g'}, now.Add(time.Second)))
	if action != "" {
		t.Fatalf("a chord a second later finished the sequence anyway: %q", action)
	}
	if !mine || matcher.Keys().String() != "g" {
		t.Fatalf("the late chord did not begin the sequence again: %q (mine=%v)", matcher.Keys().String(), mine)
	}

	// A chord that does not continue what was being typed is read on its own, because
	// the user has plainly stopped spelling one thing and started another.
	action, mine = match(&matcher, m, at(input.Chord{Rune: 'q'}, now.Add(time.Second)))
	if !mine || action != "quit" {
		t.Fatalf("the chord that broke off the sequence = %q (mine=%v)", action, mine)
	}
}

func TestASequenceAtItsTimeoutStillCompletes(t *testing.T) {
	m := &keymap.Map{Timeout: 100 * time.Millisecond}
	m.Bind("go-to-top", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	var matcher keymap.Matcher
	now := time.Unix(1700000000, 0)

	match(&matcher, m, at(input.Chord{Rune: 'g'}, now))
	action, mine := match(&matcher, m, at(input.Chord{Rune: 'g'}, now.Add(m.Timeout)))
	if !mine || action != "go-to-top" {
		t.Fatalf("chord at timeout = %q (mine=%v), want the sequence completed", action, mine)
	}
}

func TestASequenceFromTheFutureDoesNotJoinThePresent(t *testing.T) {
	m := &keymap.Map{}
	m.Bind("go-to-top", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	var matcher keymap.Matcher
	now := time.Unix(1700000000, 0)
	match(&matcher, m, at(input.Chord{Rune: 'g'}, now))

	action, mine := match(&matcher, m, at(input.Chord{Rune: 'g'}, now.Add(-time.Second)))
	if action != "" || !mine || matcher.Keys().String() != "g" {
		t.Fatalf("out-of-order chord = %q (mine=%v, pending=%q), want a fresh prefix",
			action, mine, matcher.Keys().String())
	}
}

func TestAKeystrokeNothingTimedNeverGoesStale(t *testing.T) {
	// A caller feeding synthetic events has no clock in them, and a
	// sequence that could never be completed would be worse than one with no deadline.
	m := &keymap.Map{Timeout: time.Nanosecond}
	m.Bind("go-to-top", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	var matcher keymap.Matcher

	match(&matcher, m, input.Key{Rune: 'g'})
	if action, _ := match(&matcher, m, input.Key{Rune: 'g'}); action != "go-to-top" {
		t.Fatalf("= %q, want the sequence completed", action)
	}
}

func TestANilMatcherHandlesNothing(t *testing.T) {
	m := &keymap.Map{}
	m.Bind("go-to-top", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	m.Bind("quit", input.Chord{Rune: 'q'})

	if _, mine := match(nil, m, input.Key{Rune: 'g'}); mine {
		t.Error("a nil matcher took half a sequence")
	}
	if action, mine := match(nil, m, input.Key{Rune: 'q'}); mine || action != "" {
		t.Errorf("nil matcher = %q (mine=%v), want it inert", action, mine)
	}
}

func TestAPrefixBindingNeverShadowsALongerSequence(t *testing.T) {
	m := &keymap.Map{}
	m.Bind("short", input.Chord{Rune: 'g'})
	m.Bind("long", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})

	var matcher keymap.Matcher
	if action, mine := match(&matcher, m, input.Key{Rune: 'g'}); !mine || action != "" {
		t.Fatalf("prefix = %q (mine=%v), want it held for the longer sequence", action, mine)
	}
	if action, mine := match(&matcher, m, input.Key{Rune: 'g'}); !mine || action != "long" {
		t.Fatalf("completed sequence = %q (mine=%v), want long", action, mine)
	}
}

func TestAResolverMakesAnExactPrefixReachable(t *testing.T) {
	var resolve func()
	var cancelled bool
	m := &keymap.Map{
		Timeout: 75 * time.Millisecond,
		Resolve: func(wait time.Duration, fn func()) func() {
			if wait != 75*time.Millisecond {
				t.Fatalf("resolver wait = %v", wait)
			}
			resolve = fn
			return func() { cancelled = true }
		},
	}
	m.Bind("short", input.Chord{Rune: 'g'})
	m.Bind("long", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})

	var matcher keymap.Matcher
	var actions []keymap.Action
	matched, handled := matcher.Handle(m, input.Key{Rune: 'g'}, func(action keymap.Action) bool {
		actions = append(actions, action)
		return true
	})
	if !matched || !handled || resolve == nil || len(actions) != 0 {
		t.Fatalf("first chord matched=%v handled=%v resolve=%v actions=%v",
			matched, handled, resolve != nil, actions)
	}
	resolve()
	if !cancelled || !slices.Equal(actions, []keymap.Action{"short"}) {
		t.Fatalf("resolved actions = %v, cancelled=%v", actions, cancelled)
	}
	if keys := matcher.Keys(); len(keys) != 0 {
		t.Fatalf("resolved matcher retained %q", keys.String())
	}
}

func TestAContinuationCancelsTheExactResolver(t *testing.T) {
	var cancelled int
	var late func()
	m := &keymap.Map{Resolve: func(_ time.Duration, resolve func()) func() {
		late = resolve
		return func() { cancelled++ }
	}}
	m.Bind("short", input.Chord{Rune: 'g'})
	m.Bind("long", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})

	var matcher keymap.Matcher
	match(&matcher, m, input.Key{Rune: 'g'})
	action, mine := match(&matcher, m, input.Key{Rune: 'g'})
	if !mine || action != "long" || cancelled != 1 {
		t.Fatalf("continuation = %q (mine=%v, cancelled=%d)", action, mine, cancelled)
	}
	late()
	if keys := matcher.Keys(); len(keys) != 0 {
		t.Fatalf("late resolver revived %q", keys.String())
	}
}

func TestAKeyThatBreaksAnAmbiguitySettlesItThenGetsAReading(t *testing.T) {
	m := &keymap.Map{}
	m.Bind("short", input.Chord{Rune: 'g'})
	m.Bind("long", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	m.Bind("quit", input.Chord{Rune: 'q'})

	var matcher keymap.Matcher
	var actions []keymap.Action
	do := func(action keymap.Action) bool {
		actions = append(actions, action)
		return true
	}
	matcher.Handle(m, input.Key{Rune: 'g'}, do)
	matched, handled := matcher.Handle(m, input.Key{Rune: 'q'}, do)
	if !matched || !handled || !slices.Equal(actions, []keymap.Action{"short", "quit"}) {
		t.Fatalf("breaking key matched=%v handled=%v actions=%v", matched, handled, actions)
	}
}

func TestACompletedActionCanDeclineTheEvent(t *testing.T) {
	m := &keymap.Map{}
	m.Bind("maybe", input.Chord{Rune: 'm'})
	matched, handled := (&keymap.Matcher{}).Handle(m, input.Key{Rune: 'm'}, func(keymap.Action) bool {
		return false
	})
	if !matched || handled {
		t.Fatalf("matched=%v handled=%v, want a known action that declined", matched, handled)
	}
}

func TestBindingTheSameKeysAgainReplacesWhatTheyDid(t *testing.T) {
	m := &keymap.Map{}
	m.Bind("first", input.Ctrl.Rune('k'))
	m.Bind("second", input.Ctrl.Rune('k'))

	if action, _ := m.Action(input.Ctrl.Rune('k')); action != "second" {
		t.Fatalf("= %q, want the later binding", action)
	}
	if keys := m.Keys("first"); len(keys) != 0 {
		t.Fatalf("the replaced action still lists %v", keys)
	}
}

func TestUnbindingLeavesNothingBehindToSwallowAKeystroke(t *testing.T) {
	// A tree pruned in place keeps the node the sequence went through, and a node with
	// nothing under it takes the keystroke and names nothing — a key that stops working
	// for a reason nobody can see.
	m := &keymap.Map{}
	m.Bind("go-to-top", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	if !m.Unbind(input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'}) {
		t.Fatal("unbinding said there was nothing there")
	}
	if _, mine := match(&keymap.Matcher{}, m, input.Key{Rune: 'g'}); mine {
		t.Fatal("the first chord of the sequence is still being taken")
	}
	if m.Unbind(input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'}) {
		t.Fatal("unbinding the same thing twice said it was there the second time")
	}
}

func TestAnActionListsItsKeysInTheOrderTheyWereBound(t *testing.T) {
	// Order is part of the binding model rather than an iteration accident.
	m := &keymap.Map{}
	m.Bind("delete-word-back", input.Ctrl.Rune('w'))
	m.Bind("delete-word-back", input.Alt.With(input.Backspace))

	keys := m.Keys("delete-word-back")
	if len(keys) != 2 {
		t.Fatalf("= %v, want both", keys)
	}
	if keys[0].String() != "ctrl+w" || keys[1].String() != "alt+backspace" {
		t.Fatalf("= %q and %q, want them in the order they were bound", keys[0], keys[1])
	}
	if keys := m.Keys("never-bound"); keys != nil {
		t.Errorf("an action nothing was bound to lists %v", keys)
	}
}

func TestAnEmptyMapAndANilOneAnswerNothingRatherThanPanicking(t *testing.T) {
	var zero keymap.Map
	if _, mine := match(&keymap.Matcher{}, &zero, input.Key{Code: input.Enter}); mine {
		t.Error("an empty map claimed a keystroke")
	}
	var missing *keymap.Map
	if _, mine := match(&keymap.Matcher{}, missing, input.Key{Code: input.Enter}); mine {
		t.Error("a map nobody made claimed a keystroke")
	}
	if keys := missing.Keys("anything"); keys != nil {
		t.Error("a map nobody made listed keys")
	}
	if _, ok := missing.Action(input.Chord{Code: input.Enter}); ok {
		t.Error("a map nobody made named an action")
	}
}

func TestAnActionRetainsItsIdentifier(t *testing.T) {
	if got := keymap.Action("delete-word-back").String(); got != "delete-word-back" {
		t.Fatalf("= %q", got)
	}
}

func TestShiftAndTabAreOneKeystrokeHoweverTheTerminalSpelledIt(t *testing.T) {
	// The legacy sequence and the Kitty protocol report the same physical keystroke
	// two different ways. A binding can only name one of them, so the two are decoded
	// into one — or shift+tab moves the keyboard on half the terminals there are.
	m := &keymap.Map{}
	m.Bind("focus-prev", input.Shift.With(input.Tab))

	for _, bytes := range []string{"\x1b[Z", "\x1b[9;2u"} {
		var parser input.Parser
		events := parser.Feed([]byte(bytes))
		if len(events) != 1 {
			t.Fatalf("%q decoded as %d events", bytes, len(events))
		}
		key, ok := events[0].(input.Key)
		if !ok {
			t.Fatalf("%q did not decode as a keystroke", bytes)
		}
		if action, _ := match(&keymap.Matcher{}, m, key); action != "focus-prev" {
			t.Errorf("%q decoded as %v, which the binding does not match", bytes, key)
		}
	}
}

func TestAKeybindingSurvivesAConfigurationFile(t *testing.T) {
	// The reason the round trip has to be exact: a keybinding written to a file and
	// read back has to be the same keystroke. Implementing the text encoding is what
	// lets any of the usual decoders fill a map in without being told how.
	var keys map[keymap.Action]input.Keys
	if err := json.Unmarshal([]byte(`{
		"delete-word-back": "ctrl+w",
		"go-to-top":        "g g"
	}`), &keys); err != nil {
		t.Fatalf("reading a keybinding file: %v", err)
	}
	m := &keymap.Map{}
	for action, seq := range keys {
		m.Bind(action, seq...)
	}
	if action, _ := m.Action(input.Ctrl.Rune('w')); action != "delete-word-back" {
		t.Fatalf("ctrl+w = %q", action)
	}
	if action, _ := m.Action(input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'}); action != "go-to-top" {
		t.Fatalf("the two-chord binding = %q", action)
	}

	written, err := json.Marshal(keys)
	if err != nil {
		t.Fatalf("writing it back out: %v", err)
	}
	if !strings.Contains(string(written), `"ctrl+w"`) {
		t.Fatalf("written back as %s", written)
	}
}

func TestAConfigurationFileIsToldWhatItGotWrong(t *testing.T) {
	var chord input.Chord
	err := chord.UnmarshalText([]byte("contrl+a"))
	if err == nil {
		t.Fatal("a typo in a keybinding file was accepted")
	}
	if !strings.Contains(err.Error(), "contrl+a") {
		t.Fatalf("the error is %q, want it to name what it could not read", err)
	}
}
