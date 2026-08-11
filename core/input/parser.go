package input

import (
	"bytes"
	"image"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/oolong/core/ansi"
)

// DefaultEscapeTimeout is how long a byte stream waits before treating a lone
// escape byte as the Escape key rather than the beginning of a terminal sequence.
//
// The parser itself has no clock: a transport feeds bytes into [Parser] and flushes
// a pending parse after this interval. Keeping the default beside the protocol
// makes a local terminal and a remote terminal interpret the same byte stream the
// same way.
const DefaultEscapeTimeout = 30 * time.Millisecond

// esc is the byte every sequence begins with, under the name this package's own
// reading of them uses.
const esc = ansi.Escape

const (
	// maxSequenceBody caps a control sequence's parameter section. A real key or
	// mouse report is an order of magnitude shorter; anything longer is garbage,
	// and buffering it would let a stream that never sends a final byte grow
	// memory without limit.
	maxSequenceBody = 64

	// maxPaste bounds what one paste may accumulate. A terminal that opens a
	// paste and never closes it would otherwise swallow everything typed
	// afterwards into a buffer that only grows. On reaching the bound the text so
	// far is delivered as one Paste event and accumulation begins again, so the
	// allocation stays bounded without interpreting any of the payload as input.
	maxPaste = 8 << 20
)

// pasteClose is the sequence a terminal sends to end a bracketed paste.
var pasteClose = []byte{esc, '[', '2', '0', '1', '~'}

// dropping says which kind of runaway sequence the parser is throwing away, if
// any.
//
// Both kinds have to be remembered rather than simply left behind: the bytes
// still to come would otherwise arrive as a flood of keystrokes, and a hostile
// terminal could aim that.
type dropping uint8

const (
	// droppingNothing is the zero value: nothing is being thrown away.
	droppingNothing dropping = iota
	// droppingParams is a control sequence whose parameter section overran. It
	// ends at the first byte a sequence could have ended with.
	droppingParams
	// droppingString is an operating system command whose parameters overran. It
	// ends at a terminator.
	droppingString
)

// Parser decodes terminal bytes into events, incrementally.
//
// Bytes are handed to [Parser.Feed] exactly as they arrived, at whatever
// boundaries the read produced. Anything not yet unambiguous stays buffered:
// escape sequences and multi-byte characters routinely arrive in pieces, and a
// decoder that assumed otherwise would drop keys under load.
//
// One case cannot be resolved by waiting. A lone escape byte is either the Escape
// key or the start of a sequence whose remainder has not arrived, and only time
// tells the difference. [Parser.Pending] reports that something is waiting, and
// [Parser.Flush] declares the wait over.
//
// Not safe for concurrent use: it belongs to whichever goroutine reads the
// terminal.
type Parser struct {
	buf []byte
	// pasting is set between a paste's opening and closing sequences, when bytes
	// are text rather than input to interpret.
	pasting bool
	paste   []byte
	// str is the kind of string being accumulated, between its head and its
	// terminator, when bytes are its body rather than input to interpret.
	str     stringKind
	oscCmd  int
	strBody []byte
	// dropping is set when a sequence overran what one can hold, and stays set
	// until that sequence ends. See [Parser.skipParams] and [Parser.skipString].
	dropping dropping
}

// Feed adds bytes and returns everything now decodable.
func (p *Parser) Feed(b []byte) []Event {
	p.buf = append(p.buf, b...)
	return p.drain(false)
}

// Flush resolves what only time could resolve and returns the result.
//
// A buffered escape becomes the Escape key, and anything after it is re-read as
// ordinary input. A half-arrived character is dropped, since the rest is never
// coming. A paste in progress is left alone: it is incomplete rather than
// ambiguous, and cutting it short would corrupt the text.
func (p *Parser) Flush() []Event { return p.drain(true) }

// Pending reports whether anything is waiting for more input to make sense of it:
// bytes that might yet become a sequence, or a runaway one still being dropped. It
// is what tells a caller to arm the timer that will call [Parser.Flush], and the
// runaway counts because the state has to end somewhere — otherwise the next
// keystroke that happened to be a parameter byte would vanish into it.
func (p *Parser) Pending() bool { return len(p.buf) > 0 || p.dropping != droppingNothing }

