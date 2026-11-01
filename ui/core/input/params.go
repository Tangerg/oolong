package input

import (
	"image"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/ansi"
)

// ansi.Params owns syntax validation; these readers interpret terminal report
// semantics only after Valid accepts the complete section, including unused fields.
type params struct{ ansi.Params }

func parseParams(body string) params { return params{ansi.Parse(body)} }

// deviceAttributes reads what a terminal answered when asked what it is: a class,
// then the extensions it claims.
//
// A malformed number is left out rather than refused. Unlike a key report, where
// the wrong modifiers fire something nobody asked for, a claim that cannot be read
// is simply a claim nobody can act on — and the rest of the list is still worth
// having.
func (ps params) deviceAttributes() DeviceAttributes {
	class := max(ps.At(0), 0)
	features := make([]int, 0, max(ps.Len()-1, 0))
	for i := 1; i < ps.Len(); i++ {
		if group := ps.Group(i); group.Len() > 0 {
			features = append(features, group.At(0))
		}
	}
	return Attributes(class, features...)
}

// namesNoKey reports whether the group before the modifiers is what a report that
// names its key by its final byte may carry there.
//
// A cursor key and shift-tab put nothing in it: the field is absent, or it is the
// protocol's only value, one. Something else there is another sequence that happens
// to end in the same byte, and it used to arrive as the keystroke because only the
// modifier group was ever examined.
func (ps params) namesNoKey() bool {
	group := ps.Group(0)
	if group.Len() == 0 {
		return true
	}
	return group.Len() == 1 && group.At(0) <= 1
}

// keyMeta reads the modifier and transition group that key reports carry, and
// that the Kitty keyboard protocol also adds to arrow and numbered-key reports.
//
// It reports false for a group that is malformed rather than guessing: a key
// event with the wrong modifiers is worse than no key event, because it fires
// something the user did not ask for.
func (ps params) keyMeta() (Mods, Transition, bool) {
	group := ps.Group(1)
	if group.Len() == 0 {
		return 0, Press, true
	}
	if group.Len() > 2 {
		return 0, Press, false
	}

	var mods Mods
	if group.At(0) > 1 {
		// The encoding is the modifier bits plus one, so that a parameter of one
		// means no modifiers and the field is never empty.
		//
		// Masked before it is narrowed, not after. A terminal can put any number
		// here, and narrowing first would let a large one wrap into a modifier
		// nobody held — the same class of mistake as reading a rune out of an
		// integer and asking afterwards whether it was one.
		bits := group.At(0) - 1
		const shortcutMods = Shift | Alt | Ctrl | Super | Hyper | Meta
		const lockBits = 1<<6 | 1<<7
		// Caps/Num lock describe text generation; unsupported identity modifiers
		// must never become an unmodified key binding.
		if bits & ^(int(shortcutMods)|lockBits) != 0 {
			return 0, Press, false
		}
		mods = Mods(bits & int(shortcutMods))
	}
	if group.Len() < 2 {
		return mods, Press, true
	}
	switch group.At(1) {
	case 0, 1:
		return mods, Press, true
	case 2:
		return mods, Repeat, true
	case 3:
		return mods, Release, true
	default:
		return 0, Press, false
	}
}

func (ps params) text() (string, bool) {
	if ps.Len() < 3 {
		return "", true
	}
	var b strings.Builder
	for cp := range ps.Group(2).Values() {
		r, ok := codePoint(cp)
		if !ok || unicode.IsControl(r) {
			return "", false
		}
		b.WriteRune(r)
	}
	return b.String(), true
}

// codePoint turns a parsed number into the rune it names, reporting whether it
// names one at all.
//
// The range is checked on the number rather than on the rune, because converting
// is what destroys the evidence: a rune is 32 bits, so a code point of
// 0x100000041 narrows quietly to "A" and asking utf8.ValidRune afterwards asks
// about a value the terminal never sent. Every number in this package arrives
// from a sequence somebody else wrote, so every one of them gets this.
func codePoint(cp int) (rune, bool) {
	if cp < 0 || cp > utf8.MaxRune {
		return 0, false
	}
	r := rune(cp)
	return r, utf8.ValidRune(r)
}

