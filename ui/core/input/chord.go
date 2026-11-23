package input

import (
	"errors"
	"fmt"
	"iter"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Chord excludes occurrence-specific text, transition, and arrival time so
// bindings compare only the identity that can be specified in advance.
type Chord struct {
	Code Code
	Rune rune
	Mods Mods
}

func (m Mods) Rune(r rune) Chord { return Chord{Code: Character, Rune: r, Mods: m} }

func (m Mods) With(code Code) Chord { return Chord{Code: code, Mods: m} }

func (k Key) Chord() Chord { return Chord{Code: k.Code, Rune: k.Rune, Mods: k.Mods} }

// A dedicated keypad binding takes precedence over the corresponding ordinary
// key. Keypad Begin has no ordinary equivalent.
var keypadKeys = map[Code]struct {
	name       string
	equivalent Chord
}{
	Keypad0:         {"kp-0", Chord{Rune: '0'}},
	Keypad1:         {"kp-1", Chord{Rune: '1'}},
	Keypad2:         {"kp-2", Chord{Rune: '2'}},
	Keypad3:         {"kp-3", Chord{Rune: '3'}},
	Keypad4:         {"kp-4", Chord{Rune: '4'}},
	Keypad5:         {"kp-5", Chord{Rune: '5'}},
	Keypad6:         {"kp-6", Chord{Rune: '6'}},
	Keypad7:         {"kp-7", Chord{Rune: '7'}},
	Keypad8:         {"kp-8", Chord{Rune: '8'}},
	Keypad9:         {"kp-9", Chord{Rune: '9'}},
	KeypadDecimal:   {"kp-decimal", Chord{Rune: '.'}},
	KeypadDivide:    {"kp-divide", Chord{Rune: '/'}},
	KeypadMultiply:  {"kp-multiply", Chord{Rune: '*'}},
	KeypadSubtract:  {"kp-subtract", Chord{Rune: '-'}},
	KeypadAdd:       {"kp-add", Chord{Rune: '+'}},
	KeypadEnter:     {"kp-enter", Chord{Code: Enter}},
	KeypadEqual:     {"kp-equal", Chord{Rune: '='}},
	KeypadSeparator: {"kp-separator", Chord{Rune: ','}},
	KeypadLeft:      {"kp-left", Chord{Code: Left}},
	KeypadRight:     {"kp-right", Chord{Code: Right}},
	KeypadUp:        {"kp-up", Chord{Code: Up}},
	KeypadDown:      {"kp-down", Chord{Code: Down}},
	KeypadPageUp:    {"kp-pageup", Chord{Code: PageUp}},
	KeypadPageDown:  {"kp-pagedown", Chord{Code: PageDown}},
	KeypadHome:      {"kp-home", Chord{Code: Home}},
	KeypadEnd:       {"kp-end", Chord{Code: End}},
	KeypadInsert:    {"kp-insert", Chord{Code: Insert}},
	KeypadDelete:    {"kp-delete", Chord{Code: Delete}},
	KeypadBegin:     {"kp-begin", Chord{}},
}

// ShortcutChords yields binding candidates in priority order. A local binding wins
// over a shifted symbol and the base layout. A dedicated keypad key wins over its
// ordinary equivalent. Text-only events have no shortcut identity.
func (k Key) ShortcutChords() iter.Seq[Chord] {
	return func(yield func(Chord) bool) {
		if k.Code == Character && k.Rune == 0 {
			return
		}
		if !yield(k.Chord()) {
			return
		}
		if k.Code != Character {
			equivalent := keypadKeys[k.Code].equivalent
			if equivalent != (Chord{}) {
				equivalent.Mods = k.Mods
				yield(equivalent)
			}
			return
		}
		if k.ShiftedRune != 0 && k.Mods.Has(Shift) && !yield((k.Mods &^ Shift).Rune(k.ShiftedRune)) {
			return
		}
		if k.BaseRune != 0 && k.BaseRune != k.Rune {
			yield(k.Mods.Rune(k.BaseRune))
		}
	}
}

// Typed is the text this key enters into an editor, or "" when it enters none. A
// release, a key held with any modifier other than Shift, and a key that names no
// character enter nothing. What the terminal reported the key produced wins over its
// code, because the code is the unshifted physical key: on a layout where the key
// beside "1" produces "@", the code would type "2".
func (k Key) Typed() string {
	if !k.Down() || k.Mods&^Shift != 0 {
		return ""
	}
	if k.Text != "" {
		return k.Text
	}
	if r := k.fallbackRune(); r != 0 {
		return string(r)
	}
	return ""
}

// fallbackRune is the character a key stands for when the terminal reported no
// associated text. A base-layout shortcut identity never supplies text.
func (k Key) fallbackRune() rune {
	if k.Code != Character {
		return keypadKeys[k.Code].equivalent.Rune
	}
	if k.ShiftedRune != 0 && k.Mods.Has(Shift) {
		return k.ShiftedRune
	}
	return k.Rune
}

// String and ParseChord share the configuration spelling so displayed bindings
// can be read back without a second naming convention.
// The spellings a chord is written in and read back from. A code point outside the
// printable set is written as fmt's %U writes it: the prefix and four to six hex digits.
const (
	modSeparator     = '+'
	chordSeparator   = ' '
	spaceName        = "space"
	codePointPrefix  = "U+"
	minCodePointHex  = 4
	maxCodePointHex  = 6
	codePointHexBase = 16
)

func (c Chord) String() string {
	var b strings.Builder
	if mods := c.Mods.String(); mods != "" {
		b.WriteString(mods)
		b.WriteByte(modSeparator)
	}
	if c.Code != Character {
		b.WriteString(c.Code.String())
		return b.String()
	}
	switch {
	case c.Rune == ' ':
		b.WriteString(spaceName)
	case unicode.IsSpace(c.Rune) || !unicode.IsPrint(c.Rune):
		// Keep controls and separators out of help output and sequence syntax.
		fmt.Fprintf(&b, "%U", c.Rune)
	default:
		b.WriteRune(c.Rune)
	}
	return b.String()
}

// Validate shares the configuration grammar so programmatic bindings cannot name
// a shortcut that input and persisted configuration cannot represent.
func (c Chord) Validate() error {
	written := c.String()
	if read, ok := ParseChord(written); !ok || read != c {
		return fmt.Errorf("input: chord {Code:%d Rune:%U Mods:%#x} cannot round-trip through %q", c.Code, c.Rune, uint8(c.Mods), written)
	}
	return nil
}

// MarshalText writes the chord as [Chord.String] does, so a keybinding survives being
// written to a configuration file and read back.
func (c Chord) MarshalText() ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return []byte(c.String()), nil
}