// drain decodes as much as it can. When final, trailing ambiguity is resolved
// rather than kept.
func (p *Parser) drain(final bool) (events []Event) {
	defer func() {
		if len(p.buf) > 0 {
			// The undecided tail is short by construction. Clone it after draining so
			// a lone escape or partial UTF-8 sequence cannot keep an otherwise consumed
			// terminal read buffer alive while input is idle.
			p.buf = bytes.Clone(p.buf)
		}
	}()
	for {
		event, advanced := p.advance(final)
		if !advanced {
			return events
		}
		if event != nil {
			events = append(events, event)
		}
	}
}

// advance makes one state transition. Its boolean distinguishes waiting from
// consuming bytes that deliberately produce no event.
func (p *Parser) advance(final bool) (Event, bool) {
	switch {
	case p.dropping != droppingNothing:
		return p.advanceRunaway(final)
	case p.str != noString:
		return p.readString()
	case p.pasting:
		return p.advancePaste()
	default:
		return p.advanceInput(final)
	}
}

func (p *Parser) advanceRunaway(final bool) (Event, bool) {
	if p.skipRunaway() {
		return nil, true
	}
	if final {
		// Input went quiet part-way through, so the runaway sequence is over however
		// it ended. Staying in this state would eat the next keystroke that happened
		// to be a parameter byte.
		//
		// A command still accumulating gets no such treatment: it is incomplete rather
		// than ambiguous, and time says nothing about it. Giving up on one already
		// given up on has to end somewhere; giving up on one still arriving would
		// corrupt the answer to a query as large as a clipboard.
		p.dropping = droppingNothing
	}
	return nil, false
}

func (p *Parser) advancePaste() (Event, bool) {
	text, done := p.readPaste()
	if !done || text == "" {
		return nil, done
	}
	// A paste is bytes a terminal was handed by something else, and Text is a
	// string, which every consumer will treat as text and eventually put in a cell.
	// Invalid UTF-8 is replaced rather than passed on, for the same reason a control
	// character is dropped at the cell: this is where untrusted bytes stop being
	// untrusted.
	return Paste{Text: strings.ToValidUTF8(text, "\uFFFD")}, true
}

func (p *Parser) advanceInput(final bool) (Event, bool) {
	if len(p.buf) == 0 {
		return nil, false
	}
	n, event, done := p.decode(p.buf, final)
	if done {
		p.take(n)
		return event, true
	}
	if !final {
		return nil, false
	}
	if p.buf[0] == esc {
		// Nothing followed it in time, so it was the key.
		p.take(1)
		return Key{Code: Esc}, true
	}
	// A character whose remaining bytes will never arrive. Dropping one byte rather
	// than the buffer keeps whatever follows decodable.
	p.take(1)
	return nil, true
}

// skipRunaway drops the rest of whichever runaway sequence is being thrown away.
func (p *Parser) skipRunaway() bool {
	if p.dropping == droppingString {
		return p.skipString()
	}
	return p.skipParams()
}

// skipParams drops the rest of a control sequence whose parameter section
// overran, and reports whether it found the end.
//
// The bytes are dropped rather than re-read as text. A stream of parameter bytes
// would otherwise arrive as a flood of keystrokes, which is a worse answer to a
// malformed sequence than silence — and one that a hostile terminal could aim.
func (p *Parser) skipParams() bool {
	i := 0
	for i < len(p.buf) && ansi.Body(p.buf[i]) {
		i++
	}
	if i >= len(p.buf) {
		p.take(i)
		return false // still inside it
	}
	// The byte that ended the run of parameter bytes ends the sequence too, if it
	// is one a sequence could have ended with. Anything else proved the sequence
	// malformed and is left to be read on its own terms.
	if ansi.Final(p.buf[i]) {
		i++
	}
	p.take(i)
	p.dropping = droppingNothing
	return true
}

// take drops n decoded bytes, releasing the buffer once it is empty so an idle
// parser holds nothing.
func (p *Parser) take(n int) {
	p.buf = p.buf[n:]
	if len(p.buf) == 0 {
		p.buf = nil
	}
}

// readPaste moves buffered bytes into the paste until its closing sequence or the
// payload bound is reached, reporting whether a chunk is ready to publish.
func (p *Parser) readPaste() (string, bool) {
	i := 0
	for i < len(p.buf) {
		if p.buf[i] != esc {
			p.paste = append(p.paste, p.buf[i])
			i++
			if len(p.paste) >= maxPaste {
				p.take(i)
				return p.takePaste(), true
			}
			continue
		}
		rest := p.buf[i:]
		switch shared := commonPrefix(rest, pasteClose); {
		case shared == len(pasteClose):
			p.take(i + len(pasteClose))
			return p.closePaste(), true
		case shared == len(rest):
			// What is left could still become the closing sequence.
			p.buf = p.buf[i:]
			return "", false
		default:
			// It was not the closing sequence, so it is pasted text.
			p.paste = append(p.paste, esc)
			i++
		}
	}
	p.buf = nil
	return "", false
}

