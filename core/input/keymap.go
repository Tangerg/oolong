package input

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Action is one thing something can be asked to do, under a name.
//
// # Why a name and not a keystroke
//
// A widget that owns its keystrokes owns two things at once — what it can do, and
// what produces it — and can express neither of them on its own. There is nowhere to
// put a sequence like "g g", because a field holds one key; rebinding one chord means
// replacing the whole struct, because the struct is the binding; and the same widget
// in two places, where escape means "back" in one and "close" in the other, has to be
// told about both.
//
// So a widget names what it can do and answers to the name, and a [Keymap] says which
// keystrokes produce it. The two halves can then be replaced independently, which is
// the whole point: a program keeps the widgets and rebinds the keys, or keeps the keys
// and swaps the widget.
//
// # Naming one
//
// A name is an identifier and reads as one: lowercase words joined by hyphens.
// "delete-word-back", "select-all". Anything is allowed — an action nothing else has
// heard of is exactly what a program binds its own keys to — and the only rule is that
// the two halves agree on the spelling, which is what makes a constant worth declaring
// for one.
type Action string

// Does is the action in the words a hint row shows it in: the name, with the hyphens
// that make it an identifier taken out.
//
// There is no second field for a description, and no table mapping one to the other.
// A description held beside the name is a description that drifts from it, and the
// name already says what the thing does — that is what naming an action is. A program
// that wants other words, in another language or in its own house style, names its
// actions in them.
func (a Action) Does() string { return strings.ReplaceAll(string(a), "-", " ") }

// Chord is one keystroke: a key with the modifiers held down with it.
//
// It is a [Key] with everything that is not part of the identity taken off — which
// transition it was, what text the terminal said it produced, when it arrived. Those
// describe one occurrence of a keystroke; a chord describes which keystroke it was, so
// only a chord can be written down in advance and bound to something.
type Chord struct {
	Code Code
	Rune rune
	Mods Mods
}

// Rune is the chord of a character key held with these modifiers:
//
//	input.Ctrl.Rune('w')
func (m Mods) Rune(r rune) Chord { return Chord{Code: Character, Rune: r, Mods: m} }

// With is the chord of a named key held with these modifiers:
//
//	input.Alt.With(input.Enter)
//
// A key with nothing held is a chord literal, because there is no modifier to hang the
// call off: input.Chord{Code: input.Enter}.
func (m Mods) With(code Code) Chord { return Chord{Code: code, Mods: m} }

// Chord is the keystroke this event is one of.
func (k Key) Chord() Chord { return Chord{Code: k.Code, Rune: k.Rune, Mods: k.Mods} }

// String writes the chord the way a keybinding is conventionally written, and the way
// [ParseChord] reads one back: the modifiers, then the key.
func (c Chord) String() string {
	var b strings.Builder
	if mods := c.Mods.String(); mods != "" {
		b.WriteString(mods)
		b.WriteByte('+')
	}
	if c.Code != Character {
		b.WriteString(c.Code.String())
		return b.String()
	}
	if c.Rune == ' ' {
		b.WriteString("space")
	} else {
		b.WriteRune(c.Rune)
	}
	return b.String()
}

// MarshalText writes the chord as [Chord.String] does, so a keybinding survives being
// written to a configuration file and read back.
func (c Chord) MarshalText() ([]byte, error) { return []byte(c.String()), nil }

// UnmarshalText reads what MarshalText wrote.
//
// It is here rather than left to whatever reads the file because it is the same parse
// either way, and because implementing it is what makes a keymap something any of the
// usual decoders can fill in without being told how.
func (c *Chord) UnmarshalText(b []byte) error {
	chord, ok := ParseChord(string(b))
	if !ok {
		return notAKeystroke(string(b))
	}
	*c = chord
	return nil
}

// notAKeystroke is what a configuration file gets told when it names something nobody
// can press.
func notAKeystroke(s string) error {
	return errors.New("input: " + strconv.Quote(s) + " is not a keystroke")
}

