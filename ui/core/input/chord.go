package input

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

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
	switch {
	case c.Rune == ' ':
		b.WriteString("space")
	case unicode.IsSpace(c.Rune) || !unicode.IsPrint(c.Rune):
		// Keep controls and separators out of help output and sequence syntax.
		fmt.Fprintf(&b, "%U", c.Rune)
	default:
		b.WriteRune(c.Rune)
	}
	return b.String()
}

// MarshalText writes the chord as [Chord.String] does, so a keybinding survives being
// written to a configuration file and read back.
func (c Chord) MarshalText() ([]byte, error) {
	written := c.String()
	if read, ok := ParseChord(written); !ok || read != c {
		return nil, notAKeystroke(written)
	}
	return []byte(written), nil
}

// UnmarshalText reads what MarshalText wrote.
//
// It is here rather than left to a configuration reader because the parse is part of
// the chord's text representation and can therefore be reused by any decoder.
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
	r, ok := parseRune(s)
	if !ok {
		return Chord{}, false
	}
	return Chord{Code: Character, Rune: r, Mods: mods}, true
}

func parseRune(s string) (rune, bool) {
	if code, ok := strings.CutPrefix(s, "U+"); ok && len(code) >= 4 && len(code) <= 6 {
		value, err := strconv.ParseUint(code, 16, 32)
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
func (k Keys) MarshalText() ([]byte, error) {
	written := k.String()
	if read, ok := ParseKeys(written); !ok || !slices.Equal(k, read) {
		return nil, notAKeystroke(written)
	}
	return []byte(written), nil
}

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