// takePaste publishes the bounded payload accumulated so far without ending paste
// mode. A large paste is therefore a sequence of Paste events, never text re-read as
// keystrokes merely because it crossed an allocation bound.
func (p *Parser) takePaste() string {
	text := string(p.paste)
	p.paste = nil
	return text
}

// closePaste publishes the final payload and leaves paste mode.
func (p *Parser) closePaste() string {
	p.pasting = false
	return p.takePaste()
}

// decode reads one event from the front of b. It reports how many bytes it
// consumed, the event — nil when the bytes were understood but mean nothing to
// report — and whether it got far enough to decide. When it did not, no bytes
// were consumed.
//
// final says the wait for more bytes is over, which only an escape sequence has
// any use for: it is the difference between a sequence still arriving and one
// that was never a sequence.
func (p *Parser) decode(b []byte, final bool) (n int, ev Event, done bool) {
	switch c := b[0]; {
	case c == esc:
		return p.decodeEscape(b, final)
	case c == 0x0d:
		return 1, Key{Code: Enter}, true
	case c == 0x09:
		return 1, Key{Code: Tab}, true
	case c == 0x7f, c == 0x08:
		return 1, Key{Code: Backspace}, true
	case c >= 0x01 && c <= 0x1a:
		// Ctrl with a letter, which is what the terminal sends for it. Enter, Tab
		// and Backspace fall in this range too and are answered above, where their
		// own names are the truer report.
		return 1, Key{Code: Character, Rune: rune('a' + c - 1), Mods: Ctrl}, true
	case c < 0x20:
		// The rest of the C0 block. Each is Ctrl with a punctuation key, and
		// naming them is what makes those chords bindable at all.
		return 1, Key{Code: Character, Rune: c0Rune(c), Mods: Ctrl}, true
	default:
		if !utf8.FullRune(b) {
			return 0, nil, false
		}
		r, size := utf8.DecodeRune(b)
		if r == utf8.RuneError && size == 1 {
			return 1, nil, true // not valid UTF-8: dropped
		}
		return size, Key{Code: Character, Rune: r}, true
	}
}

// c0Rune names the C0 control bytes that are not keys of their own.
func c0Rune(c byte) rune {
	switch c {
	case 0x00:
		return ' ' // Ctrl+Space
	case 0x1c:
		return '\\'
	case 0x1d:
		return ']'
	case 0x1e:
		return '^'
	case 0x1f:
		return '_'
	default:
		return rune('a' + c - 1)
	}
}

// decodeEscape reads a sequence introduced by the escape byte.
//
// When the wait is over and the sequence never completed, what arrived is read as
// a chord instead. Every introducer is also a character a terminal sends for Alt
// with that key — "\x1b[" is Alt+[ as much as it is the start of a control
// sequence — and by the time the wait is over the two are no longer ambiguous:
// both bytes arrived before the pause, so they came in one burst, and a burst is
// what a chord is. An escape that stood alone never reaches here; it is resolved
// a step earlier, when nothing had followed it at all.
func (p *Parser) decodeEscape(b []byte, final bool) (n int, ev Event, done bool) {
	if len(b) == 1 {
		return 0, nil, false // could be the key, could be a sequence: wait
	}
	n, ev, done = p.decodeIntroduced(b)
	if !done && final {
		return decodeAlt(b)
	}
	return n, ev, done
}

