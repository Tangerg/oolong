package input

import (
	"bytes"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/ansi"
)

// DefaultEscapeTimeout belongs to the protocol boundary so local and remote
// transports resolve the same ambiguous bytes consistently. Parser owns no clock.
const DefaultEscapeTimeout = 30 * time.Millisecond

const esc = ansi.Escape

const (
	maxSequenceBody = 64 << 10

	// maxPaste bounds what one paste may accumulate. A terminal that opens a
	// paste and never closes it would otherwise swallow everything typed
	// afterwards into a buffer that only grows. On reaching the bound the text so
	// far is delivered as one Paste event and accumulation begins again, so the
	// allocation stays bounded without interpreting any of the payload as input.
	// Each malformed input byte can expand to a UTF-8 replacement rune.
	maxPaste = (MaxDecodedEventBytes - eventOverhead) / len("�")
)

// Only the exact framing sequences may change whether subsequent bytes are keys or text.
const pasteOpen = "\x1b[200~"

var pasteClose = []byte{esc, '[', '2', '0', '1', '~'}

// Dropping retains oversized-sequence framing so later payload bytes cannot
// be interpreted as keystrokes.
type dropping uint8

const (
	droppingNothing dropping = iota
	// droppingParams is a control sequence whose parameter section overran. It
	// ends at the first byte a sequence could have ended with.
	droppingParams
	// droppingString is an operating system command whose parameters overran. It
	// ends at a terminator.
	droppingString
)

// KeyboardDialect owns overlapping wire meanings; enhancement flags do not
// change a protocol family's modifier namespace.
type KeyboardDialect uint8

const (
	XTermKeyboard KeyboardDialect = iota
	KittyKeyboard
)

type ParserConfig struct {
	// Legacy modified F3 overlaps an unmarked cursor-position reply. Query
	// owners must use DEC private CPR (CSI ?6n) while this dialect is active.
	Keyboard KeyboardDialect
}

func NewParser(cfg ParserConfig) *Parser { return &Parser{keyboard: cfg.Keyboard} }

// Once a payload introducer is complete, its bytes cannot regain keyboard
// meaning through a timeout. One frame kind owns that interpretation until the
// payload ends; paste, mouse and control-string bodies are mutually exclusive.
type frameKind uint8

const (
	noFrame frameKind = iota
	pasteFrame
	x10MouseFrame
	oscFrame
	dcsFrame
	apcFrame
)

// Parser retains ambiguous bytes across arbitrary chunks so split escape sequences
// and UTF-8 cannot lose keys. A lone escape needs an external deadline: Pending
// reports it and Flush ends the wait. A Parser belongs to one reader goroutine,
// must not be copied after first use, and is not safe for concurrent use.
type Parser struct {
	noCopy   noCopy
	keyboard KeyboardDialect

	buf       []byte
	expiring  bool
	controlAt int
	frame     frameKind
	paste     []byte
	oscCmd    int
	strBody   []byte
	// dropping is set when a sequence overran what one can hold, and stays set
	// until that sequence ends. See [Parser.skipParams] and [Parser.skipString].
	dropping dropping
}

// noCopy makes the parser's single-owner contract visible to go vet. Its methods
// are never called.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

func (p *Parser) Feed(b []byte) []Event {
	p.buf = append(p.buf, b...)
	return p.drain(false)
}

// Flush resolves pending input at the end of the byte stream.
// Use Ambiguous and Expire for an Escape timer while input can still arrive.
//
// A buffered escape becomes the Escape key, and anything after it is re-read as
// ordinary input. A half-arrived character is dropped, since the rest is never
// coming. A paste in progress is left alone: it is incomplete rather than
// ambiguous, and cutting it short would corrupt the text.
// A truncated X10 report is discarded because its raw fields cannot become keys.
func (p *Parser) Flush() []Event { return p.drain(true) }

// Pending reports whether the parser is waiting for more input. It does not
// indicate a timeout: partial UTF-8 and paste data must continue waiting. Arm an
// Escape timer only when Ambiguous reports true, and call Expire when it fires.
//
// An open paste and a string being accumulated count even with nothing buffered.
// Their bytes have been consumed into the payload rather than left in the buffer,
// so a report drawn from the buffer alone would say a paste that had not reached
// its terminator was nothing at all.
func (p *Parser) Pending() bool {
	return len(p.buf) > 0 || p.dropping != droppingNothing || p.frame != noFrame
}

