package input_test

import (
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
)

// at is a keystroke that arrived at a given moment, which is what a sequence is judged
// by. The zero time means nothing timed it.
func at(chord input.Chord, when time.Time) input.Key {
	return input.Key{Code: chord.Code, Rune: chord.Rune, Mods: chord.Mods, At: when}
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
	m := &input.Keymap{}
	m.Bind("delete-word-back", input.Ctrl.Rune('w'))

	action, mine := m.Lookup(input.Key{Code: input.Character, Rune: 'w', Mods: input.Ctrl}, &input.Pending{})
	if !mine || action != "delete-word-back" {
		t.Fatalf("ctrl+w = %q (mine=%v)", action, mine)
	}
	// A keystroke the map says nothing about is not the map's, which is what lets it
	// carry on to whatever else might want it.
	if _, mine := m.Lookup(input.Key{Code: input.Character, Rune: 'q'}, &input.Pending{}); mine {
		t.Error("the map claimed a keystroke nothing was bound to")
	}
	// A key coming back up is not a keystroke.
	up := input.Key{Code: input.Character, Rune: 'w', Mods: input.Ctrl, Transition: input.Release}
	if _, mine := m.Lookup(up, &input.Pending{}); mine {
		t.Error("the map answered a key being let go of")
	}
}

func TestASequenceIsFinishedByItsLastChord(t *testing.T) {
	m := &input.Keymap{}
	m.Bind("go-to-top", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	var pending input.Pending
	now := time.Unix(1700000000, 0)

	// The first chord is the map's and names nothing yet. Consumed all the same: a
	// caller that passed it on would let half a sequence act somewhere else too.
	action, mine := m.Lookup(at(input.Chord{Rune: 'g'}, now), &pending)
	if !mine || action != "" {
		t.Fatalf("the first chord = %q (mine=%v), want it taken and nothing done", action, mine)
	}
	if got := pending.Keys().String(); got != "g" {
		t.Fatalf("waiting on %q", got)
	}
	action, mine = m.Lookup(at(input.Chord{Rune: 'g'}, now.Add(50*time.Millisecond)), &pending)
	if !mine || action != "go-to-top" {
		t.Fatalf("the second chord = %q (mine=%v)", action, mine)
	}
	if got := pending.Keys(); len(got) != 0 {
		t.Fatalf("still waiting on %q after the sequence finished", got.String())
	}
}

func TestASequenceLeftTooLongIsOver(t *testing.T) {
	// The pause is the whole difference between a sequence and two keystrokes that
	// happen to be adjacent, and only the arrival time can tell them apart.
	m := &input.Keymap{Timeout: 100 * time.Millisecond}
	m.Bind("go-to-top", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	m.Bind("quit", input.Chord{Rune: 'q'})
	var pending input.Pending
	now := time.Unix(1700000000, 0)

	m.Lookup(at(input.Chord{Rune: 'g'}, now), &pending)
	action, mine := m.Lookup(at(input.Chord{Rune: 'g'}, now.Add(time.Second)), &pending)
	if action != "" {
		t.Fatalf("a chord a second later finished the sequence anyway: %q", action)
	}
	if !mine || pending.Keys().String() != "g" {
		t.Fatalf("the late chord did not begin the sequence again: %q (mine=%v)", pending.Keys().String(), mine)
	}

	// A chord that does not continue what was being typed is read on its own, because
	// the user has plainly stopped spelling one thing and started another.
	action, mine = m.Lookup(at(input.Chord{Rune: 'q'}, now.Add(time.Second)), &pending)
	if !mine || action != "quit" {
		t.Fatalf("the chord that broke off the sequence = %q (mine=%v)", action, mine)
	}
}

func TestAKeystrokeNothingTimedNeverGoesStale(t *testing.T) {
	// A caller feeding a widget events it made up itself has no clock in them, and a
	// sequence that could never be completed would be worse than one with no deadline.
	m := &input.Keymap{Timeout: time.Nanosecond}
	m.Bind("go-to-top", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	var pending input.Pending

	m.Lookup(input.Key{Rune: 'g'}, &pending)
	if action, _ := m.Lookup(input.Key{Rune: 'g'}, &pending); action != "go-to-top" {
		t.Fatalf("= %q, want the sequence completed", action)
	}
}

func TestWithNowhereToRememberOnlySingleChordsAreReachable(t *testing.T) {
	m := &input.Keymap{}
	m.Bind("go-to-top", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	m.Bind("quit", input.Chord{Rune: 'q'})

	if _, mine := m.Lookup(input.Key{Rune: 'g'}, nil); mine {
		t.Error("half a sequence was taken by a reader with nowhere to keep it")
	}
	if action, _ := m.Lookup(input.Key{Rune: 'q'}, nil); action != "quit" {
		t.Errorf("= %q, want a single chord to work regardless", action)
	}
}

func TestBindingTheSameKeysAgainReplacesWhatTheyDid(t *testing.T) {
	m := &input.Keymap{}
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
	m := &input.Keymap{}
	m.Bind("go-to-top", input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'})
	if !m.Unbind(input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'}) {
		t.Fatal("unbinding said there was nothing there")
	}
	if _, mine := m.Lookup(input.Key{Rune: 'g'}, &input.Pending{}); mine {
		t.Fatal("the first chord of the sequence is still being taken")
	}
	if m.Unbind(input.Chord{Rune: 'g'}, input.Chord{Rune: 'g'}) {
		t.Fatal("unbinding the same thing twice said it was there the second time")
	}
}

func TestAnActionListsItsKeysInTheOrderTheyWereBound(t *testing.T) {
	// A hint row shows the first, so the order is a decision and not an accident: the
	// chord that works on every terminal goes first.
	m := &input.Keymap{}
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
	var zero input.Keymap
	if _, mine := zero.Lookup(input.Key{Code: input.Enter}, &input.Pending{}); mine {
		t.Error("an empty map claimed a keystroke")
	}
	var missing *input.Keymap
	if _, mine := missing.Lookup(input.Key{Code: input.Enter}, &input.Pending{}); mine {
		t.Error("a map nobody made claimed a keystroke")
	}
	if keys := missing.Keys("anything"); keys != nil {
		t.Error("a map nobody made listed keys")
	}
	if _, ok := missing.Action(input.Chord{Code: input.Enter}); ok {
		t.Error("a map nobody made named an action")
	}
}

func TestAnActionSaysWhatItDoes(t *testing.T) {
	// There is no second field for a description. The name is what there is to say,
	// which is what stops the two from disagreeing.
	if got := input.Action("delete-word-back").Does(); got != "delete word back" {
		t.Fatalf("= %q", got)
	}
	if got := input.Action("send").Does(); got != "send" {
		t.Fatalf("= %q", got)
	}
}

func TestShiftAndTabAreOneKeystrokeHoweverTheTerminalSpelledIt(t *testing.T) {
	// The legacy sequence and the Kitty protocol report the same physical keystroke
	// two different ways. A binding can only name one of them, so the two are decoded
	// into one — or shift+tab moves the keyboard on half the terminals there are.
	m := &input.Keymap{}
	m.Bind("focus-prev", input.Shift.With(input.Tab))

	for _, bytes := range []string{"\x1b[Z", "\x1b[9;2u"} {
		key, ok := one(t, bytes).(input.Key)
		if !ok {
			t.Fatalf("%q did not decode as a keystroke", bytes)
		}
		if action, _ := m.Lookup(key, &input.Pending{}); action != "focus-prev" {
			t.Errorf("%q decoded as %v, which the binding does not match", bytes, key)
		}
	}
}