// decodeIntroduced reads the sequence the byte after the escape introduces.
func (p *Parser) decodeIntroduced(b []byte) (n int, ev Event, done bool) {
	switch second := b[1]; {
	case second == '[':
		return p.decodeControl(b)
	case second == oscIntro:
		cmd, size := oscHead(b)
		switch {
		case size > 0:
			p.beginString(oscString, cmd)
			return size, nil, true
		case size == 0:
			return 0, nil, false // too few bytes to tell a command from the key
		default:
			return decodeAlt(b)
		}
	case second == dcsIntro:
		switch size := dcsHead(b); {
		case size > 0:
			p.beginString(dcsString, 0)
			return size, nil, true
		case size == 0:
			return 0, nil, false
		default:
			return decodeAlt(b)
		}
	case second == 'O':
		if len(b) < 3 {
			return 0, nil, false
		}
		if code, ok := applicationKey(b[2]); ok {
			return 3, Key{Code: code}, true
		}
		return 3, nil, true
	case second == esc:
		// Two in a row: the first was the key, and the second starts again.
		return 1, Key{Code: Esc}, true
	case second == 0x0d:
		// Alt+Enter, which terminals send this way rather than as a modified-key
		// report.
		return 2, Key{Code: Enter, Mods: Alt}, true
	case second < 0x20, second == 0x7f:
		// A control byte cannot continue the sequence, so the escape stood alone
		// and the control byte is read on the next pass.
		return 1, Key{Code: Esc}, true
	default:
		return decodeAlt(b)
	}
}

// decodeAlt reads an escape followed by a character as that character with Alt
// held, which is how a terminal reports the chord.
func decodeAlt(b []byte) (n int, ev Event, done bool) {
	if !utf8.FullRune(b[1:]) {
		return 0, nil, false
	}
	r, size := utf8.DecodeRune(b[1:])
	if r == utf8.RuneError && size == 1 {
		return 1, Key{Code: Esc}, true
	}
	return 1 + size, Key{Code: Character, Rune: r, Mods: Alt}, true
}

// decodeControl reads a control sequence: a parameter section, then a final byte
// that says what the sequence was.
func (p *Parser) decodeControl(b []byte) (n int, ev Event, done bool) {
	i := 2
	for i < len(b) && ansi.Body(b[i]) {
		i++
	}
	if i-2 > maxSequenceBody {
		// Overran what one may hold. The check does not wait to see whether a
		// final byte happens to be in this chunk, and it does not drop "whatever
		// is buffered": either would make the cap depend on where the read split,
		// and the same bytes would decode differently for arriving in different
		// pieces. What is dropped is the introducer and the body seen so far, and
		// [Parser.skipParams] drops the rest wherever it turns up.
		p.dropping = droppingParams
		return i, nil, true
	}
	if i >= len(b) {
		return 0, nil, false
	}
	final := b[i]
	if !ansi.Final(final) {
		// A byte that cannot appear in a control sequence. Drop the malformed
		// prefix and start again at the byte that proved it malformed.
		return i, nil, true
	}
	n = i + 1
	ps := parseParams(string(b[2:i]))

	// A private marker says the sequence is a report and not a key, because a key
	// never carries one. So anything with a marker is answered as a report or
	// dropped, and nothing carrying one can reach the code below that reads keys.
	//
	// The rule is the fix for a whole class rather than for the two reports that were
	// found broken. Dispatching on the final byte alone let a keyboard-flags reply —
	// "CSI ? 31 u" — decode as a keystroke of an invisible control character, which is
	// a defect that hides itself: printing the events showed an empty pair of brackets.
	if ps.Marker() != 0 {
		return n, ps.report(final), true
	}

	switch {
	case ps.Len() == 0 && final == 'I':
		return n, FocusIn{}, true
	case ps.Len() == 0 && final == 'O':
		return n, FocusOut{}, true
	}

	switch final {
	case 'u':
		return n, ps.extendedKey(), true
	case '~':
		return n, p.decodeNumberedKey(ps), true
	case 'Z':
		mods, transition, ok := ps.keyMeta()
		if !ok {
			return n, nil, true
		}
		// Shift and tab, not a key of its own. A terminal speaking the Kitty protocol
		// reports the same keystroke as tab with shift held, and one keystroke that
		// arrives under two names is one nothing can be bound to: whichever of the two
		// a binding names, it misses on half the terminals there are.
		return n, Key{Code: Tab, Mods: mods | Shift, Transition: transition}, true
	default:
		code, ok := cursorKey(final)
		if !ok {
			return n, nil, true // a sequence this decoder does not recognize
		}
		mods, transition, ok := ps.keyMeta()
		if !ok {
			return n, nil, true
		}
		return n, Key{Code: code, Mods: mods, Transition: transition}, true
	}
}

// decodeNumberedKey reads the sequences that name a key by number, which is also
// how a terminal announces a paste.
func (p *Parser) decodeNumberedKey(ps params) Event {
	switch num := ps.At(0); num {
	case pasteOpen:
		p.pasting = true
		return nil
	case pasteCloseNum:
		return nil // a closing sequence with no paste open
	default:
		code, ok := numberedKey(num)
		if !ok {
			return nil
		}
		mods, transition, ok := ps.keyMeta()
		if !ok {
			return nil
		}
		return Key{Code: code, Mods: mods, Transition: transition}
	}
}