// modifiedKey is code with the modifiers the sequence carried, or nil when the
// sequence names a key of its own as well — one keystroke has one name.
func (ps params) modifiedKey(code Code, extra Mods) Event {
	mods, transition, ok := ps.keyMeta()
	if !ok || !ps.namesNoKey() {
		return nil
	}
	return Key{Code: code, Mods: mods | extra, Transition: transition}
}

// extendedKey reads the Kitty keyboard protocol's key report, which is the
// only form that distinguishes releases from presses and can say what text a key
// produced.
func (ps params) extendedKey() Event {
	if ps.Len() > 3 {
		return nil
	}
	mods, transition, ok := ps.keyMeta()
	if !ok {
		return nil
	}
	key, ok := primaryKey(ps.Group(0), mods)
	if !ok {
		return nil
	}
	text, ok := ps.text()
	if !ok || key.Code == Character && key.Rune == 0 && text == "" {
		return nil
	}
	key.Text, key.Transition = text, transition
	return key
}

func primaryKey(group ansi.Parameter, mods Mods) (Key, bool) {
	if group.Len() == 0 || group.Len() > 3 {
		return Key{}, false
	}
	code, r, ok := extendedKeyCode(group.At(0))
	shifted, shiftedOK := codePoint(group.At(1))
	base, baseOK := codePoint(group.At(2))
	if !ok || !shiftedOK || !baseOK || shifted != 0 && !mods.Has(Shift) {
		return Key{}, false
	}
	if group.At(0) == 0 && (shifted != 0 || base != 0) {
		return Key{}, false
	}
	return Key{Code: code, Rune: r, ShiftedRune: shifted, BaseRune: base, Mods: mods}, true
}

func (ps params) mouse(down bool) Event {
	if ps.Len() != 3 || ps.Group(0).Len() != 1 || ps.Group(1).Len() != 1 || ps.Group(2).Len() != 1 {
		return nil
	}
	bits, x, y := ps.At(0), ps.At(1), ps.At(2)
	if bits < 0 || bits & ^127 != 0 || x < 1 || y < 1 {
		return nil
	}
	// The terminal counts from one; everything above this package counts from zero.
	ev := Mouse{Pos: image.Pt(x-1, y-1), Mods: mouseMods(bits)}
	switch {
	case bits&64 != 0:
		switch bits & 3 {
		case 0:
			ev.Action = WheelUp
		case 1:
			ev.Action = WheelDown
		default:
			return nil // horizontal wheel, which nothing here reads
		}
	case bits&32 != 0:
		ev.Button = mouseButton(bits & 3)
		if ev.Button == ButtonNone {
			ev.Action = MouseMove
		} else {
			ev.Action = MouseDrag
		}
	default:
		ev.Button = mouseButton(bits & 3)
		if down {
			ev.Action = MouseDown
		} else {
			ev.Action = MouseUp
		}
	}
	return ev
}

// report reads a control sequence that carried a private marker.
//
// Every one of these is a terminal answering rather than a person typing, so an
// unrecognised one is dropped. Reading it as a key would put whatever the terminal
// said into whatever had focus.
func (ps params) report(final byte) Event {
	switch {
	case ps.Marker() == '<' && (final == 'M' || final == 'm'):
		return ps.mouse(final == 'M')
	case ps.Marker() == '?' && final == 'c':
		return ps.deviceAttributes()
	case ps.Marker() == '?' && final == 'u' && ps.Len() == 1 && ps.Group(0).Len() == 1 && ps.Valid():
		return KeyboardFlags{Features: KeyboardFeatures(ps.At(0))}
	case ps.Marker() == '>' && final == 'c':
		return DeviceVersion{Kind: max(ps.At(0), 0), Version: max(ps.At(1), 0), Patch: max(ps.At(2), 0)}
	default:
		return nil
	}
}
