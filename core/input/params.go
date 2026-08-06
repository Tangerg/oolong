package input

import (
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
	class := max(ps.First(), 0)
	features := make([]int, 0, max(ps.Count()-1, 0))
	for i := 1; i < ps.Count(); i++ {
		if group := ps.Group(i); len(group) > 0 {
			features = append(features, group[0])
		}
	}
	return Attributes(class, features...)
}

// keyMeta reads the modifier and transition group that key reports carry, and
// that the Kitty keyboard protocol also adds to arrow and numbered-key reports.
//
// It reports false for a group that is malformed rather than guessing: a key
// event with the wrong modifiers is worse than no key event, because it fires
// something the user did not ask for.
func (ps params) keyMeta() (Mods, Transition, bool) {
	group := ps.Group(1)
	if len(group) == 0 {
		return 0, Press, true
	}
	if len(group) > 2 || group[0] < 0 {
		return 0, Press, false
	}

	var mods Mods
	if group[0] > 1 {
		// The encoding is the modifier bits plus one, so that a parameter of one
		// means no modifiers and the field is never empty.
		//
		// Masked before it is narrowed, not after. A terminal can put any number
		// here, and narrowing first would let a large one wrap into a modifier
		// nobody held — the same class of mistake as reading a rune out of an
		// integer and asking afterwards whether it was one.
		mods = Mods((group[0] - 1) & int(Shift|Alt|Ctrl|Super))
	}
	if len(group) < 2 {
		return mods, Press, true
	}
	switch group[1] {
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
	if ps.Count() < 3 {
		return "", true
	}
	var b strings.Builder
	for _, cp := range ps.Group(2) {
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
