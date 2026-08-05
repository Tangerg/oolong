package input

import "strings"

const (
	// bel is the older of the two ways a terminal ends a string it is sending. The
	// other is ST: an escape and a backslash.
	bel = 0x07

	// oscIntro and dcsIntro are the bytes that introduce the two strings a terminal
	// answers with: an operating system command, and a device control string.
	//
	// Each is also what a terminal sends for a chord — Alt+] for the first,
	// Alt+Shift+P for the second — and the introducer alone cannot tell the two
	// apart. So each is decoded only when what follows looks like an answer, and
	// anything else is the keystroke. See [oscHead] and [dcsHead].
	//
	// The rest of the family is still not decoded. A start-of-string and a privacy
	// message would cost Alt+Shift+X and Alt+^ to catch sequences nothing here ever
	// asks for, and a sequence nobody asked for does not arrive.
	oscIntro = ']'
	dcsIntro = 'P'

	// maxOSCCommand bounds how many digits a command number may have. The longest in
	// use is four — iTerm2's 1337 — so five is room to spare and a bound all the same.
	maxOSCCommand = 5

	// maxStringBody bounds what one string's body may accumulate.
	//
	// It is far larger than maxSequenceBody because one of these legitimately carries
	// a clipboard: the answer to a read of the terminal's selection is the whole of
	// it, base64-encoded. It is a bound all the same, because a terminal that opens a
	// string and never ends it must not be able to grow memory without limit.
	maxStringBody = 1 << 20
)

// stringKind is which of the two strings is being read.
type stringKind uint8

const (
	noString stringKind = iota
	oscString
	dcsString
)

// oscHead reads the command number at the start of an operating system command.
//
// The returned size is how many bytes the introducer, the number and its separator
// took, and its sign is the verdict: positive for a command, zero when too few bytes
// have arrived to tell, and negative when what follows the introducer is not a number
// — which means it was the keystroke Alt+] and not an introducer at all.
//
// A terminator directly after the number is left unconsumed for [Parser.readString] to
// find, which is what makes a command with no parameters decode as one rather than as
// two halves of nothing.
func oscHead(b []byte) (cmd, size int) {
	i := 2
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		if i-2 == maxOSCCommand {
			return 0, -1
		}
		cmd = cmd*10 + int(b[i]-'0')
		i++
	}
	if i >= len(b) {
		// Either nothing followed the introducer yet, or the number may still have
		// digits to come. Waiting settles both.
		return 0, 0
	}
	if i == 2 {
		return 0, -1 // no number, so no command
	}
	switch b[i] {
	case ';':
		return cmd, i + 1
	case bel, esc:
		return cmd, i
	default:
		return 0, -1
	}
}

// dcsHead decides whether what follows the device-control introducer is an answer.
//
// A device control string has no command number, so there is no single thing to read.
// What it has instead is a small set of shapes a terminal replies in: a ">" for the
// version report, and a digit followed by "$" or "+" for the answers to a setting or a
// capability query. Nothing a person types after Alt+Shift+P looks like either.
//
// The size is the introducer only, because the marker is part of the body: what a
// caller matches on is ">|kitty(0.32)", not the tail of it.
func dcsHead(b []byte) (size int) {
	if len(b) < 3 {
		return 0 // too few bytes to tell
	}
	switch {
	case b[2] == '>':
		return 2
	case b[2] >= '0' && b[2] <= '9':
		if len(b) < 4 {
			return 0
		}
		if b[3] == '$' || b[3] == '+' {
			return 2
		}
		return -1
	default:
		return -1
	}
}

// beginString starts accumulating a string of the given kind.
func (p *Parser) beginString(kind stringKind, cmd int) {
	p.str, p.oscCmd, p.strBody = kind, cmd, nil
}

// readString moves buffered bytes into the current string's body until its terminator
// arrives, reporting the finished event when one does.
//
// Two byte sequences end it: the BEL older terminals use, and the ST that ECMA-48
// specifies, which is an escape and a backslash. An escape followed by anything else
// never legitimately appears inside one, so it is taken to have abandoned the string:
// the body so far is dropped and the escape is read again on its own terms. That is
// what makes a terminal which stops mid-answer cost one keystroke rather than every
// keystroke after it.
func (p *Parser) readString() (Event, bool) {
	for i := 0; i < len(p.buf); {
		switch c := p.buf[i]; c {
		case bel:
			p.take(i + 1)
			return p.endString(), true
		case esc:
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
				p.abandonString()
				p.dropping = droppingString
				return nil, true
			}
		}
	}
	p.buf = nil
	return nil, false
}

// endString finishes the current string and returns it as an event.
//
// The body is a string, which every consumer will treat as text. Invalid UTF-8 is
// replaced rather than passed on, for the same reason it is in a paste: this is where
// untrusted bytes stop being untrusted.
func (p *Parser) endString() Event {
	body := strings.ToValidUTF8(string(p.strBody), "�")
	kind, cmd := p.str, p.oscCmd
	p.abandonString()
	if kind == dcsString {
		return DCS{Body: body}
	}
	return OSC{Command: cmd, Params: body}
}

// abandonString forgets whatever was being accumulated.
func (p *Parser) abandonString() {
	p.str, p.oscCmd, p.strBody = noString, 0, nil
}

// skipString drops the rest of a string whose body overran, and reports whether it
// found the end.
func (p *Parser) skipString() bool {
	for i := range len(p.buf) {
		switch p.buf[i] {
		case bel:
			p.take(i + 1)
			p.dropping = droppingNothing
			return true
		case esc:
			if i+1 >= len(p.buf) {
				p.take(i)
				return false // could still become ST
			}
			if p.buf[i+1] == '\\' {
				p.take(i + 2)
			} else {
				// The escape abandoned the string, and belongs to whatever comes next
				// rather than to this.
				p.take(i)
			}
			p.dropping = droppingNothing
			return true
		}
	}
	p.buf = nil
	return false
}

// report reads a control sequence that carried a private marker.
//
// Every one of these is a terminal answering rather than a person typing, so an
// unrecognised one is dropped. Reading it as a key would put whatever the terminal
// said into whatever had focus.
func (ps params) report(final byte) Event {
	switch {
	case ps.private == '<' && (final == 'M' || final == 'm'):
		return ps.mouse(final == 'M')
	case ps.private == '?' && final == 'c':
		return ps.deviceAttributes()
	case ps.private == '?' && final == 'u':
		return KeyboardFlags{Flags: max(ps.first(), 0)}
	case ps.private == '>' && final == 'c':
		return DeviceVersion{Kind: max(ps.at(0), 0), Version: max(ps.at(1), 0), Patch: max(ps.at(2), 0)}
	default:
		return nil
	}
}
