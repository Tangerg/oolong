package content

import (
	"context"
	"errors"
	"math"
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
)

// Source and preparation errors preserve the failed stage without advancing it.
var (
	ErrSourceClosed     = errors.New("content: source already ended")
	ErrSourceLimit      = errors.New("content: source exceeds its byte budget")
	ErrStalePreparation = errors.New("content: preparation does not match the current source")
	ErrAlreadyPrepared  = errors.New("content: revision already prepared")
)

// Revision identifies one immutable source version within its document.
type Revision uint64

// DocumentState combines source completion with the current preparation result.
type DocumentState uint8

// Document states distinguish provisional previews, final results and useful partial results.
const (
	Receiving DocumentState = iota
	Preparing
	Finalizing
	Final
	Partial
	Failed
)

// StreamConfig selects syntax and bounds retained source bytes; zero MaxSourceBytes uses 4 MiB.
type StreamConfig struct {
	Format         Format
	MaxSourceBytes int
}

// SourceSnapshot is immutable input for worker-side preparation. Its identity
// prevents an equal revision from another document being accepted accidentally.
type SourceSnapshot struct {
	owner    *byte
	revision Revision
	format   Format
	source   string
	final    bool
}

// Revision identifies the source version captured by this snapshot.
func (s SourceSnapshot) Revision() Revision { return s.revision }

// Format selects the renderer without inspecting source text.
func (s SourceSnapshot) Format() Format { return s.format }

// Text returns immutable complete source bytes, including any incomplete final UTF-8 sequence.
func (s SourceSnapshot) Text() string { return s.source }

// Final reports source EOF, not whether a final preparation has been accepted.
func (s SourceSnapshot) Final() bool { return s.final }

// Prepared transfers its passive result to the document on Accept. Renderers
// must stop mutating that result before returning it; there is no hidden second
// parse or renderer running on the UI goroutine.
type Prepared struct {
	source SourceSnapshot
	body   grid.Drawable
	err    error
}

// Prepare renders this complete snapshot and may run on a worker; it changes no document state.
func (s SourceSnapshot) Prepare(ctx context.Context, render Renderer) Prepared {
	result := Prepared{source: s}
	if err := context.Cause(ctx); err != nil {
		result.err = err
		return result
	}
	if render == nil {
		result.err = ErrUnknownFormat
		return result
	}
	result.body, result.err = render(ctx, s.source)
	if nilContent(result.body) {
		result.body = nil
		if result.err == nil {
			result.err = ErrInvalidResult
		}
	}
	return result
}

// Prepare selects the registered renderer and binds its result to this exact document revision.
func (r *Registry) Prepare(ctx context.Context, source SourceSnapshot) Prepared {
	body, err := r.Render(ctx, source.format, source.source)
	return Prepared{source: source, body: body, err: err}
}

// DocumentChange describes one settled document version. Body is a read-only
// passive projection; it may not be mutated by subscribers or retained renderers.
type DocumentChange struct {
	Revision         Revision
	PreparedRevision Revision
	State            DocumentState
	Body             grid.Drawable
	SourceError      error
	PrepareError     error
}

// Publishable reports a final result; publishing Partial still requires an explicit retention policy.
func (c DocumentChange) Publishable() bool {
	return c.State == Final || c.State == Partial
}

// StreamDocument owns source, source revision, prepared content and finalization.
// It starts no goroutine. All methods other than SourceSnapshot.Prepare belong
// to one owner sequence; notifications may read, but cannot mutate the document.
// Construct it with NewStreamDocument; the zero value is not usable.
// A StreamDocument must not be copied after first use.
type StreamDocument struct {
	noCopy           noCopy
	id               *byte
	cfg              StreamConfig
	source           strings.Builder
	revision         Revision
	ended            bool
	sourceErr        error
	preparedRevision Revision
	body             grid.Drawable
	prepareErr       error
	notifying        bool
	listeners        []*documentListener
}

type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

type documentListener struct{ fn func(DocumentChange) }

// NewStreamDocument creates a single-owner bounded source with an explicit format.
func NewStreamDocument(cfg StreamConfig) (*StreamDocument, error) {
	format, err := normalize(cfg.Format)
	if err != nil {
		return nil, err
	}
	if cfg.MaxSourceBytes < 0 {
		return nil, errors.New("content: source byte budget cannot be negative")
	}
	if cfg.MaxSourceBytes == 0 {
		cfg.MaxSourceBytes = 4 << 20
	}
	cfg.Format = format
	return &StreamDocument{id: new(byte), cfg: cfg}, nil
}

