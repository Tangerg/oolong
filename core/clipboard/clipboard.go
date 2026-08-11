// Package clipboard owns the OSC 52 channel that carries text between a program
// and the clipboard beside its user-facing terminal.
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
// refusal is reported — a terminal that will not do it simply does nothing — so an
// unanswered [Channel.Request] is ordinary. The channel expires it rather than
// treating an unrelated future OSC 52 as the requested text.
//
// Nothing here touches a terminal or a clipboard. A Channel produces byte strings
// and settles answers; terminal adapters own when and where those strings are sent.
package clipboard

import (
	"encoding/base64"
	"strings"
	"sync"
	"time"
)

// Command is OSC 52's operating system command number.
const Command = 52

// Selection is which of a terminal's two clipboards is meant.
type Selection uint8

const (
	// System is the clipboard a copy command fills and a paste command reads. It
	// is the zero value because it is what "the clipboard" means to nearly
	// everyone: the X11 primary selection is a convention of one windowing system,
	// and this one is universal.
	System Selection = iota
	// Primary is the X11 selection that middle-click pastes, filled by selecting
	// text rather than by any command. Terminals elsewhere ignore it.
	Primary
)

// maxPayload bounds how much text one sequence will carry.
//
// The bound exists because the far end has one too, and theirs is silent: a
// terminal handed more than it will take does not copy the first part, it discards
// the lot. Multiplexers impose another bound. A conservative size that survives
// both is more useful than a larger request that reports success and vanishes.
const maxPayload = 100_000

// responseTimeout bounds ownership of one read request. A response is a terminal
// round trip, including an SSH round trip when remote; ten seconds leaves room for a
// severely delayed connection without letting a terminal that silently refused the
// request arm an unrelated OSC 52 indefinitely.
const responseTimeout = 10 * time.Second

// Channel is one stateful OSC 52 path to a user-facing terminal.
//
// Its zero value is a direct path ready for use; a nil *Channel is inert. [New]
// additionally adapts encoding to a tmux session described by an environment. All
// methods are safe to call from different goroutines. A Channel must not be copied
// after first use.
type Channel struct {
	tmux bool

	mu        sync.Mutex
	pending   bool
	selection Selection
	until     time.Time
	now       func() time.Time
}

// New returns a channel suited to the terminal environment. lookup belongs to the
// terminal being driven: a remote adapter passes the accepted PTY environment, not
// the server process environment. Nil constructs the same direct channel as the
// zero value.
func New(lookup func(string) (string, bool)) *Channel {
	c := &Channel{}
	if lookup == nil {
		return c
	}
	if value, ok := lookup("TMUX"); ok && strings.TrimSpace(value) != "" {
		c.tmux = true
		return c
	}
	if value, ok := lookup("TERM"); ok && strings.HasPrefix(strings.ToLower(value), "tmux") {
		c.tmux = true
	}
	return c
}

// Copy returns the sequence that asks the terminal to put text on a clipboard.
//
// The text is base64-encoded, which is what makes this safe to send at all: the
// encoding's alphabet contains neither the escape byte nor the terminator, so no
// text — pasted, downloaded, or produced by something hostile upstream — can end
// the sequence early and have the rest of itself read as commands.
//
// It reports false for text too large to carry. See [Limit].
func (c *Channel) Copy(sel Selection, text string) (string, bool) {
	if c == nil || len(text) > maxPayload {
		return "", false
	}
	return c.wrap(encode(sel, base64.StdEncoding.EncodeToString([]byte(text)))), true
}

// Request starts one clipboard read and returns the sequence to send.
//
// It reports false while an earlier request is still eligible for an answer. OSC
// 52 has no request identity, so sending two and guessing which response belongs to
// which would make ownership weaker rather than concurrency stronger. A request a
// terminal silently refuses expires by itself.
func (c *Channel) Request(sel Selection) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock()
	if c.pending && !now.After(c.until) {
		return "", false
	}
	sel = normalized(sel)
	c.pending, c.selection = true, sel
	c.until = now.Add(responseTimeout)
	return c.wrap(encode(sel, "?")), true
}

// Answer settles a terminal's OSC 52 parameters as text when they answer the live
// request for the same selection.
//
// A syntactically valid answer for the requested selection settles the request even
// when its payload cannot be decoded; treating that payload as an empty success would
// clear a selection. Malformed parameters and answers for another selection are left
// alone for the terminal adapter to publish as the raw OSC event it received, and do
// not take ownership of the live request.
func (c *Channel) Answer(params string) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.pending {
		return "", false
	}
	if c.clock().After(c.until) {
		c.pending = false
		return "", false
	}
	selection, ok := selectionOf(params)
	if !ok || selection != c.selection {
		return "", false
	}
	c.pending = false
	_, text, ok := parse(params)
	return text, ok
}

func (c *Channel) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *Channel) wrap(sequence string) string {
	if !c.tmux || sequence == "" {
		return sequence
	}
	// tmux DCS passthrough doubles every escape in the inner sequence. This stays
	// here rather than becoming a generic terminal wrapper: it is a property of this
	// capability, and unrelated protocols have different transport contracts.
	inner := strings.ReplaceAll(sequence, "\x1b", "\x1b\x1b")
	return "\x1bPtmux;" + inner + "\x1b\\"
}

func normalized(sel Selection) Selection {
	if sel != System && sel != Primary {
		return System
	}
	return sel
}

func selectionOf(params string) (Selection, bool) {
	selection, _, found := strings.Cut(params, ";")
	if !found || len(selection) != 1 {
		return 0, false
	}
	switch selection[0] {
	case 'c':
		return System, true
	case 'p':
		return Primary, true
	default:
		return 0, false
	}
}

// parse reads the text out of a terminal's answer. The request lifecycle belongs to
// Channel.Answer; keeping this decoder private prevents a caller from accepting an
// answer it never asked for.
func parse(params string) (Selection, string, bool) {
	which, ok := selectionOf(params)
	if !ok {
		return 0, "", false
	}
	_, payload, _ := strings.Cut(params, ";")
	if payload == "" {
		return 0, "", false
	}
	if len(payload) > base64.StdEncoding.EncodedLen(maxPayload) {
		return 0, "", false
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(raw) > maxPayload {
		return 0, "", false
	}
	return which, strings.ToValidUTF8(string(raw), "�"), true
}

// encode wraps a payload in the sequence that carries it. ST is used rather than
// BEL because multiplexers recognise the standard-named terminator more reliably.
func encode(sel Selection, payload string) string {
	sel = normalized(sel)
	var b strings.Builder
	b.Grow(len("\x1b]52;c;") + len(payload) + len("\x1b\\"))
	b.WriteString("\x1b]52;")
	b.WriteByte(selectionCode(sel))
	b.WriteByte(';')
	b.WriteString(payload)
	b.WriteString("\x1b\\")
	return b.String()
}

func selectionCode(sel Selection) byte {
	if normalized(sel) == Primary {
		return 'p'
	}
	return 'c'
}

// Limit is the largest text [Channel.Copy] will carry, in bytes. It is here so a
// refusal can be explained as a size instead of appearing as a copy that did nothing.
func Limit() int { return maxPayload }