const (
	pasteOpen     = 200
	pasteCloseNum = 201
)

// extendedKey reads the Kitty keyboard protocol's key report, which is the
// only form that distinguishes releases from presses and can say what text a key
// produced.
func (ps params) extendedKey() Event {
	if ps.Len() == 0 || ps.Len() > 3 {
		return nil // a bare sequence here is a cursor report, not a key
	}
	primary := ps.Group(0)
	if primary.Len() == 0 || primary.Len() > 3 || primary.At(0) <= 0 {
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
	code, r, ok := extendedKeyCode(primary.At(0))
	if !ok {
		return nil
	}
	return Key{Code: code, Rune: r, Mods: mods, Transition: transition, Text: text}
}

// extendedKeyCode maps a Kitty key number onto a [Code].
//
// Numbers in the Unicode private-use area that this program does not recognise
// are rejected rather than passed through: emitting one as a character would put
// a glyph nobody typed into the text.
func extendedKeyCode(num int) (Code, rune, bool) {
	if code, ok := extendedKeys[num]; ok {
		return code, 0, true
	}
	if num >= 57364 && num <= 57375 {
		return F1 + Code(num-57364), 0, true
	}
	// The private use area is where a terminal puts keys that are not characters,
	// and this function has already taken the ones it knows about.
	r, ok := codePoint(num)
	if !ok || (num >= 0xe000 && num <= 0xf8ff) {
		return 0, 0, false
	}
	return Character, r, true
}

var extendedKeys = map[int]Code{
	8: Backspace, 127: Backspace, 57347: Backspace,
	9: Tab, 57346: Tab,
	13: Enter, 57345: Enter,
	27: Esc, 57344: Esc,
	57348: Insert,
	57349: Delete,
	57350: Left,
	57351: Right,
	57352: Up,
	57353: Down,
	57354: PageUp,
	57355: PageDown,
	57356: Home,
	57357: End,
}

// mouse reads an SGR mouse report. down distinguishes the final byte that
// means "went down or moved" from the one that means "came up".
func (ps params) mouse(down bool) Event {
	if ps.Len() < 3 {
		return nil
	}
	bits, x, y := ps.At(0), ps.At(1), ps.At(2)
	if bits < 0 || x < 0 || y < 0 {
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

func mouseMods(bits int) Mods {
	var mods Mods
	if bits&4 != 0 {
		mods |= Shift
	}
	if bits&8 != 0 {
		mods |= Alt
	}
	if bits&16 != 0 {
		mods |= Ctrl
	}
	return mods
}

func mouseButton(bits int) Button {
	switch bits {
	case 0:
		return ButtonLeft
	case 1:
		return ButtonMiddle
	case 2:
		return ButtonRight
	default:
		return ButtonNone
	}
}

// commonPrefix is how many leading bytes a and b share.
func commonPrefix(a, b []byte) int {
	n := min(len(a), len(b))
	for i := range n {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func cursorKey(final byte) (Code, bool) {
	code, ok := cursorKeys[final]
	return code, ok
}

var cursorKeys = map[byte]Code{
	'A': Up, 'B': Down, 'C': Right, 'D': Left,
	'H': Home, 'F': End,
	'P': F1, 'Q': F2, 'R': F3, 'S': F4,
}

// applicationKey reads the keypad-mode form some terminals use for arrows and the
// first four function keys.
func applicationKey(c byte) (Code, bool) {
	code, ok := applicationKeys[c]
	return code, ok
}

var applicationKeys = map[byte]Code{
	'P': F1, 'Q': F2, 'R': F3, 'S': F4,
	'A': Up, 'B': Down, 'C': Right, 'D': Left,
	'H': Home, 'F': End,
}

func numberedKey(num int) (Code, bool) {
	code, ok := numberedKeys[num]
	return code, ok
}

var numberedKeys = map[int]Code{
	1: Home, 7: Home,
	2: Insert,
	3: Delete,
	4: End, 8: End,
	5:  PageUp,
	6:  PageDown,
	11: F1, 12: F2, 13: F3, 14: F4, 15: F5,
	17: F6, 18: F7, 19: F8, 20: F9, 21: F10,
	23: F11, 24: F12,
}
