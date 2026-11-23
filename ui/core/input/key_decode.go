package input

import (
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/ansi"
)

// Final bytes of the control sequences a terminal reports input with.
const (
	focusInFinal      = 'I'
	focusOutFinal     = 'O'
	cursorReportFinal = 'R'
	kittyKeyFinal     = 'u'
	numberedKeyFinal  = '~'
	backtabFinal      = 'Z'
	deviceReplyFinal  = byte(ansi.DA)
)

// ss3Introducer is the byte after Escape that begins a single-shift key report,
// which only input reads.
const ss3Introducer = 'O'

// rxvt reports a modified numbered key by replacing its tilde: '$' adds Shift,
// '^' Ctrl and '@' both. '$' is an intermediate byte, so without this reading the
// sequence would never complete and its bytes would wait for an unrelated final.
const (
	rxvtShifted        = '$'
	rxvtControl        = '^'
	rxvtControlShifted = '@'
)

// decode consumes nothing when more bytes may change the result. A nil event
// with consumed bytes means recognized input that deliberately produces no event.
func (p *Parser) decode(b []byte, trigger parseTrigger) (n int, ev Event, done bool) {
	switch c := b[0]; {
	case c == esc:
		return p.decodeEscape(b, trigger)
	case c == '\r':
		return 1, Key{Code: Enter}, true
	case c == '\t':
		return 1, Key{Code: Tab}, true
	case c == ansi.Delete, c == '\b':
		return 1, Key{Code: Backspace}, true
	case ansi.C0(c):
		// Enter, Tab and Backspace are C0 codes too and are answered above, where
		// their own names are the truer report.
		return 1, Key{Code: Character, Rune: ctrlRune(c), Mods: Ctrl}, true
	default:
		if !utf8.FullRune(b) {
			return 0, nil, false
		}
		r, size := utf8.DecodeRune(b)
		if r == utf8.RuneError && size == 1 || unicode.IsControl(r) {
			// Invalid UTF-8, and the C1 controls and DEL that no key types, are
			// dropped rather than reported as printable keys.
			return size, nil, true
		}
		return size, Key{Code: Character, Rune: r}, true
	}
}

// ctrlRune names the key held with Ctrl to send the C0 code c. The code is that
// key's ASCII value with its upper bits cleared, so setting ctrlBit restores
// it; letters are named in lower case like every other key. NUL is named as
// the space bar, which sends it more commonly than the at sign does.
func ctrlRune(c byte) rune {
	const ctrlBit = 0x40
	if c == 0 {
		return ' '
	}
	return unicode.ToLower(rune(c | ctrlBit))
}

func (p *Parser) decodeEscape(b []byte, trigger parseTrigger) (n int, ev Event, done bool) {
	if len(b) == 1 {
		return 0, nil, false
	}
	n, ev, done = p.decodeIntroduced(b)
	if done || trigger == inputAvailable {
		return n, ev, done
	}
	return p.expireIntroduced(b, trigger)
}

// expireIntroduced settles an introducer whose sequence did not complete. A bare
// introducer is an Alt chord: ESC [ names Alt+[ as well as a CSI prefix. When the
// Escape wait expires after body bytes arrived inside it, those bytes are part of
// a protocol unit the transport split, not keys anybody pressed, so they are never
// replayed as typing: a control sequence is discarded through its final byte, and
// a string head is discarded on its own. At the end of input there is no timing
// evidence, and the bytes are read as the Alt chord and keys they spell.
func (p *Parser) expireIntroduced(b []byte, trigger parseTrigger) (int, Event, bool) {
	switch second := b[1]; {
	case second == esc:
		return 1, Key{Code: Esc}, true
	case len(b) == introducerLen, trigger == inputEnded:
		return decodeAlt(b)
	case second == ansi.ControlIntroducer:
		p.frame = overrunFrame
		return len(b), nil, true
	case second == byte(ansi.OSC), second == byte(ansi.DCS), second == byte(ansi.APC):
		return len(b), nil, true
	default:
		return decodeAlt(b)
	}
}

