// Package publication owns admission and settlement of permanent content.
// It has no terminal writer: the session compositor commits accepted candidates
// together with its live frame and projects writer progress back into receipts.
package publication

import (
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/Tangerg/oolong/ui/core/grid"
)

// Admission errors transfer no responsibility and leave the producer-owned content intact.
var (
	ErrCapacity = errors.New("publication: no pending capacity")
	ErrTooLarge = errors.New("publication: item exceeds its budget")
	ErrClosed   = errors.New("publication: closed")
)

// Config bounds retained captures; zero limits use conservative per-item and session defaults.
type Config struct {
	Width           int
	Ground          grid.Ground
	MaxItems        int
	MaxCells        int
	MaxItemBytes    int64
	MaxPendingBytes int64
}

// ID identifies an accepted publication independently of its eventual writer sequence.
type ID uint64

// State records the strongest transport outcome known for an accepted publication.
type State uint8

// Publication states never infer a physical write from queue admission.
const (
	Accepted State = iota
	Written
	Failed
	UnknownPartial
)

// Status is a detached receipt value; Sequence is zero until writer admission.
type Status struct {
	ID       ID
	State    State
	Sequence uint64
	Err      error
}

// Receipt is a concurrency-safe projection of one accepted responsibility.
// Retaining it does not retain content or the publisher. ID is not a wire sequence.
type Receipt struct{ record *record }

type record struct {
	mu     sync.RWMutex
	status Status
}

// Snapshot may be read concurrently with owner-side settlement and retains no content.
func (r Receipt) Snapshot() Status {
	if r.record == nil {
		return Status{State: Failed, Err: ErrClosed}
	}
	r.record.mu.RLock()
	defer r.record.mu.RUnlock()
	return r.record.status
}

func (r *record) update(state State, sequence uint64, err error) {
	r.mu.Lock()
	r.status.State, r.status.Sequence, r.status.Err = state, sequence, err
	r.mu.Unlock()
}

// Contribution is immutable complete content waiting for the session compositor.
type Contribution struct {
	ID      ID
	Content grid.Frozen
}

type entry struct {
	contribution Contribution
	receipt      *record
	sequence     uint64
}

// Publisher belongs to the UI owner. Capacity covers both not-yet-composed and
// accepted-by-writer items; a slow writer cannot move growth into an unbounded
// second queue. Drawable callbacks run during Capture, before admission, and
// cannot reenter publisher mutation. Deferred progress subscribers run only in
// FlushNotifications after the compositor has settled all owners.
// A Publisher must not be copied after first use.
type Publisher struct {
	noCopy       noCopy
	cfg          Config
	next         ID
	bytes        int64
	items        int
	pending      []*entry
	inflight     []*entry
	preparing    bool
	closed       error
	admissionErr error
	lastSequence uint64
	dirty        bool
	notifying    bool
	listeners    []*listener
}

type listener struct{ fn func() }

type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// New creates a bounded owner for pending and writer-admitted permanent content.
func New(cfg Config) (*Publisher, error) {
	if cfg.Width <= 0 || cfg.MaxItems < 0 || cfg.MaxCells < 0 || cfg.MaxItemBytes < 0 || cfg.MaxPendingBytes < 0 {
		return nil, errors.New("publication: invalid configuration")
	}
	if cfg.MaxItems == 0 {
		cfg.MaxItems = 128
	}
	if cfg.MaxCells == 0 {
		cfg.MaxCells = 1 << 16
	}
	if cfg.MaxItemBytes == 0 {
		cfg.MaxItemBytes = 4 << 20
	}
	if cfg.MaxPendingBytes == 0 {
		cfg.MaxPendingBytes = 8 << 20
	}
	if cfg.MaxItemBytes > cfg.MaxPendingBytes {
		return nil, errors.New("publication: item budget exceeds pending budget")
	}
	return &Publisher{cfg: cfg}, nil
}

func (p *Publisher) assertIdle() {
	if p != nil && p.preparing {
		panic("publication: mutation during content capture")
	}
}

