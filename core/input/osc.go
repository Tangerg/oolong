package input

import "strings"

const (
	// bel is the older of the two ways a terminal ends an operating system
	// command. The other is ST: an escape and a backslash.
	bel = 0x07

	// oscIntro is the byte that introduces an operating system command.
	//
	// It is also what a terminal sends for Alt+], and every other
	// string-introducing byte has the same clash: Alt+Shift+P is
	// indistinguishable from the start of a device control string, Alt+Shift+X
	// from a start-of-string. The introducer alone cannot tell them apart.
	//
	// So this package decodes the one string a terminal actually answers with,
	// and decodes it only when what follows looks like an answer — a command
	// number and a separator. Anything else is the keystroke. The rest of the
	// family is not decoded at all: doing so would cost a keystroke each to catch
	// sequences nothing here ever asks for, and a sequence nobody asked for does
	// not arrive.
	oscIntro = ']'

	// maxOSCCommand bounds how many digits a command number may have. The longest
	// in use is four — iTerm2's 1337 — so five is room to spare and a bound all
	// the same.
	maxOSCCommand = 5

	// maxOSCParams bounds what one command's parameters may accumulate.
	//
	// It is far larger than maxSequenceBody because one of these legitimately
	// carries a clipboard: the answer to a read of the terminal's selection is
	// the whole of it, base64-encoded. It is a bound all the same, because a
	// terminal that opens a command and never ends it must not be able to grow
	// memory without limit.
	maxOSCParams = 1 << 20
)

// oscHead reads the command number at the start of an operating system command.
//
// The returned size is how many bytes the introducer, the number and its
// separator took, and its sign is the verdict: positive for a command, zero when
// too few bytes have arrived to tell, and negative when what follows the
// introducer is not a number — which means it was the keystroke Alt+] and not an
// introducer at all. See [oscIntro].
//
// A terminator directly after the number is left unconsumed for [Parser.readOSC]
// to find, which is what makes a command with no parameters at all decode as one
// rather than as two halves of nothing.
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

// beginOSC starts accumulating the parameters of command cmd.
func (p *Parser) beginOSC(cmd int) {
	p.inOSC, p.oscCmd, p.oscParams = true, cmd, nil
}

// readOSC moves buffered bytes into the current command's parameters until its
// terminator arrives, reporting the finished command when one does.
//
// Two byte sequences end it: the BEL older terminals use, and the ST that
// ECMA-48 specifies, which is an escape and a backslash. An escape followed by
// anything else never legitimately appears inside one, so it is taken to have
// abandoned the command: the parameters so far are dropped and the escape is read
// again on its own terms. That is what makes a terminal which stops mid-answer
// cost one keystroke rather than every keystroke after it.
func (p *Parser) readOSC() (Event, bool) {
	for i := 0; i < len(p.buf); {
		switch c := p.buf[i]; c {
		case bel:
			p.take(i + 1)
			return p.endOSC(), true
		case esc:
			if i+1 >= len(p.buf) {
				// What is left could still become ST. Keep the escape.
				p.take(i)
				return nil, false
			}
			if p.buf[i+1] == '\\' {
				p.take(i + 2)
				return p.endOSC(), true
			}
			p.take(i)
			p.inOSC, p.oscCmd, p.oscParams = false, 0, nil
			return nil, true
		default:
			p.oscParams = append(p.oscParams, c)
			i++
			if len(p.oscParams) >= maxOSCParams {
				// Overran what one may hold. The rest is dropped wherever it turns
				// up rather than read as text, for the same reason a runaway control
				// sequence is: a flood of keystrokes is a worse answer to a
				// malformed sequence than silence, and one a hostile terminal could
				// aim.
				p.take(i)
				p.inOSC, p.oscCmd, p.oscParams = false, 0, nil
				p.dropping = droppingString
				return nil, true
			}
		}
	}
	p.buf = nil
	return nil, false
}

// endOSC finishes the current command and returns it as an event.
//
// The parameters are a string, which every consumer will treat as text. Invalid
// UTF-8 is replaced rather than passed on, for the same reason it is in a paste:
// this is where untrusted bytes stop being untrusted.
func (p *Parser) endOSC() Event {
	ev := OSC{Command: p.oscCmd, Params: strings.ToValidUTF8(string(p.oscParams), "�")}
	p.inOSC, p.oscCmd, p.oscParams = false, 0, nil
	return ev
}

// skipString drops the rest of an operating system command whose parameters
// overran, and reports whether it found the end.
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
				// The escape abandoned the command, and belongs to whatever comes
				// next rather than to this.
				p.take(i)
			}
			p.dropping = droppingNothing
			return true
		}
	}
	p.buf = nil
	return false
}
