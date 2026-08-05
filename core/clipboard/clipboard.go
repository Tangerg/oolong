// Package clipboard encodes the sequences a terminal uses to carry text to and
// from the system clipboard.
//
// A terminal is often not where the program is. Over ssh, in a container, inside a
// multiplexer on another machine, the tools that reach a clipboard directly —
// pbcopy, wl-copy, xclip — reach the wrong one or none at all. The terminal is the
// only thing on the user's side of the connection, so asking it to do the copying
// is the only approach that works everywhere the same way.
//
// That is what OSC 52 is. It is also a capability a terminal may refuse: writing is
// commonly allowed and reading commonly is not, because a program that can read the
// clipboard can read whatever the user copied out of a password manager. Neither
// refusal is reported — a terminal that will not do it simply does nothing — so a
// [Request] that goes unanswered is the ordinary case and not an error.
//
// Nothing here touches a terminal or a clipboard. These are byte strings and the
// text they carry, which is what lets every claim in this package be a test.
package clipboard

import (
	"encoding/base64"
	"strings"
)

// Selection is which of a terminal's two clipboards is meant.
type Selection byte

const (
	// System is the clipboard a copy command fills and a paste command reads. It
	// is the zero value because it is what "the clipboard" means to nearly
	// everyone: the X11 primary selection is a convention of one windowing system,
	// and this one is universal.
	System Selection = 'c'
	// Primary is the X11 selection that middle-click pastes, filled by selecting
	// text rather than by any command. Terminals elsewhere ignore it.
	Primary Selection = 'p'
)

// maxPayload bounds how much text one sequence will carry.
//
// The bound exists because the far end has one too, and theirs is silent: a
// terminal handed more than it will take does not copy the first part, it discards
// the lot. xterm's limit is configurable and small by default, and a multiplexer in
// the middle has its own. So a copy this size is refused here, where a caller can
// be told, rather than being written and quietly lost.
const maxPayload = 1 << 20

// Copy is the sequence that asks the terminal to put text on a clipboard.
//
// The text is base64-encoded, which is what makes this safe to send at all: the
// encoding's alphabet contains neither the escape byte nor the terminator, so no
// text — pasted, downloaded, or produced by something hostile upstream — can end
// the sequence early and have the rest of itself read as commands.
//
// It reports false for text too large to carry. See [maxPayload].
func Copy(sel Selection, text string) (string, bool) {
	if len(text) > maxPayload {
		return "", false
	}
	return encode(sel, base64.StdEncoding.EncodeToString([]byte(text))), true
}

// Clear is the sequence that empties a clipboard.
//
// It is a copy of nothing rather than a command of its own, which is all the
// protocol offers: an empty payload is how it is spelled.
func Clear(sel Selection) string { return encode(sel, "") }

// Request is the sequence that asks a terminal what is on a clipboard.
//
// The answer does not come back from here. It arrives on the terminal's input as
// an operating system command numbered 52, mixed in with whatever the user is
// typing, and [Parse] reads it. Most terminals will not answer at all — see the
// package comment — so nothing should wait on one.
func Request(sel Selection) string { return encode(sel, "?") }

// Parse reads the text out of a terminal's answer to a [Request].
//
// The argument is the parameters of the operating system command, which is
// everything after the command number: a selection, a semicolon, and the text
// base64-encoded.
//
// It reports false for anything it cannot read, and that is the common case worth
// getting right rather than an edge: a terminal that declines to answer may still
// answer with nothing, and a decoder that turned that into an empty successful
// paste would clear whatever the user had selected.
func Parse(params string) (Selection, string, bool) {
	sel, payload, found := strings.Cut(params, ";")
	if !found || len(sel) != 1 {
		return 0, "", false
	}
	// The reply's selection is echoed back from the request, so a terminal that
	// answers about something nobody asked about is not answering this.
	which := Selection(sel[0])
	if which != System && which != Primary {
		return 0, "", false
	}
	if payload == "" {
		return 0, "", false
	}
	// Two bounds, and neither replaces the other. The first refuses an answer that
	// could not possibly be within the limit without decoding it into memory. The
	// second is the limit itself, and it has to come after: an encoded length covers
	// three decoded ones, differing in how much padding they end with, so the cheap
	// check cannot tell the largest allowed answer from one two bytes over.
	if len(payload) > base64.StdEncoding.EncodedLen(maxPayload) {
		return 0, "", false
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(raw) > maxPayload {
		return 0, "", false
	}
	// The bytes came from a terminal, which got them from somewhere else again, and
	// they are about to become a string that ends up in a document and then in a
	// cell. This is where untrusted bytes stop being untrusted.
	return which, strings.ToValidUTF8(string(raw), "�"), true
}

// encode wraps a payload in the sequence that carries it.
//
// The terminator is ST rather than BEL. Both are accepted everywhere, and BEL is
// the older spelling, but a multiplexer that passes sequences through by pattern is
// likelier to recognise the one the standard names.
func encode(sel Selection, payload string) string {
	if sel != System && sel != Primary {
		sel = System
	}
	var b strings.Builder
	b.Grow(len("\x1b]52;c;") + len(payload) + len("\x1b\\"))
	b.WriteString("\x1b]52;")
	b.WriteByte(byte(sel))
	b.WriteByte(';')
	b.WriteString(payload)
	b.WriteString("\x1b\\")
	return b.String()
}

// Limit is the largest text [Copy] will carry, in bytes. It is here so that a
// refusal can be explained to the user as a size rather than appearing as a copy
// that did nothing.
func Limit() int { return maxPayload }