func (p *Parser) decodeIntroduced(b []byte) (n int, ev Event, done bool) {
	switch second := b[1]; {
	case second == ansi.ControlIntroducer:
		return p.decodeControl(b)
	case second == byte(ansi.OSC):
		cmd, size := oscHead(b)
		return p.introduceString(b, ansi.OSC, cmd, size)
	case second == byte(ansi.DCS):
		return p.introduceString(b, ansi.DCS, 0, dcsHead(b))
	case second == byte(ansi.APC):
		return p.introduceString(b, ansi.APC, 0, graphicsHead(b))
	case second == ss3Introducer:
		return decodeSS3(b)
	case second == esc:
		return p.decodeMetaPrefixed(b)
	case ansi.C0(second), second == ansi.Delete:
		consumed, event, ready := p.decode(b[1:], inputAvailable)
		if key, ok := event.(Key); ok {
			key.Mods |= Alt
			event = key
		}
		return consumed + 1, event, ready
	default:
		return decodeAlt(b)
	}
}

// decodeSS3 reads ESC O <final>, and the legacy form that carries an xterm
// modifier parameter as one digit before the final. A byte that cannot continue
// the sequence proves ESC O was Alt+O; it is left to be read on its own terms.
func decodeSS3(b []byte) (n int, ev Event, done bool) {
	if len(b) <= introducerLen {
		return 0, nil, false
	}
	at, mods := introducerLen, Mods(0)
	if digit := b[at]; digit >= '1' && digit <= '9' {
		if len(b) <= at+1 {
			return 0, nil, false
		}
		modifiers, ok := XTermKeyboard.modifiers(int(digit - '1'))
		if !ok {
			return decodeAlt(b)
		}
		at, mods = at+1, modifiers
	}
	if !ansi.Final(b[at]) {
		return decodeAlt(b)
	}
	code, ok := finalByteKeys[b[at]]
	if !ok {
		return at + 1, nil, true
	}
	return at + 1, Key{Code: code, Mods: mods}, true
}

// decodeMetaPrefixed reads two Escapes in a row: terminals that send Meta as an
// Escape prefix report Alt+Up as an Escape before ESC [ A. Only a following CSI or
// SS3 introducer makes the first Escape a modifier; otherwise it is a key of its own.
func (p *Parser) decodeMetaPrefixed(b []byte) (n int, ev Event, done bool) {
	// The second Escape opens the introducer that follows the first.
	if len(b) <= introducerLen {
		return 0, nil, false
	}
	if b[introducerLen] != ansi.ControlIntroducer && b[introducerLen] != ss3Introducer {
		return 1, Key{Code: Esc}, true
	}
	consumed, event, ready := p.decodeIntroduced(b[1:])
	if !ready {
		return 0, nil, false
	}
	if key, ok := event.(Key); ok {
		key.Mods |= Alt
		event = key
	}
	return consumed + 1, event, true
}

func decodeAlt(b []byte) (n int, ev Event, done bool) {
	if !utf8.FullRune(b[1:]) {
		return 0, nil, false
	}
	r, size := utf8.DecodeRune(b[1:])
	if r == utf8.RuneError && size == 1 || unicode.IsControl(r) {
		return 1, Key{Code: Esc}, true
	}
	return 1 + size, Key{Code: Character, Rune: r, Mods: Alt}, true
}

func (p *Parser) decodeControl(b []byte) (n int, ev Event, done bool) {
	i := max(introducerLen, p.controlAt)
	for i < len(b) && ansi.Body(b[i]) {
		if b[i] == rxvtShifted && rxvtKeyNumber(b[introducerLen:i]) {
			return i + 1, parseParams(string(b[introducerLen:i])).numberedKey(p.keyboard, Shift), true
		}
		i++
	}
	if i-introducerLen > maxParameterBytes {
		// Overran what one may hold. The check does not wait to see whether a
		// final byte happens to be in this chunk, and it does not drop "whatever
		// is buffered": either would make the cap depend on where the read split,
		// and the same bytes would decode differently for arriving in different
		// pieces. What is dropped is the introducer and the body seen so far, and
		// [Parser.skipParams] drops the rest wherever it turns up.
		p.frame = overrunFrame
		return i, nil, true
	}
	if i >= len(b) {
		p.controlAt = i
		return 0, nil, false
	}
	final := b[i]
	if !ansi.Final(final) {
		// A byte that cannot appear in a control sequence. Drop the malformed
		// prefix and start again at the byte that proved it malformed.
		return i, nil, true
	}
	n = i + 1
	switch string(b[:n]) {
	case pasteOpen:
		p.frame = pasteFrame
		return n, nil, true
	case x10MouseIntro:
		p.frame = x10MouseFrame
		return n, nil, true
	}
	piece, _, valid := ansi.Next(string(b[:n]))
	if !valid || piece.Kind != ansi.Control || piece.Intermediates != "" {
		return n, nil, true
	}
	ps := parseParams(piece.Parameters)

	return n, p.controlEvent(ps, final), true
}

