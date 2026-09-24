package input

import (
	"github.com/Tangerg/oolong/core/ansi"
)

const (
	// bel is the older of the two ways a terminal ends a string it is sending. The
	// other is ST: an escape and a backslash.
	bel = ansi.Bell

	// Each introducer is also what a terminal sends for a chord — Alt+] and
	// Alt+Shift+P — and the byte alone cannot tell the two apart, so each is decoded
	// only when what follows looks like an answer. See [oscHead] and [dcsHead].
	//
	// The rest of the family is left undecoded: catching a start-of-string or a privacy
	// message would cost Alt+Shift+X and Alt+^ for sequences nothing here asks for.
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
