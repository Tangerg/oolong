// Package clipboard owns OSC 52 requests and replies for the user-facing terminal,
// which may be remote from the process. Terminal refusal is silent, so unanswered
// requests expire before unrelated replies can claim them. [Channel] handles protocol
// state; adapters own sending the bytes.
package clipboard

import (
	"encoding/base64"
	"strconv"
	"strings"
	"sync"
	"time"
)

const Command = 52

// Selection uses wire identities so encoding and reply matching cannot drift.
// Zero selects System at admission; an unknown value is refused.
type Selection byte

const (
	System  Selection = 'c'
	Primary Selection = 'p'
)

type Config struct {
	// tmux's native OSC 52 path needs no wrapping. Passthrough is separate
	// terminal policy and must be enabled by whoever owns the tmux session.
	TmuxPassthrough bool
}

// MaxPayload is the largest text [Channel.Copy] will carry, in bytes.
//
// The bound exists because the far end has one too, and theirs is silent: a
// terminal handed more than it will take does not copy the first part, it discards
// the lot. Multiplexers impose another bound. A conservative size that survives
// both is more useful than a larger request that reports success and vanishes. The
// exported constant lets an application explain a refusal before it looks like a
// copy that did nothing.
const MaxPayload = 100_000

// responseTimeout bounds ownership of one read request. A response is a terminal
// round trip, including an SSH round trip when remote; ten seconds leaves room for a
// severely delayed connection without letting a terminal that silently refused the
// request arm an unrelated OSC 52 indefinitely.
const responseTimeout = 10 * time.Second

// Channel is one stateful OSC 52 path to a user-facing terminal.
//
// Its zero value is a direct path ready for use; a nil *Channel is inert. [New]
// selects passthrough only when explicitly configured. All methods are safe to
// call from different goroutines. A Channel must not be copied
// after first use.
type Channel struct {
	passthrough bool

	mu        sync.Mutex
	selection Selection
	until     time.Time
	now       func() time.Time
}

func New(cfg Config) *Channel { return &Channel{passthrough: cfg.TmuxPassthrough} }

// Copy returns the sequence that asks the terminal to put text on a clipboard.
//
// The text is base64-encoded, which is what makes this safe to send at all: the
// encoding's alphabet contains neither the escape byte nor the terminator, so no
// text — pasted, downloaded, or produced by something hostile upstream — can end
// the sequence early and have the rest of itself read as commands.
//
// It reports false for text too large to carry. See [MaxPayload].
func (c *Channel) Copy(sel Selection, text string) (string, bool) {
	selection, valid := normalized(sel)
	if c == nil || !valid || len(text) > MaxPayload {
		return "", false
	}
	return c.wrap(encode(selection, base64.StdEncoding.EncodeToString([]byte(text)))), true
}

// Request starts one clipboard read only when send accepts its sequence.
//
// It reports false while an earlier request is still eligible for an answer. OSC
// 52 has no request identity, so sending two and guessing which response belongs to
// which would make ownership weaker rather than concurrency stronger. A request a
// terminal silently refuses expires by itself. A nil send refuses the request.
// send must enqueue synchronously without blocking for delivery or reentering this
// Channel. It runs under the request lock, so an answer cannot settle a proposal
// before transport admission decides whether that request exists.
func (c *Channel) Request(sel Selection, send func(string) bool) bool {
	selection, valid := normalized(sel)
	if c == nil || send == nil || !valid {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock()
	if c.live(now) {
		return false
	}
	if !send(c.wrap(encode(selection, "?"))) {
		return false
	}
	c.selection = selection
	c.until = now.Add(responseTimeout)
	return true
}

// Answer settles a live request only for its selection and receive interval.
// A matching reply consumes the request even when its payload is invalid or empty;
// in that case it returns false and produces no paste. Foreign or unrecognized
// selections leave the request pending. at is transport receive time before
// queueing: pre-request and deadline replies cannot settle, while delayed decoding
// preserves timely replies.
func (c *Channel) Answer(params string, at time.Time) (string, bool) {
	if c == nil {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.live(at) {
		return "", false
	}
	selection, payload, ok := splitAnswer(params)
	if !ok || selection != c.selection {
		return "", false
	}
	c.until = time.Time{}
	return decodePayload(payload)
}

// CancelPending revokes the outstanding read when another foreground owner takes
// the terminal. Its reply can no longer be attributed to this session.
func (c *Channel) CancelPending() {
	if c != nil {
		c.mu.Lock()
		c.until = time.Time{}
		c.mu.Unlock()
	}
}

// live is the one ownership rule for both issuing a request and settling its answer.
// A deadline is the first instant no request is live, matching context and network
// deadline semantics; keeping the comparison here prevents the two operations from
// disagreeing at that exact boundary.
func (c *Channel) live(at time.Time) bool {
	return !c.until.IsZero() && !at.Before(c.until.Add(-responseTimeout)) && at.Before(c.until)
}

func (c *Channel) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *Channel) wrap(sequence string) string {
	if !c.passthrough || sequence == "" {
		return sequence
	}
	// tmux DCS passthrough doubles every escape in the inner sequence. This stays
	// here rather than becoming a generic terminal wrapper: it is a property of this
	// capability, and unrelated protocols have different transport contracts.
	inner := strings.ReplaceAll(sequence, "\x1b", "\x1b\x1b")
	return "\x1bPtmux;" + inner + "\x1b\\"
}

func normalized(sel Selection) (Selection, bool) {
	switch sel {
	case 0:
		return System, true
	case System, Primary:
		return sel, true
	default:
		return 0, false
	}
}

func splitAnswer(params string) (Selection, string, bool) {
	selection, payload, found := strings.Cut(params, ";")
	if !found || len(selection) != 1 {
		return 0, "", false
	}
	sel := Selection(selection[0])
	return sel, payload, sel == System || sel == Primary
}

func decodePayload(payload string) (string, bool) {
	if len(payload) > base64.StdEncoding.EncodedLen(MaxPayload) {
		return "", false
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(raw) > MaxPayload {
		return "", false
	}
	// Emptiness is decided after decoding, not before. The encoding ignores line
	// breaks, so a payload of one newline is a non-empty field carrying no text at
	// all — and an answer with nothing in it, delivered as a paste, replaces what the
	// user had selected with nothing.
	if len(raw) == 0 {
		return "", false
	}
	return strings.ToValidUTF8(string(raw), "�"), true
}

// encode wraps a payload in the sequence that carries it. ST is used rather than
// BEL because multiplexers recognise the standard-named terminator more reliably.
func encode(sel Selection, payload string) string {
	var b strings.Builder
	b.WriteString("\x1b]")
	b.WriteString(strconv.Itoa(Command))
	b.WriteByte(';')
	b.WriteByte(byte(sel))
	b.WriteByte(';')
	b.WriteString(payload)
	b.WriteString("\x1b\\")
	return b.String()
}