// UnmarshalText shares ParseChord so configuration decoders cannot introduce
// a second chord grammar.
func (c *Chord) UnmarshalText(b []byte) error {
	chord, ok := ParseChord(string(b))
	if !ok {
		return notAKeystroke(string(b))
	}
	*c = chord
	return nil
}

func notAKeystroke(s string) error {
	return errors.New("input: " + strconv.Quote(s) + " is not a keystroke")
}

// ParseChord reads what [Chord.String] writes, and reports whether it was a keystroke
// this package can name.
//
// Together with String it supplies a stable configuration representation without
// coupling this package to a particular decoder or configuration format.
func ParseChord(s string) (Chord, bool) {
	var mods Mods
	for {
		name, rest, found := strings.Cut(s, string(modSeparator))
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
	case spaceName:
		return Chord{Code: Character, Rune: ' ', Mods: mods}, true
	}
	if code, ok := codeNamed(s); ok {
		return Chord{Code: code, Mods: mods}, true
	}
	// One character and no more. A name this package does not know is not a key, and
	// guessing that the first rune of it was meant would bind "contrl+a" to "c".
	r, ok := parseRune(s)
	if !ok {
		return Chord{}, false
	}
	return Chord{Code: Character, Rune: r, Mods: mods}, true
}

// parseRune refuses control characters: no key reports one as its identity, so
// a chord naming one could never match and its binding would be silently dead.
func parseRune(s string) (rune, bool) {
	r, ok := spelledRune(s)
	return r, ok && !unicode.IsControl(r)
}

func spelledRune(s string) (rune, bool) {
	if code, ok := strings.CutPrefix(s, codePointPrefix); ok && len(code) >= minCodePointHex && len(code) <= maxCodePointHex {
		value, err := strconv.ParseUint(code, codePointHexBase, 32)
		if err == nil && value <= unicode.MaxRune && utf8.ValidRune(rune(value)) {
			return rune(value), true
		}
	}
	r, size := utf8.DecodeRuneInString(s)
	if size != len(s) || r == utf8.RuneError && size == 1 {
		return 0, false
	}
	return r, true
}

func modNamed(name string) (Mods, bool) {
	for _, named := range modifierNames {
		if named.name == name {
			return named.mod, true
		}
	}
	return 0, false
}

func codeNamed(name string) (Code, bool) {
	code, ok := codesByName[name]
	return code, ok
}

// codesByName is [Code.String] read the other way, built from it rather than
// written again so that the two cannot say different things. Only the spelling
// String writes parses: "f01" or "f+1" name no key.
//
// [Character] is left out on purpose. It is not a key anybody presses — it is what a
// key that produced text is called — and a name that parsed to it would be a chord
// with no character in it.
var codesByName = func() map[string]Code {
	out := make(map[string]Code, int(KeypadBegin))
	for code := Character + 1; code <= KeypadBegin; code++ {
		out[code.String()] = code
	}
	return out
}()

type Keys []Chord

// String writes the sequence with a space between the chords, which is how a
// keybinding file spells one and what [ParseKeys] reads back.
func (k Keys) String() string {
	parts := make([]string, len(k))
	for i, c := range k {
		parts[i] = c.String()
	}
	return strings.Join(parts, string(chordSeparator))
}

func (k Keys) MarshalText() ([]byte, error) {
	written := k.String()
	if read, ok := ParseKeys(written); !ok || !slices.Equal(k, read) {
		return nil, notAKeystroke(written)
	}
	return []byte(written), nil
}

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
