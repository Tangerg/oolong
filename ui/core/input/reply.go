package input

import (
	"bytes"
	"strings"

	"github.com/Tangerg/oolong/ui/core/ansi"
	"github.com/Tangerg/oolong/ui/core/graphics"
)

const (
	bel = ansi.Bell

	// maxOSCCommand bounds how many digits a command number may have. The longest in
	// use is four — iTerm2's 1337 — so five is room to spare and a bound all the same.
	maxOSCCommand = 5

	// maxStringBody bounds what one string's body may accumulate.
	//
	// It is far larger than maxParameterBytes because one of these legitimately carries
	// a clipboard: the answer to a read of the terminal's selection is the whole of
	// it, base64-encoded. It is a bound all the same, because a terminal that opens a
	// string and never ends it must not be able to grow memory without limit.
	maxStringBody = 1 << 20
)

// The returned size is how many bytes the introducer, the number and its separator
// took, and its sign is the verdict: positive for a command, zero when too few bytes
// have arrived to tell, and negative when what follows the introducer is not a number
// — which means it was the keystroke Alt+] and not an introducer at all.
//
// A terminator directly after the number is left unconsumed for [Parser.readString] to
// find, which is what makes a command with no parameters decode as one rather than as
// two halves of nothing.
func oscHead(b []byte) (cmd, size int) {
	i := introducerLen
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		if i-introducerLen == maxOSCCommand {
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
	if i == introducerLen {
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
	const marker = introducerLen
	if len(b) <= marker {
		return 0
	}
	switch {
	case b[marker] == versionReplyMarker:
		return introducerLen
	case b[marker] >= '0' && b[marker] <= '9':
		if len(b) <= marker+1 {
			return 0
		}
		if b[marker+1] == statusReplyMarker || b[marker+1] == capabilityReplyMarker {
			return introducerLen
		}
		return -1
	default:
		return -1
	}
}

// Markers after a device-control string's leading digit: the answers to a setting
// query (DECRQSS) and to a capability query (XTGETTCAP).
const (
	statusReplyMarker     = '$'
	capabilityReplyMarker = '+'
)

func graphicsHead(b []byte) int {
	matched, pending := graphics.MatchReplyPrefix(b[introducerLen:])
	if pending {
		return 0
	}
	if matched {
		return introducerLen
	}
	return -1
}

// controlString is the body of the string sequence a stringFrame reads. A
// refused string keeps only its framing, and an OSC its bounded correlation
// header, so its remaining bytes cannot gain keyboard authority.
type controlString struct {
	family ansi.StringFamily
	// command is the OSC command number; other families have none.
	command int
	body    []byte
	refused bool
}

func (p *Parser) introduceString(b []byte, family ansi.StringFamily, cmd, size int) (int, Event, bool) {
	switch {
	case size < 0:
		return decodeAlt(b)
	case size == 0:
		return 0, nil, false
	default:
		p.frame, p.str = stringFrame, controlString{family: family, command: cmd}
		return size, nil, true
	}
}

func (p *Parser) readString() (Event, bool) {
	for i := 0; i < len(p.buf); {
		n, result := p.str.family.Step(p.buf[i], p.buf[min(i+1, len(p.buf)-1)], i+1 < len(p.buf))
		switch result {
		case ansi.StringTerminated:
			p.take(i + n)
			return p.endString(), true
		case ansi.StringIncomplete:
			p.take(i)
			return nil, false
		case ansi.StringCanceled:
			p.take(i + n)
			p.abandonString()
			return nil, true
		case ansi.StringContent:
			i++
			if p.str.refused {
				continue
			}
			p.str.body = append(p.str.body, p.buf[i-1])
			if len(p.str.body) >= maxStringBody {
				// Overran what one may hold. The rest is dropped wherever it turns up
				// rather than read as text, for the same reason a runaway control
				// sequence is: a flood of keystrokes is a worse answer to a malformed
				// sequence than silence, and one a hostile terminal could aim.
				p.take(i)
				p.str.refuse()
				return nil, true
			}
		}
	}
	p.take(len(p.buf))
	return nil, false
}

func (s *controlString) refuse() {
	var header []byte
	if s.family == ansi.OSC {
		if end := bytes.IndexByte(s.body, ';'); end >= 0 && end < maxParameterBytes {
			header = bytes.Clone(s.body[:end+1])
		}
	}
	s.body, s.refused = header, true
}

// Reply bodies become text at this boundary; replace invalid UTF-8 before
// consumers can treat the result as displayable text.
func (p *Parser) endString() Event {
	str := p.str
	p.abandonString()
	return str.event()
}

func (s *controlString) event() Event {
	body := strings.ToValidUTF8(string(s.body), replacement)
	switch {
	case s.refused && s.family == ansi.OSC:
		return OSCRejection{Command: s.command, Prefix: body}
	case s.refused:
		return nil
	case s.family == ansi.APC:
		return APC{Body: body}
	case s.family == ansi.DCS:
		return DCS{Body: body}
	default:
		return OSC{Command: s.command, Params: body}
	}
}

func (p *Parser) abandonString() {
	p.frame, p.str = noFrame, controlString{}
}