func (p *Parser) controlEvent(ps params, final byte) Event {
	// A private marker says the sequence is a report and not a key, because a key
	// never carries one. Dispatching on the final byte alone let a keyboard-flags
	// reply — "CSI ? 31 u" — decode as a keystroke of an invisible control character.
	if ps.Marker() != 0 {
		event := ps.report(final)
		if _, ok := event.(KeyboardFlags); ok {
			p.keyboard = KittyKeyboard
		}
		return event
	}

	// Everything past here turns a sequence into a keystroke, and a keystroke nobody
	// pressed is worse than none — so the whole parameter section has to be readable,
	// not merely the groups this form looks at. The group a form ignores is the one
	// something pretending to be a terminal is free to fill.
	//
	// A report is the other way round: an extension nobody can read is a claim nobody
	// can act on, and the rest of the list is still worth having. See
	// [params.deviceAttributes].
	if !ps.Valid() {
		return nil
	}

	switch {
	case ps.Len() == 0 && final == focusInFinal:
		return FocusIn{}
	case ps.Len() == 0 && final == focusOutFinal:
		return FocusOut{}
	}

	switch final {
	case cursorReportFinal:
		if p.keyboard != XTermKeyboard {
			return nil
		}
		return ps.legacyF3()
	case kittyKeyFinal:
		return ps.extendedKey()
	case numberedKeyFinal:
		return ps.numberedKey(p.keyboard, 0)
	case rxvtControl, rxvtControlShifted:
		return ps.numberedKey(p.keyboard, rxvtModifiers(final))
	case backtabFinal:
		// Shift and tab, not a key of its own. A terminal speaking the Kitty protocol
		// reports the same keystroke as tab with shift held, and one keystroke that
		// arrives under two names is one nothing can be bound to.
		return ps.modifiedKey(Tab, Shift, p.keyboard)
	default:
		code, ok := finalByteKeys[final]
		if !ok {
			return nil
		}
		return ps.modifiedKey(code, 0, p.keyboard)
	}
}

func rxvtModifiers(final byte) Mods {
	if final == rxvtControlShifted {
		return Ctrl | Shift
	}
	return Ctrl
}

func rxvtKeyNumber(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	for _, c := range body {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (ps params) numberedKey(dialect KeyboardDialect, extra Mods) Event {
	code, ok := tildeKeyNumbers[ps.At(keyField)]
	if !ok || ps.Group(keyField).Len() != 1 {
		return nil
	}
	return ps.functionalKey(code, extra, dialect)
}

// Unknown Kitty private-use key numbers are refused so unsupported keys do not
// insert glyphs the user never typed.
func extendedKeyCode(num int) (Code, rune, bool) {
	if code, ok := kittyKeyNumbers[num]; ok {
		return code, 0, true
	}
	// Kitty numbers the function row and then the keypad consecutively.
	if offset := num - kittyF1; offset >= 0 && offset <= int(KeypadBegin-F1) {
		return F1 + Code(offset), 0, true
	}
	// The private use area is where a terminal puts keys that are not characters,
	// and this function has already taken the ones it knows about.
	r, ok := codePoint(num)
	if !ok || r != 0 && unicode.IsControl(r) || (num >= kittyFunctionKeysFrom && num <= kittyFunctionKeysTo) {
		return 0, 0, false
	}
	return Character, r, true
}
