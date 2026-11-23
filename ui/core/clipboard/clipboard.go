// Package clipboard owns OSC 52 queries and their uncorrelated replies. Cancelling
// a read ends delivery, not the terminal's ability to answer it. Adapters own transport.
package clipboard

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/ansi"
)

const (
	queryPayload = "?"
	escape       = string(rune(ansi.Escape))
)

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

// A silent terminal refusal must not leave the caller waiting for the session's
// lifetime. This deadline never releases the uncorrelated protocol slot.
const responseTimeout = 10 * time.Second

var (
	ErrTooLarge    = errors.New("clipboard: text exceeds MaxPayload")
	ErrPending     = errors.New("clipboard: an earlier query remains unanswered")
	ErrSelection   = errors.New("clipboard: unsupported selection")
	ErrResponse    = errors.New("clipboard: invalid response")
	ErrInterrupted = errors.New("clipboard: terminal ownership interrupted")
)

// Channel owns one OSC 52 path. Its zero value is ready and a nil receiver refuses
// operations. Methods are concurrency-safe; a Channel must not be copied after use.
// An unanswered query continues to own the path after its caller stops waiting.
// A transport must keep the same Channel for its reply stream: constructing another
// one cannot make that stream's uncorrelated outstanding replies safe to reuse.
type Channel struct {
	passthrough bool
	mu          sync.Mutex
	pending     *query
}

type query struct {
	selection      Selection
	started, until time.Time
	// A nil delivery means its one result has already been settled. The query
	// itself stays pending until a matching wire reply discharges it.
	delivery chan readResult
}

type readResult struct {
	text string
	err  error
}

func New(cfg Config) *Channel { return &Channel{passthrough: cfg.TmuxPassthrough} }

// Copy returns the sequence that asks the terminal to put text on a clipboard.
//
// The text is base64-encoded, which is what makes this safe to send at all: the
// encoding's alphabet contains neither the escape byte nor the terminator, so no
// text — pasted, downloaded, or produced by something hostile upstream — can end
// the sequence early and have the rest of itself read as commands.
//
// A nil Channel refuses with [errors.ErrUnsupported], like [Channel.Read]; text
// larger than [MaxPayload] is refused with [ErrTooLarge].
func (c *Channel) Copy(sel Selection, text string) (string, error) {
	selection, valid := normalized(sel)
	switch {
	case c == nil:
		return "", errors.ErrUnsupported
	case !valid:
		return "", ErrSelection
	case len(text) > MaxPayload:
		return "", ErrTooLarge
	}
	return c.wrap(encode(selection, base64.StdEncoding.EncodeToString([]byte(text)))), nil
}

// Read waits for one query's result, caller cancellation, or the response deadline.
// send must admit synchronously without waiting for delivery or reentering Channel.
// An error from send must guarantee that no bytes or queued write escaped admission;
// physical delivery errors after admission cannot be reported as send errors.
// A send error is returned unchanged and creates no query. ErrPending means an
// earlier query still owns the wire, even if its caller timed out or cancelled.
//
// The first result settled under the channel lock wins. A decoded reply received
// before admission or at the same clock instant cannot settle this read because
// those timestamps cannot prove that the reply followed the query.
// A reply received at/after its deadline settles DeadlineExceeded. Timely bytes
// decoded after cancellation has settled cannot revive delivery. A cancellation
// signal racing with reply settlement does not retract a reply that settles first.
// A settled result remains successful even if the caller reads it later. Empty
// clipboard contents are a successful empty result.
func (c *Channel) Read(ctx context.Context, sel Selection, send func(string) error) (string, error) {
	selection, valid := normalized(sel)
	if c == nil || send == nil {
		return "", errors.ErrUnsupported
	}
	if !valid {
		return "", ErrSelection
	}
	if err := ctx.Err(); err != nil {
		return "", context.Cause(ctx)
	}
	delivery := make(chan readResult, 1)
	q := &query{selection: selection, delivery: delivery}
	if err := c.admit(ctx, q, send); err != nil {
		return "", err
	}
	timer := time.NewTimer(time.Until(q.until))
	defer timer.Stop()
	select {
	case result := <-delivery:
		return result.text, result.err
	case <-ctx.Done():
		c.settle(q, readResult{err: context.Cause(ctx)})
	case <-timer.C:
		c.settle(q, readResult{err: context.DeadlineExceeded})
	}
	result := <-delivery
	return result.text, result.err
}