// Ambiguous reports whether an Escape timeout can resolve pending input.
// Incomplete UTF-8 and recognized paste, mouse or string payloads must wait for more bytes.
func (p *Parser) Ambiguous() bool {
	// A discarded OSC/DCS body still has framing. Silence in its payload
	// grants no keyboard authority; only a trailing Escape is ambiguous.
	if p.dropping == droppingParams {
		return true
	}
	return (p.frame == noFrame || p.dropping == droppingString) && len(p.buf) > 0 && p.buf[0] == esc
}

// Expire resolves Escape ambiguity without declaring the byte stream finished.
func (p *Parser) Expire() []Event {
	if !p.Ambiguous() {
		return nil
	}
	p.expiring = true
	defer func() { p.expiring = false }()
	return p.drain(true)
}

func (p *Parser) drain(final bool) (events []Event) {
	initial := len(p.buf)
	defer func() {
		if len(p.buf) > 0 && len(p.buf) < initial/2 {
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
	if p.dropping != droppingNothing {
		return p.advanceRunaway(final)
	}
	switch p.frame {
	case pasteFrame:
		return p.advancePaste()
	case x10MouseFrame:
		return p.advanceX10Mouse(final)
	case oscFrame, dcsFrame, apcFrame:
		return p.readString()
	default:
		return p.advanceInput(final)
	}
}

func (p *Parser) advanceRunaway(final bool) (Event, bool) {
	if p.skipRunaway() {
		return nil, true
	}
	if final {
		// EOF releases every discard. During a live stream only ambiguous
		// input can reach this branch; an ordinary string payload has no timer.
		p.abandonString()
		p.dropping = droppingNothing
		return nil, true
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
	if !final || (p.expiring && p.buf[0] != esc) {
		return nil, false
	}
	if p.buf[0] == esc {
		p.take(1)
		return Key{Code: Esc}, true
	}
	// A character whose remaining bytes will never arrive. Dropping one byte rather
	// than the buffer keeps whatever follows decodable.
	p.take(1)
	return nil, true
}

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
		return false
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
	p.controlAt = 0
	p.buf = p.buf[n:]
	if len(p.buf) == 0 {
		p.buf = nil
	}
}

func (p *Parser) readPaste() (string, bool) {
	i := 0
	for i < len(p.buf) {
		if p.buf[i] == esc {
			rest := p.buf[i:]
			switch shared := commonPrefix(rest, pasteClose); {
			case shared == len(pasteClose):
				p.take(i + len(pasteClose))
				return p.closePaste(), true
			case shared == len(rest):
				p.take(i)
				return "", false
			}
		}
		p.paste = append(p.paste, p.buf[i])
		i++
		if len(p.paste) >= maxPaste {
			end := len(p.paste) - 1
			for end > 0 && !utf8.RuneStart(p.paste[end]) {
				end--
			}
			if utf8.FullRune(p.paste[end:]) {
				end = len(p.paste)
			}
			text := string(p.paste[:end])
			p.paste = bytes.Clone(p.paste[end:])
			p.take(i)
			return text, true
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

func (p *Parser) closePaste() string {
	p.frame = noFrame
	return p.takePaste()
}

// decode consumes nothing when more bytes may change the result. A nil event
// with consumed bytes means recognized input that deliberately produces no event.
// final closes Escape ambiguity because no continuation can arrive.
func (p *Parser) decode(b []byte, final bool) (n int, ev Event, done bool) {
	switch c := b[0]; {
	case c == esc:
		return p.decodeEscape(b, final)
	case c == '\r':
		return 1, Key{Code: Enter}, true
	case c == '\t':
		return 1, Key{Code: Tab}, true
	case c == deleteByte, c == '\b':
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

func c0Rune(c byte) rune {
	switch c {
	case 0x00:
		return ' '
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

// Once the Escape wait expires, an unfinished introducer can be read as an
// Alt chord: ESC [ names Alt+[ as well as a CSI prefix. A lone Escape is settled
// before this path.
func (p *Parser) decodeEscape(b []byte, final bool) (n int, ev Event, done bool) {
	if len(b) == 1 {
		return 0, nil, false
	}
	n, ev, done = p.decodeIntroduced(b)
	if !done && final {
		return decodeAlt(b)
	}
	return n, ev, done
}

func (p *Parser) decodeIntroduced(b []byte) (n int, ev Event, done bool) {
	switch second := b[1]; {
	case second == '[':
		return p.decodeControl(b)
	case second == oscIntro:
		cmd, size := oscHead(b)
		return p.introduceString(b, oscFrame, cmd, size)
	case second == dcsIntro:
		return p.introduceString(b, dcsFrame, 0, dcsHead(b))
	case second == apcIntro:
		return p.introduceString(b, apcFrame, 0, graphicsHead(b))
	case second == 'O':
		if len(b) < 3 {
			return 0, nil, false
		}
		if code, ok := finalByteKeys[b[2]]; ok {
			return 3, Key{Code: code}, true
		}
		return 3, nil, true
	case second == esc:
		return 1, Key{Code: Esc}, true
	case second < 0x20, second == deleteByte:
		consumed, event, ready := p.decode(b[1:], false)
		if key, ok := event.(Key); ok {
			key.Mods |= Alt
			event = key
		}
		return consumed + 1, event, ready
	default:
		return decodeAlt(b)
	}
}

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

func (p *Parser) decodeControl(b []byte) (n int, ev Event, done bool) {
	i := max(2, p.controlAt)
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
	case ps.Len() == 0 && final == 'I':
		return FocusIn{}
	case ps.Len() == 0 && final == 'O':
		return FocusOut{}
	}

	switch final {
	case 'R':
		if p.keyboard != XTermKeyboard {
			return nil
		}
		return ps.legacyF3()
	case 'u':
		return ps.extendedKey()
	case '~':
		return ps.numberedKey(p.keyboard)
	case 'Z':
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

func (ps params) numberedKey(dialect KeyboardDialect) Event {
	code, ok := tildeKeyNumbers[ps.At(0)]
	if !ok || ps.Group(0).Len() != 1 {
		return nil
	}
	return ps.functionalKey(code, 0, dialect)
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
	if !ok || (num >= kittyFunctionKeysFrom && num <= kittyFunctionKeysTo) {
		return 0, 0, false
	}
	return Character, r, true
}

func commonPrefix(a, b []byte) int {
	n := min(len(a), len(b))
	for i := range n {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func (p *Parser) introduceString(b []byte, kind frameKind, cmd, size int) (int, Event, bool) {
	switch {
	case size < 0:
		return decodeAlt(b)
	case size == 0:
		return 0, nil, false
	default:
		p.frame, p.oscCmd, p.strBody = kind, cmd, nil
		return size, nil, true
	}
}

// OSC and DCS accept BEL; graphics replies require ST. An unexpected Escape
// abandons the current body so a truncated reply cannot consume every later key.
func (p *Parser) readString() (Event, bool) {
	for i := 0; i < len(p.buf); {
		c := p.buf[i]
		switch {
		case c == bel && p.frame != apcFrame:
			p.take(i + 1)
			return p.endString(), true
		case c == esc:
			if i+1 >= len(p.buf) {
				// What is left could still become ST. Keep the escape.
				p.take(i)
				return nil, false
			}
			if p.buf[i+1] == '\\' {
				p.take(i + 2)
				return p.endString(), true
			}
			p.take(i)
			p.abandonString()
			return nil, true
		default:
			p.strBody = append(p.strBody, c)
			i++
			if len(p.strBody) >= maxStringBody {
				// Overran what one may hold. The rest is dropped wherever it turns up
				// rather than read as text, for the same reason a runaway control
				// sequence is: a flood of keystrokes is a worse answer to a malformed
				// sequence than silence, and one a hostile terminal could aim.
				p.take(i)
				p.strBody = nil
				p.dropping = droppingString
				return nil, true
			}
		}
	}
	p.buf = nil
	return nil, false
}

// Reply bodies become text at this boundary; replace invalid UTF-8 before
// consumers can treat the result as displayable text.
func (p *Parser) endString() Event {
	body := strings.ToValidUTF8(string(p.strBody), "�")
	kind, cmd := p.frame, p.oscCmd
	p.abandonString()
	switch kind {
	case apcFrame:
		return APC{Body: body}
	case dcsFrame:
		return DCS{Body: body}
	default:
		return OSC{Command: cmd, Params: body}
	}
}

func (p *Parser) abandonString() {
	p.frame, p.oscCmd, p.strBody = noFrame, 0, nil
}

func (p *Parser) skipString() bool {
	for i := range len(p.buf) {
		switch {
		case p.buf[i] == bel && p.frame != apcFrame:
			p.take(i + 1)
			p.abandonString()
			p.dropping = droppingNothing
			return true
		case p.buf[i] == esc:
			if i+1 >= len(p.buf) {
				p.take(i)
				return false
			}
			if p.buf[i+1] == '\\' {
				p.take(i + 2)
			} else {
				// The escape abandoned the string, and belongs to whatever comes next
				// rather than to this.
				p.take(i)
			}
			p.abandonString()
			p.dropping = droppingNothing
			return true
		}
	}
	p.buf = nil
	return false
}