// SetGeometry affects future captures; already accepted cells remain immutable and reflow at composition.
func (p *Publisher) SetGeometry(width int, ground grid.Ground) error {
	p.assertIdle()
	if p == nil || p.closed != nil {
		return p.closedError()
	}
	if width <= 0 {
		return errors.New("publication: width must be positive")
	}
	p.cfg.Width, p.cfg.Ground = width, ground
	return nil
}

// TryPublish freezes a complete drawable before accepting responsibility; refusals leave it with the caller.
func (p *Publisher) TryPublish(body grid.Drawable) (Receipt, error) {
	p.assertIdle()
	if p == nil || p.closed != nil || p.admissionErr != nil {
		return Receipt{}, p.closedError()
	}
	if p.next == ID(math.MaxUint64) {
		return Receipt{}, errors.New("publication: identities exhausted")
	}
	p.preparing = true
	defer func() { p.preparing = false }()
	frozen, err := grid.Capture(grid.CaptureConfig{Content: body, Width: p.cfg.Width, Ground: p.cfg.Ground, MaxCells: p.cfg.MaxCells, MaxBytes: p.cfg.MaxItemBytes})
	if err != nil {
		if errors.Is(err, grid.ErrCaptureLimit) {
			return Receipt{}, fmt.Errorf("%w: %w", ErrTooLarge, err)
		}
		return Receipt{}, err
	}
	if frozen.RowCount() > 0 && p.items >= p.cfg.MaxItems || frozen.StoredBytes() > p.cfg.MaxPendingBytes-p.bytes {
		return Receipt{}, ErrCapacity
	}
	p.next++
	r := &record{status: Status{ID: p.next, State: Accepted}}
	if frozen.RowCount() == 0 {
		r.status.State = Written
		return Receipt{record: r}, nil
	}
	p.pending = append(p.pending, &entry{contribution: Contribution{ID: p.next, Content: frozen}, receipt: r})
	p.bytes += frozen.StoredBytes()
	p.items++
	p.dirty = true
	return Receipt{record: r}, nil
}

func (p *Publisher) closedError() error {
	if p == nil {
		return ErrClosed
	}
	cause := p.closed
	if cause == nil {
		cause = p.admissionErr
	}
	if cause == nil {
		return ErrClosed
	}
	if errors.Is(cause, ErrClosed) {
		return cause
	}
	return errors.Join(ErrClosed, cause)
}

// CloseAdmission ends new requests while preserving every accepted contribution
// for the compositor's final frame and writer drain. It is distinct from Fail,
// which settles the accepted responsibilities themselves.
func (p *Publisher) CloseAdmission(err error) {
	p.assertIdle()
	if p == nil || p.admissionErr != nil || p.closed != nil {
		return
	}
	if err == nil {
		err = ErrClosed
	}
	p.admissionErr = err
	p.dirty = true
}

// Batch names a pending prefix. Reading it cannot advance any receipt. Prepare
// and Commit belong to one owner turn with writer admission between them.
type Batch struct {
	owner   *Publisher
	entries []*entry
}

// Prepare snapshots the pending prefix without removing it or advancing any receipt.
func (p *Publisher) Prepare() Batch {
	p.assertIdle()
	if p == nil || p.closed != nil {
		return Batch{}
	}
	return Batch{owner: p, entries: append([]*entry(nil), p.pending...)}
}

// Items returns a detached list whose content values share immutable cell storage.
func (b Batch) Items() []Contribution {
	out := make([]Contribution, len(b.entries))
	for i, e := range b.entries {
		out[i] = e.contribution
	}
	return out
}

// Len counts complete contributions in this candidate prefix.
func (b Batch) Len() int { return len(b.entries) }

