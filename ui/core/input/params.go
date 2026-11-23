package input

import (
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/ansi"
)

// ansi.Params owns syntax validation; these readers interpret terminal report
// semantics only after Valid accepts the complete section, including unused fields.
type params struct{ ansi.Params }

func parseParams(body string) params { return params{ansi.Parse(body)} }

// A key report's parameter groups: the key, then its modifiers and event type,
// then the text it produced. Only a Kitty report carries text.
const (
	keyField = iota
	modifierField
	textField
	keyReportFields
)

// functionalKeyFields is a key and its modifiers: a functional key produces no text.
const functionalKeyFields = textField

// The values in a Kitty report's key field: the key, the same key shifted in the
// active layout, and the key in the standard PC-101 layout.
const (
	keyCode = iota
	shiftedKey
	baseKey
	keyIdentities
)

// The values in a modifier field: the modifier bits plus one, then the event type.
const (
	modifierBits = iota
	eventType
	modifierValues
)

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
	group := ps.Group(keyField)
	if group.Len() == 0 {
		return true
	}
	return group.Len() == 1 && group.At(0) <= 1
}

// keyMeta rejects malformed metadata because guessed modifiers could execute
// a binding the user did not press.
func (ps params) keyMeta(dialect KeyboardDialect) (Mods, Transition, bool) {
	group := ps.Group(modifierField)
	if group.Len() == 0 {
		return 0, Press, true
	}
	if group.Len() > modifierValues {
		return 0, Press, false
	}

	var mods Mods
	if group.At(modifierBits) > 1 {
		// The encoding is the modifier bits plus one, so that a parameter of one
		// means no modifiers and the field is never empty.
		//
		// Masked before it is narrowed, not after. A terminal can put any number
		// here, and narrowing first would let a large one wrap into a modifier
		// nobody held — the same class of mistake as reading a rune out of an
		// integer and asking afterwards whether it was one.
		var ok bool
		mods, ok = dialect.modifiers(group.At(modifierBits) - 1)
		if !ok {
			return 0, Press, false
		}
	}
	if group.Len() <= eventType {
		return mods, Press, true
	}
	switch group.At(eventType) {
	case kittyEventDefault, kittyEventPress:
		return mods, Press, true
	case kittyEventRepeat:
		return mods, Repeat, true
	case kittyEventRelease:
		return mods, Release, true
	default:
		return 0, Press, false
	}
}

// Kitty event types follow the modifier field; an omitted type is a press.
const (
	kittyEventDefault = iota
	kittyEventPress
	kittyEventRepeat
	kittyEventRelease
)

func (ps params) text() (string, bool) {
	if ps.Len() <= textField {
		return "", true
	}
	var b strings.Builder
	for cp := range ps.Group(textField).Values() {
		r, ok := codePoint(cp)
		if !ok || !associatedTextRune(r) {
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
	if !ok || ps.Len() > functionalKeyFields {
		return nil
	}
	return Key{Code: code, Mods: mods | extra, Transition: transition}
}

func (ps params) extendedKey() Event {
	if ps.Len() > keyReportFields {
		return nil
	}
	mods, transition, ok := ps.keyMeta(KittyKeyboard)
	if !ok {
		return nil
	}
	key, ok := primaryKey(ps.Group(keyField), mods)
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
	if group.Len() == 0 || group.Len() > keyIdentities {
		return Key{}, false
	}
	code, r, ok := extendedKeyCode(group.At(keyCode))
	shifted, shiftedOK := codePoint(group.At(shiftedKey))
	base, baseOK := codePoint(group.At(baseKey))
	if !ok || !shiftedOK || !baseOK {
		return Key{}, false
	}
	key := Key{Code: code, Rune: r, ShiftedRune: shifted, BaseRune: base, Mods: mods}
	return key, key.validate() == nil
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

func (ps params) sgrMouse(final byte) Event {
	if !ps.scalars(mouseReportFields) {
		return nil
	}
	report := mouseReport{bits: ps.At(0), x: ps.At(1), y: ps.At(2), released: final == sgrMouseRelease}
	return report.event()
}

// Private-marker sequences are terminal reports. Unknown reports cannot fall
// through to keys without injecting unsolicited text into the focused control.
func (ps params) report(final byte) Event {
	switch {
	case ps.Marker() == sgrMouseMarker && (final == sgrMousePress || final == sgrMouseRelease):
		return ps.sgrMouse(final)
	case ps.Marker() == privateReplyMarker && final == deviceReplyFinal:
		return ps.deviceAttributes()
	case ps.Marker() == privateReplyMarker && final == kittyKeyFinal && ps.scalars(1):
		return KeyboardFlags{Features: KeyboardFeatures(ps.At(0))}
	case ps.Marker() == versionReplyMarker && final == deviceReplyFinal && ps.scalars(deviceVersionFields):
		return DeviceVersion{Kind: ps.At(0), Version: ps.At(1), Patch: ps.At(2)}
	default:
		return nil
	}
}

// Private markers that open terminal reports rather than keystrokes.
const (
	sgrMouseMarker     = '<'
	privateReplyMarker = ansi.DECPrivate
	versionReplyMarker = '>'

	deviceVersionFields = 3
)

func (ps params) legacyF3() Event {
	if !ps.scalars(functionalKeyFields) || ps.At(keyField) != 1 || ps.At(modifierField) <= 1 {
		return nil
	}
	return ps.functionalKey(F3, 0, XTermKeyboard)
}

func (d KeyboardDialect) modifiers(bits int) (Mods, bool) {
	const basic = Shift | Alt | Ctrl
	const legacyMetaBit = 1 << 3
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
