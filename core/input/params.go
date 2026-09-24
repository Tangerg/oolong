package input

import (
	"image"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/oolong/core/ansi"
)

// params is a control sequence's parameter section: the syntax, which is shared
// with everything else that reads a sequence, and the meanings that only a
// terminal's own reports have.
//
// The split is what keeps the two apart. Which bytes are parameters and what an
// empty field means are facts about the wire and belong to nobody in particular;
// that the second group of a key report is a modifier mask plus one is a fact about
// keyboards, and belongs here.
//
// The readers below that make a keystroke are reached only for a section
// [ansi.Params.Valid] has already accepted, so none of them asks again whether a
// number is a number. That question has one owner: a key form only ever looks at the
// groups it uses, and what it does not look at is exactly where an unreadable number
// went unnoticed.
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
		// Caps/Num lock describe text generation; unsupported identity modifiers
		// must never become an unmodified key binding.
		if bits & ^(int(Shift|Alt|Ctrl|Super)|64|128) != 0 {
			return 0, Press, false
		}
		mods = Mods(bits & int(Shift|Alt|Ctrl|Super))
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

// text reads the associated-text group of a Kitty key report: the code points the
// key produced, which the terminal is better placed to know than this program is.
func (ps params) text() (string, bool) {
	if ps.Len() < 3 {
		return "", true
	}
	var b strings.Builder
	for cp := range ps.Group(2).Values() {
		if cp == 0 {
			continue
		}
		r, ok := codePoint(cp)
		if !ok {
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
	if ps.Len() == 0 || ps.Len() > 3 {
		return nil // a bare sequence here is a cursor report, not a key
	}
	primary := ps.Group(0)
	if primary.Len() == 0 || primary.Len() > 3 {
		return nil
	}
	// Alternate key codes are accepted and then ignored: reporting the key that
	// was pressed is this type's job, and reporting which key it would have been
	// under another layout is not.
	for i := 1; i < primary.Len(); i++ {
		alternate := primary.At(i)
		if _, ok := codePoint(alternate); alternate != 0 && !ok {
			return nil
		}
	}
	mods, transition, ok := ps.keyMeta()
	if !ok {
		return nil
	}
	text, ok := ps.text()
	if !ok {
		return nil
	}
	if primary.At(0) == 0 {
		if text == "" {
			return nil
		}
		return Key{Code: Character, Text: text, Mods: mods, Transition: transition}
	}
	code, r, ok := extendedKeyCode(primary.At(0))
	if !ok {
		return nil
	}
	return Key{Code: code, Rune: r, Mods: mods, Transition: transition, Text: text}
}

// mouse reads an SGR mouse report. down distinguishes the final byte that
// means "went down or moved" from the one that means "came up".
func (ps params) mouse(down bool) Event {
	if ps.Len() < 3 {
		return nil
	}
	bits, x, y := ps.At(0), ps.At(1), ps.At(2)
	if bits < 0 || bits & ^127 != 0 || x < 0 || y < 0 {
		return nil // a malformed report says nothing about where the mouse is
	}
	// The terminal counts from one; everything above this package counts from zero.
	ev := Mouse{Pos: image.Pt(max(x-1, 0), max(y-1, 0)), Mods: mouseMods(bits)}
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
	case ps.Marker() == '?' && final == 'u':
		return KeyboardFlags{Features: KeyboardFeatures(max(ps.At(0), 0))}
	case ps.Marker() == '>' && final == 'c':
		return DeviceVersion{Kind: max(ps.At(0), 0), Version: max(ps.At(1), 0), Patch: max(ps.At(2), 0)}
	default:
		return nil
	}
}
