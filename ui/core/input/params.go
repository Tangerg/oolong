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

const (
	mouseLeftButton = iota
	mouseMiddleButton
	mouseRightButton
	mouseNoButton
)

const (
	mouseShiftBit = 1 << (iota + 2)
	mouseAltBit
	mouseCtrlBit
	mouseMotionBit
	mouseWheelBit
)

const (
	mouseButtonMask = mouseNoButton
	mouseReportMask = mouseButtonMask | mouseShiftBit | mouseAltBit | mouseCtrlBit | mouseMotionBit | mouseWheelBit
	mouseWheelUp    = 0
	mouseWheelDown  = 1
)

func parseParams(body string) params { return params{ansi.Parse(body)} }

// Malformed capability claims are omitted independently: unlike malformed key
// modifiers, an unreadable claim cannot trigger an unintended user action.
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

// Final-byte keys permit only an absent or one-valued leading group. Other
// values may identify unrelated replies and must not become keystrokes.
func (ps params) namesNoKey() bool {
	group := ps.Group(0)
	if group.Len() == 0 {
		return true
	}
	return group.Len() == 1 && group.At(0) <= 1
}

// keyMeta rejects malformed metadata because guessed modifiers could execute
// a binding the user did not press.
func (ps params) keyMeta(dialect KeyboardDialect) (Mods, Transition, bool) {
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
		var ok bool
		mods, ok = dialect.modifiers(group.At(0) - 1)
		if !ok {
			return 0, Press, false
		}
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

// Validate before converting to rune: narrowing an out-of-range wire integer
// can turn it into a valid but different Unicode scalar.
func codePoint(cp int) (rune, bool) {
	if cp < 0 || cp > utf8.MaxRune {
		return 0, false
	}
	r := rune(cp)
	return r, utf8.ValidRune(r)
}

// modifiedKey is code with the modifiers the sequence carried, or nil when the
// sequence names a key of its own as well — one keystroke has one name.
func (ps params) modifiedKey(code Code, extra Mods, dialect KeyboardDialect) Event {
	if !ps.namesNoKey() {
		return nil
	}
	return ps.functionalKey(code, extra, dialect)
}

func (ps params) functionalKey(code Code, extra Mods, dialect KeyboardDialect) Event {
	mods, transition, ok := ps.keyMeta(dialect)
	if !ok || ps.Len() > 2 {
		return nil
	}
	return Key{Code: code, Mods: mods | extra, Transition: transition}
}

func (ps params) extendedKey() Event {
	if ps.Len() > 3 {
		return nil
	}
	mods, transition, ok := ps.keyMeta(KittyKeyboard)
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

func (ps params) scalars(count int) bool {
	if ps.Len() != count {
		return false
	}
	for i := range count {
		if group := ps.Group(i); group.Len() != 1 || group.At(0) < 0 {
			return false
		}
	}
	return true
}

func (ps params) mouse(down bool) Event {
	if !ps.scalars(3) {
		return nil
	}
	bits, x, y := ps.At(0), ps.At(1), ps.At(2)
	if bits & ^mouseReportMask != 0 || x < 1 || y < 1 {
		return nil
	}
	// The terminal counts from one; everything above this package counts from zero.
	ev := Mouse{Pos: image.Pt(x-1, y-1), Mods: mouseMods(bits)}
	switch {
	case bits&mouseWheelBit != 0:
		switch bits & mouseButtonMask {
		case mouseWheelUp:
			ev.Action = WheelUp
		case mouseWheelDown:
			ev.Action = WheelDown
		default:
			return nil // horizontal wheel, which nothing here reads
		}
	case bits&mouseMotionBit != 0:
		ev.Button = mouseButton(bits & mouseButtonMask)
		if ev.Button == ButtonNone {
			ev.Action = MouseMove
		} else {
			ev.Action = MouseDrag
		}
	default:
		ev.Button = mouseButton(bits & mouseButtonMask)
		if down {
			ev.Action = MouseDown
		} else {
			ev.Action = MouseUp
		}
	}
	return ev
}

// Private-marker sequences are terminal reports. Unknown reports cannot fall
// through to keys without injecting unsolicited text into the focused control.
func (ps params) report(final byte) Event {
	switch {
	case ps.Marker() == '<' && (final == 'M' || final == 'm'):
		return ps.mouse(final == 'M')
	case ps.Marker() == '?' && final == 'c':
		return ps.deviceAttributes()
	case ps.Marker() == '?' && final == 'u' && ps.scalars(1):
		return KeyboardFlags{Features: KeyboardFeatures(ps.At(0))}
	case ps.Marker() == '>' && final == 'c' && ps.scalars(3):
		return DeviceVersion{Kind: ps.At(0), Version: ps.At(1), Patch: ps.At(2)}
	default:
		return nil
	}
}

func (ps params) legacyF3() Event {
	if !ps.scalars(2) || ps.At(0) != 1 || ps.At(1) <= 1 {
		return nil
	}
	return ps.functionalKey(F3, 0, XTermKeyboard)
}

func (d KeyboardDialect) modifiers(bits int) (Mods, bool) {
	const basic = Shift | Alt | Ctrl
	const legacyMetaBit = 1 << 3
	const shortcutMods = basic | Super | Hyper | Meta
	const capsLockBit = 1 << 6
	const numLockBit = 1 << 7
	const lockBits = capsLockBit | numLockBit
	switch d {
	case XTermKeyboard:
		if bits & ^(int(basic)|legacyMetaBit) != 0 {
			return 0, false
		}
		mods := Mods(bits & int(basic))
		if bits&legacyMetaBit != 0 {
			mods |= Meta
		}
		return mods, true
	case KittyKeyboard:
		if bits & ^(int(shortcutMods)|lockBits) != 0 {
			return 0, false
		}
		return Mods(bits & int(shortcutMods)), true
	default:
		return 0, false
	}
}