// ParseChord reads what [Chord.String] writes, and reports whether it was a keystroke
// this package can name.
//
// It is the half of the round trip that makes keys configurable at run time: a
// keybinding read out of a file is a string, and turning it into something a keymap
// can hold is this. Nothing else in the repository needs it, which is why it is here
// rather than in whatever ends up reading the file.
func ParseChord(s string) (Chord, bool) {
	var mods Mods
	for {
		name, rest, found := strings.Cut(s, "+")
		if !found || name == "" {
			break
		}
		mod, ok := modNamed(name)
		if !ok {
			break
		}
		mods |= mod
		s = rest
	}
	switch s {
	case "":
		return Chord{}, false
	case "space":
		return Chord{Code: Character, Rune: ' ', Mods: mods}, true
	}
	if code, ok := codeNamed(s); ok {
		return Chord{Code: code, Mods: mods}, true
	}
	// One character and no more. A name this package does not know is not a key, and
	// guessing that the first rune of it was meant would bind "contrl+a" to "c".
	r := []rune(s)
	if len(r) != 1 {
		return Chord{}, false
	}
	return Chord{Code: Character, Rune: r[0], Mods: mods}, true
}

// modNamed is the modifier a name stands for, as [Mods.String] writes them.
func modNamed(name string) (Mods, bool) {
	switch name {
	case "ctrl":
		return Ctrl, true
	case "alt":
		return Alt, true
	case "shift":
		return Shift, true
	case "super":
		return Super, true
	}
	return 0, false
}

// codeNamed is the key a name stands for, as [Code.String] writes them.
func codeNamed(name string) (Code, bool) {
	if code, ok := codesByName[name]; ok {
		return code, true
	}
	if rest, ok := strings.CutPrefix(name, "f"); ok {
		if n, err := strconv.Atoi(rest); err == nil && n >= 1 && n <= 12 {
			return F1 + Code(n-1), true
		}
	}
	return 0, false
}

// codesByName is [codeNames] read the other way, built from it rather than written
// again so that the two cannot say different things.
//
// [Character] is left out on purpose. It is not a key anybody presses — it is what a
// key that produced text is called — and a name that parsed to it would be a chord
// with no character in it.
var codesByName = func() map[string]Code {
	out := make(map[string]Code, len(codeNames))
	for code, name := range codeNames {
		if code == Character {
			continue
		}
		out[name] = code
	}
	return out
}()

// Keys is a sequence of chords: one keystroke, or several typed one after another.
type Keys []Chord

// String writes the sequence with a space between the chords, which is how a
// keybinding file spells one and what [ParseKeys] reads back.
func (k Keys) String() string {
	parts := make([]string, len(k))
	for i, c := range k {
		parts[i] = c.String()
	}
	return strings.Join(parts, " ")
}

// MarshalText writes the sequence as [Keys.String] does.
func (k Keys) MarshalText() ([]byte, error) { return []byte(k.String()), nil }

// UnmarshalText reads what MarshalText wrote.
func (k *Keys) UnmarshalText(b []byte) error {
	keys, ok := ParseKeys(string(b))
	if !ok {
		return notAKeystroke(string(b))
	}
	*k = keys
	return nil
}

// ParseKeys reads a sequence: chords separated by spaces. A chord that is itself the
// space bar is written "space", so there is nothing ambiguous to split on.
func ParseKeys(s string) (Keys, bool) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil, false
	}
	out := make(Keys, 0, len(fields))
	for _, field := range fields {
		chord, ok := ParseChord(field)
		if !ok {
			return nil, false
		}
		out = append(out, chord)
	}
	return out, true
}

// DefaultKeyTimeout is how long a partly-typed sequence waits for the rest of itself
// when a keymap does not say.
//
// A second, which is what a text editor with sequences in it has settled on. It is long
// enough that nobody types "g g" too slowly by accident and short enough that a "g"
// left over from a minute ago does not turn the next one into something else.
const DefaultKeyTimeout = time.Second

// Pending is how far into a sequence the chords typed so far have got.
//
// It belongs to whoever is reading keys and not to the map they are read against. A
// map is a table, and tables are shared: two fields with the same bindings are two
// places a sequence can be half typed, and one of them finishing it must not finish
// the other's.
//
// The zero value is nothing typed yet.
type Pending struct {
	keys Keys
	at   time.Time
}