// Answer consumes a matching query reply, including invalid or empty payloads and
// replies whose delivery has been revoked. Only consumption releases the wire slot.
// Foreign selections and bytes not provably received after admission remain
// unrelated input; equality cannot establish causal order or release the wire slot.
// started is the first reply byte's receive time and completed is the last one's.
// These are transport observations, not the later time the reply was decoded.
func (c *Channel) Answer(params string, started, completed time.Time) bool {
	return c.answer(params, started, completed, false)
}

// Reject consumes a complete reply whose payload the decoder refused to retain.
// prefix must contain the complete selection and its semicolon. A partial frame
// is not a reply: only its terminator authorizes this call. Matching and timing
// follow Answer; a timely live result settles ErrResponse and a revoked result stays
// revoked. An unrelated or incomplete selection cannot release the wire slot.
func (c *Channel) Reject(prefix string, started, completed time.Time) bool {
	return c.answer(prefix, started, completed, true)
}

func (c *Channel) answer(params string, started, completed time.Time, rejected bool) bool {
	if c == nil || completed.Before(started) {
		return false
	}
	selection, payload, valid := splitAnswer(params)
	if !valid {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	q := c.pending
	if q == nil || selection != q.selection || !started.After(q.started) {
		return false
	}
	c.pending = nil
	result := readResult{err: ErrResponse}
	if !completed.Before(q.until) {
		result.err = context.DeadlineExceeded
	} else if !rejected {
		result.text, result.err = decodePayload(payload)
	}
	q.settle(result)
	return true
}

// Interrupt revokes delivery when foreground ownership changes. It does not
// establish that the terminal forgot the query, so its wire slot remains occupied.
func (c *Channel) Interrupt() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending != nil {
		c.pending.settle(readResult{err: ErrInterrupted})
	}
}

func (c *Channel) settle(q *query, result readResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	q.settle(result)
}

func (q *query) settle(result readResult) {
	if q.delivery != nil {
		q.delivery <- result
		q.delivery = nil
	}
}

func (c *Channel) admit(ctx context.Context, q *query, send func(string) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending != nil {
		return ErrPending
	}
	if err := ctx.Err(); err != nil {
		return context.Cause(ctx)
	}
	q.started = time.Now()
	q.until = q.started.Add(responseTimeout)
	c.pending = q
	if err := send(c.wrap(encode(q.selection, queryPayload))); err != nil {
		c.pending = nil
		return err
	}
	return nil
}

func (c *Channel) wrap(sequence string) string {
	if !c.passthrough || sequence == "" {
		return sequence
	}
	// tmux DCS passthrough doubles every escape in the inner sequence. This stays
	// here rather than becoming a generic terminal wrapper: it is a property of this
	// capability, and unrelated protocols have different transport contracts.
	inner := strings.ReplaceAll(sequence, escape, escape+escape)
	return ansi.DCSIntroducer + "tmux;" + inner + ansi.ST
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

func decodePayload(payload string) (string, error) {
	if len(payload) > base64.StdEncoding.EncodedLen(MaxPayload) {
		return "", ErrResponse
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(raw) > MaxPayload {
		return "", ErrResponse
	}
	value := strings.ToValidUTF8(string(raw), string(utf8.RuneError))
	if len(value) > MaxPayload {
		return "", ErrResponse
	}
	return value, nil
}

// encode wraps a payload in the sequence that carries it. ST is used rather than
// BEL because multiplexers recognise the standard-named terminator more reliably.
func encode(sel Selection, payload string) string {
	var b strings.Builder
	b.WriteString(ansi.OSCPrefix(ansi.OSCClipboard))
	b.WriteByte(byte(sel))
	b.WriteByte(';')
	b.WriteString(payload)
	b.WriteString(ansi.ST)
	return b.String()
}