func (d *StreamDocument) assertMutable() {
	if d == nil || d.id == nil {
		panic("content: uninitialized stream document")
	}
	if d.notifying {
		panic("content: document mutation during notification")
	}
}

// Append atomically accepts the entire batch or refuses it. Network boundaries
// never finalize syntax and never cause an implicit language downgrade.
func (d *StreamDocument) Append(batch []byte) error {
	d.assertMutable()
	if d.ended {
		return ErrSourceClosed
	}
	if len(batch) == 0 {
		return nil
	}
	if len(batch) > d.cfg.MaxSourceBytes-d.source.Len() {
		return ErrSourceLimit
	}
	if d.revision == Revision(math.MaxUint64) {
		return errors.New("content: revisions exhausted")
	}
	_, _ = d.source.Write(batch)
	d.revision++
	d.notify()
	return nil
}

// End closes the source and creates a new revision, even for unchanged bytes.
// Final parsing may settle an incomplete UTF-8 sequence or unresolved syntax
// differently from a provisional preview. A source error retains useful content.
func (d *StreamDocument) End(cause error) error {
	d.assertMutable()
	if d.ended {
		return ErrSourceClosed
	}
	if d.revision == Revision(math.MaxUint64) {
		return errors.New("content: revisions exhausted")
	}
	d.ended, d.sourceErr = true, cause
	d.revision++
	d.notify()
	return nil
}

// Snapshot clones source into immutable worker input and preserves the originating identity.
func (d *StreamDocument) Snapshot() SourceSnapshot {
	if d == nil {
		return SourceSnapshot{}
	}
	return SourceSnapshot{owner: d.id, revision: d.revision, format: d.cfg.Format, source: strings.Clone(d.source.String()), final: d.ended}
}

// Accept transfers a prepared result only when identity, revision and EOF still match.
func (d *StreamDocument) Accept(prepared Prepared) error {
	d.assertMutable()
	if prepared.source.owner != d.id || prepared.source.revision != d.revision || prepared.source.final != d.ended {
		return ErrStalePreparation
	}
	if d.preparedRevision == d.revision && (d.body != nil || d.prepareErr != nil) {
		return ErrAlreadyPrepared
	}
	d.body, d.prepareErr, d.preparedRevision = prepared.body, prepared.err, d.revision
	d.notify()
	return nil
}

// Current returns the settled source and prepared-result projection without triggering work.
func (d *StreamDocument) Current() DocumentChange {
	if d == nil {
		return DocumentChange{State: Receiving}
	}
	state := Receiving
	switch {
	case d.revision != d.preparedRevision || d.body == nil && d.prepareErr == nil:
		if d.ended {
			state = Finalizing
		} else if d.revision > 0 {
			state = Preparing
		}
	case !d.ended:
		state = Receiving
	case d.body == nil:
		state = Failed
	case d.sourceErr != nil || d.prepareErr != nil:
		state = Partial
	default:
		state = Final
	}
	return DocumentChange{Revision: d.revision, PreparedRevision: d.preparedRevision, State: state, Body: d.body, SourceError: d.sourceErr, PrepareError: d.prepareErr}
}

// Subscribe returns an idempotent owner-side cancellation function. Cancellation
// during notification is allowed and prevents later delivery in that turn.
func (d *StreamDocument) Subscribe(fn func(DocumentChange)) func() {
	d.assertMutable()
	if fn == nil {
		return func() {}
	}
	listener := &documentListener{fn: fn}
	d.listeners = append(d.listeners, listener)
	return func() {
		listener.fn = nil
		if d.notifying {
			return
		}
		for index, entry := range d.listeners {
			if entry == listener {
				copy(d.listeners[index:], d.listeners[index+1:])
				d.listeners[len(d.listeners)-1] = nil
				d.listeners = d.listeners[:len(d.listeners)-1]
				return
			}
		}
	}
}

func (d *StreamDocument) notify() {
	d.notifying = true
	defer func() { d.notifying = false }()
	change := d.Current()
	for _, listener := range d.listeners {
		if listener.fn != nil {
			listener.fn(change)
		}
	}
	active := d.listeners[:0]
	for _, listener := range d.listeners {
		if listener.fn != nil {
			active = append(active, listener)
		}
	}
	clear(d.listeners[len(active):])
	d.listeners = active
}