// Keys are the chords typed so far that have not yet named an action, which is what an
// interface showing a "waiting for the rest of it" hint would draw.
func (p *Pending) Keys() Keys { return slices.Clone(p.keys) }

// Clear abandons a partly-typed sequence, which is what to do when the keyboard goes
// somewhere else.
func (p *Pending) Clear() {
	if p != nil {
		p.keys, p.at = nil, time.Time{}
	}
}

// Keymap says which keystrokes produce which actions.
//
// It is a table and nothing else: it knows no widget, and every widget that reads keys
// through one answers the actions it recognises and ignores the rest. That is what lets
// one map serve a whole interface — a field, the container around it and the program's
// own keys, all in the same table — and what lets a program hand the same map to two
// widgets without either of them knowing.
//
// # Sequences
//
// A binding can be more than one chord long. The chords have to arrive within
// [Keymap.Timeout] of each other, judged by when the terminal's reader stamped them;
// a chord that does not continue what was being typed abandons it and is read on its
// own.
//
// A binding that is a proper prefix of a longer one is not reached. Nothing here can
// wake an interface up after a pause, so a chord that might still be the start of
// something longer can only be decided by what comes next — and deciding it as the
// shorter binding is deciding that the longer one never fires. The longer one wins, and
// the shorter is a binding that exists and never happens.
//
// The zero Keymap is empty and binds nothing. Use it by pointer: a map with bindings
// added to a copy of it would leave the original quietly sharing them.
type Keymap struct {
	// Timeout is how long a partly-typed sequence waits for the rest of itself. Zero
	// uses [DefaultKeyTimeout].
	Timeout time.Duration

	// bound is every binding in the order it was made, which is the order
	// [Keymap.Keys] answers in and therefore the order a hint row shows.
	bound []Binding
	// root is the same bindings as a tree, which is what a keystroke is looked up in.
	// It is rebuilt whenever bound changes: binding is rare, looking up happens on
	// every keystroke, and a tree pruned in place leaves prefixes that swallow keys
	// and name nothing.
	root *chordNode
}

// Binding is one entry of a map: a sequence of chords, and what it does.
type Binding struct {
	Keys   Keys
	Action Action
}

// String writes the binding the way a keybinding file does: what to press, then what it
// does.
func (b Binding) String() string { return b.Keys.String() + " " + string(b.Action) }

type chordNode struct {
	next   map[Chord]*chordNode
	action Action
}

// Bind makes a sequence of chords produce an action, replacing whatever those chords
// produced before.
//
// An action may have several sequences bound to it — Ctrl+W and Alt+Backspace both
// delete a word — and the order they are bound in is the order [Keymap.Keys] gives
// them back, so the first is the one a hint row shows.
func (m *Keymap) Bind(a Action, keys ...Chord) {
	if a == "" || len(keys) == 0 {
		return
	}
	seq := Keys(slices.Clone(keys))
	m.bound = slices.DeleteFunc(m.bound, func(b Binding) bool {
		return slices.Equal(b.Keys, seq)
	})
	m.bound = append(m.bound, Binding{Keys: seq, Action: a})
	m.rebuild()
}

// Unbind takes a sequence out, reporting whether it was there.
func (m *Keymap) Unbind(keys ...Chord) bool {
	before := len(m.bound)
	m.bound = slices.DeleteFunc(m.bound, func(b Binding) bool {
		return slices.Equal(b.Keys, Keys(keys))
	})
	if len(m.bound) == before {
		return false
	}
	m.rebuild()
	return true
}

// Keys are the sequences bound to an action, in the order they were bound. Nothing
// bound to it is no sequences, which is what a hint row skips.
func (m *Keymap) Keys(a Action) []Keys {
	if m == nil {
		return nil
	}
	var out []Keys
	for _, b := range m.bound {
		if b.Action == a {
			out = append(out, slices.Clone(b.Keys))
		}
	}
	return out
}