// Commit cannot fail after writer admission. A stale batch, wrong owner or
// non-monotonic sequence is a compositor programming error and panics.
func (p *Publisher) Commit(b Batch, sequence uint64) {
	p.assertIdle()
	if len(b.entries) == 0 {
		return
	}
	if p == nil || p.closed != nil || b.owner != p || len(b.entries) > len(p.pending) || sequence == 0 || sequence <= p.lastSequence {
		panic("publication: invalid batch commit")
	}
	for i, e := range b.entries {
		if p.pending[i] != e {
			panic("publication: stale batch commit")
		}
	}
	for _, e := range b.entries {
		e.sequence = sequence
		e.receipt.update(Accepted, sequence, nil)
	}
	p.inflight = append(p.inflight, b.entries...)
	clear(p.pending[:len(b.entries)])
	p.pending = p.pending[len(b.entries):]
	if len(p.pending) == 0 {
		p.pending = nil
	}
	p.lastSequence = sequence
	p.dirty = true
}

// Written projects the physical writer watermark and releases all covered inflight captures.
func (p *Publisher) Written(sequence uint64) {
	p.assertIdle()
	if p == nil {
		return
	}
	n := 0
	for _, e := range p.inflight {
		if e.sequence > sequence {
			break
		}
		e.receipt.update(Written, e.sequence, nil)
		p.bytes -= e.contribution.Content.StoredBytes()
		p.items--
		n++
	}
	clear(p.inflight[:n])
	p.inflight = p.inflight[n:]
	if len(p.inflight) == 0 {
		p.inflight = nil
	}
	if n > 0 {
		p.dirty = true
	}
}

// Fail terminates every accepted responsibility. Only items handed to a writer
// can have an unknown partial result; still-pending items are definitely unwritten.
func (p *Publisher) Fail(err error, unknownPartial bool) {
	p.assertIdle()
	if p == nil || p.closed != nil {
		return
	}
	if err == nil {
		err = ErrClosed
	}
	p.closed = err
	for _, e := range p.pending {
		e.receipt.update(Failed, 0, err)
	}
	state := Failed
	if unknownPartial {
		state = UnknownPartial
	}
	for _, e := range p.inflight {
		e.receipt.update(state, e.sequence, err)
	}
	clear(p.pending)
	clear(p.inflight)
	p.pending, p.inflight = nil, nil
	p.bytes, p.items = 0, 0
	p.dirty = true
}

// Close settles outstanding work, conservatively marking writer-admitted output as unknown partial.
func (p *Publisher) Close(err error) { p.Fail(err, p != nil && len(p.inflight) > 0) }

// Pending reports item and owned-byte usage across both pending and inflight responsibilities.
func (p *Publisher) Pending() (items int, bytes int64) {
	if p == nil {
		return 0, 0
	}
	return p.items, p.bytes
}

// Subscribe observes settled admission/progress on a later owner turn. It does
// not call fn during registration, candidate commit or transport settlement.
func (p *Publisher) Subscribe(fn func()) func() {
	p.assertIdle()
	if p == nil || p.closed != nil || fn == nil {
		return func() {}
	}
	l := &listener{fn: fn}
	p.listeners = append(p.listeners, l)
	return func() {
		p.assertIdle()
		l.fn = nil
		for index, entry := range p.listeners {
			if entry == l {
				copy(p.listeners[index:], p.listeners[index+1:])
				p.listeners[len(p.listeners)-1] = nil
				p.listeners = p.listeners[:len(p.listeners)-1]
				return
			}
		}
	}
}

// FlushNotifications is called after the compositor has settled all owners.
// Transitions requested by callbacks remain dirty for the next turn, so they
// cannot recursively observe a half-committed frame or starve terminal input.
func (p *Publisher) FlushNotifications() bool {
	p.assertIdle()
	if p == nil || !p.dirty || p.notifying {
		return false
	}
	p.dirty, p.notifying = false, true
	defer func() { p.notifying = false }()
	listeners := append([]*listener(nil), p.listeners...)
	for _, l := range listeners {
		if l.fn != nil {
			l.fn()
		}
	}
	active := p.listeners[:0]
	for _, l := range p.listeners {
		if p.closed != nil {
			l.fn = nil
		}
		if l.fn != nil {
			active = append(active, l)
		}
	}
	clear(p.listeners[len(active):])
	p.listeners = active
	return true
}