// Bindings is every binding in the map, in the order it was made. It is what a program
// showing its own keybinding list, or writing one back out to a file, asks for.
func (m *Keymap) Bindings() []Binding {
	if m == nil {
		return nil
	}
	out := make([]Binding, len(m.bound))
	for i, b := range m.bound {
		out[i] = Binding{Keys: slices.Clone(b.Keys), Action: b.Action}
	}
	return out
}

// Action is what a sequence names on its own, and whether it names anything.
//
// It is the lookup with no sequence under way and nothing remembered afterwards, which
// is what asking a question about the map — rather than reading keys through it — wants.
func (m *Keymap) Action(keys ...Chord) (Action, bool) {
	node, ok := m.follow(keys)
	if !ok || node.action == "" {
		return "", false
	}
	return node.action, true
}

// Lookup is what a keystroke means, following any sequence already under way.
//
// It reports the action the keystroke completed, and whether the keystroke was this
// map's at all. Those are not the same question: a chord that begins a sequence is the
// map's without naming an action yet, and comes back as an empty action that was
// nonetheless taken. A caller that passed it on instead would let the first half of a
// sequence act somewhere else as well.
//
//	switch action, mine := keys.Lookup(key, &w.pending); {
//	case !mine:
//		return false // not ours; let it go past
//	case action == "":
//		return true // the start of a sequence, waiting for the rest
//	}
//
// A nil [Pending] has nowhere to remember a half-typed sequence, so only what a chord
// names on its own is reachable through one.
//
// Releases are not keystrokes and name nothing: a map answers a key going down.
func (m *Keymap) Lookup(k Key, p *Pending) (Action, bool) {
	if m == nil || !k.Down() {
		return "", false
	}
	chord := k.Chord()

	// Where the chords already typed have got to, if they still count for anything.
	prefix := p.current(m.timeout(), k.At)
	node, ok := m.follow(append(slices.Clone(prefix), chord))
	if !ok && len(prefix) > 0 {
		// The chord does not continue what was being typed, so what was being typed is
		// over. The chord is read from the start rather than thrown away with it: the
		// user has plainly stopped spelling one thing and started another.
		prefix = nil
		node, ok = m.follow(Keys{chord})
	}
	p.Clear()
	if !ok {
		return "", false
	}
	if len(node.next) > 0 && p != nil {
		p.keys, p.at = append(slices.Clone(prefix), chord), k.At
		return "", true
	}
	if node.action == "" {
		return "", false
	}
	return node.action, true
}

// follow walks the tree to the node a sequence reaches.
func (m *Keymap) follow(keys Keys) (*chordNode, bool) {
	if m == nil || m.root == nil || len(keys) == 0 {
		return nil, false
	}
	node := m.root
	for _, chord := range keys {
		next, ok := node.next[chord]
		if !ok {
			return nil, false
		}
		node = next
	}
	return node, true
}

// rebuild makes the tree say what the bindings say.
func (m *Keymap) rebuild() {
	root := &chordNode{}
	for _, b := range m.bound {
		node := root
		for _, chord := range b.Keys {
			if node.next == nil {
				node.next = map[Chord]*chordNode{}
			}
			next, ok := node.next[chord]
			if !ok {
				next = &chordNode{}
				node.next[chord] = next
			}
			node = next
		}
		node.action = b.Action
	}
	m.root = root
}

func (m *Keymap) timeout() time.Duration {
	if m.Timeout > 0 {
		return m.Timeout
	}
	return DefaultKeyTimeout
}

// current is what has been typed so far, if it still counts for anything: nothing when
// no sequence is under way, and nothing when too long passed for the next chord to be
// part of the same one.
//
// A keystroke nothing timed cannot say how long it has been, so it never expires. The
// arrival time is stamped by whatever read the terminal, and a caller feeding a widget
// events it made up itself gets sequences with no deadline rather than sequences that
// can never be completed.
func (p *Pending) current(timeout time.Duration, now time.Time) Keys {
	if p == nil || len(p.keys) == 0 {
		return nil
	}
	if !p.at.IsZero() && !now.IsZero() && now.Sub(p.at) > timeout {
		return nil
	}
	return p.keys
}
