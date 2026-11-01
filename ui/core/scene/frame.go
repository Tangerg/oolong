package scene

import (
	"image"
	"time"

	"github.com/Tangerg/oolong/ui/core/grid"
)

// Frame borrows the candidate drawing space. Derived presentation state staged
// here becomes observable only after the entire frame is admitted and committed.
type Frame struct {
	grid.View
	transaction *transaction
	generation  uint64
}

// Sub creates a clipped child frame sharing the same atomic preparation transaction.
func (f Frame) Sub(area image.Rectangle) Frame {
	return Frame{View: f.View.Sub(area), transaction: f.transaction, generation: f.generation}
}

// Subs projects several rectangles through this frame and its transaction.
func (f Frame) Subs(areas []image.Rectangle) []Frame {
	children := make([]Frame, len(areas))
	for i, area := range areas {
		children[i] = f.Sub(area)
	}
	return children
}

// Snapshot is derived geometry, never mutable application state. T must be a
// value or own immutable backing storage: Value does not deep-copy arbitrary T.
// A Snapshot must not be copied after first use.
type Snapshot[T any] struct {
	noCopy   noCopy
	current  T
	pending  T
	staged   *transaction
	revision uint64
}

// Value returns the last admitted immutable projection; it does not prepare or publish anything.
func (s *Snapshot[T]) Value() T { return s.current }

// Revision returns the layout revision that last published this projection.
func (s *Snapshot[T]) Revision() uint64 { return s.revision }

func (s *Snapshot[T]) prepared() any { return s.pending }

// Stage reserves the sole producer of this snapshot in a candidate frame.
// The returned handle can refine that producer's value until Prepare returns.
// It panics outside an active Prepare, during a read-only projection, or when a
// second producer stages the same snapshot.
func (s *Snapshot[T]) Stage(frame Frame, value T) *Staged[T] {
	if err := frame.checkProjection(); err != nil {
		panic(err)
	}
	if s.staged != nil {
		panic("scene: snapshot has multiple candidate producers")
	}
	s.staged, s.pending = frame.transaction, value
	frame.transaction.states = append(frame.transaction.states, s)
	return &Staged[T]{snapshot: s, frame: frame}
}

// Staged is a temporary capability for refining one projection during its active Prepare call.
type Staged[T any] struct {
	snapshot *Snapshot[T]
	frame    Frame
}

func (s *Staged[T]) valid() {
	if s == nil || !s.frame.active() || s.snapshot.staged != s.frame.transaction {
		panic("scene: candidate geometry used after Prepare")
	}
}

// Value reads the candidate. It panics after its preparation borrow has expired.
func (s *Staged[T]) Value() T { s.valid(); return s.snapshot.pending }

// Set refines the candidate. It panics after its preparation borrow has expired
// or while the frame's scene is borrowed for a read-only projection.
func (s *Staged[T]) Set(value T) {
	s.valid()
	if err := s.frame.checkProjection(); err != nil {
		panic(err)
	}
	s.snapshot.pending = value
}

func (s *Snapshot[T]) commit(tx *transaction) {
	if s.staged != tx {
		panic("scene: snapshot transaction mismatch")
	}
	s.current = s.pending
	s.revision = tx.revision
	var zero T
	s.pending, s.staged = zero, nil
}

func (s *Snapshot[T]) abort(tx *transaction) {
	if s.staged != tx {
		return
	}
	var zero T
	s.pending, s.staged = zero, nil
}

type stagedState interface {
	commit(tx *transaction)
	abort(tx *transaction)
	prepared() any
}
type transaction struct {
	root       *Root
	states     []stagedState
	active     bool
	generation uint64
	revision   uint64
}

func (f Frame) active() bool {
	return f.transaction != nil && f.transaction.active && f.generation == f.transaction.generation
}

func (f Frame) checkProjection() error {
	if !f.active() {
		return ErrInvalid
	}
	if f.transaction.root.observing {
		return ErrBusy
	}
	return nil
}

func (t *transaction) commit() {
	for _, state := range t.states {
		state.commit(t)
	}
	clear(t.states)
	t.states = nil
}

func (t *transaction) abort() {
	for _, state := range t.states {
		state.abort(t)
	}
	clear(t.states)
	t.states = nil
	t.active = false
}

type geometry struct {
	hit     bool
	target  Target
	area    image.Rectangle
	clip    image.Rectangle
	content contentVersion
}

type contentVersion struct {
	id       string
	revision uint64
}

func (r *Root) contentVersion(n *node) contentVersion {
	if n.config.Semantics == nil {
		return contentVersion{}
	}
	s := r.project(n.config.Semantics)
	return contentVersion{s.ContentID, s.ContentRevision}
}

type layoutSnapshot struct {
	model       uint64
	revision    uint64
	sequence    uint64
	geometry    []geometry
	projections map[stagedState]any
	writtenAt   time.Time
	epoch       uint64
}

// Prepared owns one complete candidate. Commit publishes only prepared values;
// it performs no user callbacks and cannot reject already-admitted output.
// A Prepared must not be copied after first use.
type Prepared struct {
	noCopy noCopy
	root   *Root
	tx     *transaction
	layout *layoutSnapshot
	done   bool
}

// Commit atomically publishes the candidate after writer admission. Zero inherits the previous writer sequence.
// Wrote then publishes its input geometry, including when that watermark was already confirmed.
// It panics if the candidate has already settled, is foreign, regresses the writer sequence, or settlement interrupts a pure phase.
func (p *Prepared) Commit(sequence uint64) {
	if p == nil || p.done {
		panic("scene: candidate already settled")
	}
	p.requireSettlement()
	r := p.root
	if sequence == 0 {
		sequence = r.lastSequence
	}
	if sequence < r.lastSequence {
		panic("scene: writer sequence moved backwards")
	}
	p.layout.sequence = sequence
	r.lastSequence = sequence
	p.tx.commit()
	r.layout = p.layout
	r.pending = nil
	p.done = true
	if n := len(r.inFlight); n != 0 && r.inFlight[n-1].sequence == sequence {
		r.inFlight[n-1] = p.layout
	} else {
		r.inFlight = append(r.inFlight, p.layout)
	}
	r.signal()
}

// Abort discards an unsettled candidate without undoing model or attachment changes.
// It panics if called on a copy of a live candidate or during a pure projection.
func (p *Prepared) Abort() {
	if p == nil || p.done {
		return
	}
	p.requireSettlement()
	p.tx.abort()
	p.root.pending = nil
	p.done = true
}

// The writer clock may be sampled between preparation and admission. Its pure
// read must not be able to commit or discard the candidate it interrupted.
func (p *Prepared) requireSettlement() {
	if p.root.pending != p {
		panic("scene: candidate is not owned by this root")
	}
	if p.root.preparing || p.root.observing {
		panic(ErrBusy)
	}
}

// Prepare paints a complete candidate without publishing geometry. A panic aborts every staged projection before propagating.
func (r *Root) Prepare(view grid.View) (prepared *Prepared, err error) {
	if err := r.mutable(); err != nil {
		return nil, err
	}
	if len(r.inFlight) >= r.maxPendingFrames {
		return nil, ErrBusy
	}
	r.preparing = true
	candidate := &layoutSnapshot{model: r.modelRevision, revision: next(r.layoutRevision()), epoch: r.geometryEpoch}
	tx := &transaction{root: r, active: true, generation: 1, revision: candidate.revision}
	prepared = &Prepared{root: r, tx: tx, layout: candidate}
	succeeded := false
	defer func() {
		r.preparing = false
		tx.active = false
		if !succeeded {
			tx.abort()
		}
	}()
	frame := Frame{View: view, transaction: tx, generation: tx.generation}
	if r.content.node != nil {
		r.paint(Item{Element: r.content}, frame, image.Point{}, candidate)
	}
	candidate.projections = make(map[stagedState]any, len(tx.states))
	for _, state := range tx.states {
		candidate.projections[state] = state.prepared()
	}
	r.pending = prepared
	succeeded = true
	return prepared, nil
}

func (r *Root) paint(item Item, frame Frame, origin image.Point, into *layoutSnapshot) {
	n := item.Element.node
	placed := geometry{target: n.target(), area: frame.Bounds().Add(origin), clip: frame.Visible().Add(origin), hit: r.enabled(n) && (n.config.Handle != nil || n.config.Preview != nil)}
	placed.content = r.contentVersion(n)
	into.geometry = append(into.geometry, placed)
	paint := item.Paint
	if paint == nil {
		paint = n.config.Paint
	}
	if paint != nil {
		paint(frame)
	}
	if g := n.group; g != nil {
		width, height := frame.Size()
		areas := r.project(func() []image.Rectangle { return g.rectangles(image.Pt(width, height)) })
		for i, item := range g.items {
			r.paint(item, frame.Sub(areas[i]), origin.Add(areas[i].Min), into)
		}
	}
}

// Wrote advances transport completion. It is distinct from logical layout and
// does not claim that a physical terminal has displayed the bytes. It panics during
// paint or observation; report writer progress between these pure phases.
func (r *Root) Wrote(sequence uint64) {
	if r == nil || r.closed || sequence < r.writtenSequence {
		return
	}
	if r.preparing || r.observing {
		panic(ErrBusy)
	}
	covered := 0
	for covered < len(r.inFlight) && r.inFlight[covered].sequence <= sequence {
		covered++
	}
	if covered == 0 && sequence == r.writtenSequence {
		return
	}
	if covered > 0 {
		// One watermark provides one observation time. Intermediate frames
		// crossed together cannot be assigned invented timestamps.
		at := r.readOnly(r.now)
		r.publishWritten(r.inFlight[covered-1], at)
		clear(r.inFlight[:covered])
		r.inFlight = r.inFlight[covered:]
		if len(r.inFlight) == 0 {
			r.inFlight = nil
		}
	}
	r.writtenSequence = sequence
	r.signal()
}

// InvalidateGeometry refuses coordinate input until a complete replacement frame
// is written, for example after a terminal resize or uncertain external output.
// It panics during a pure phase or while a prepared candidate awaits settlement.
func (r *Root) InvalidateGeometry() {
	if r == nil || r.closed {
		return
	}
	if err := r.mutable(); err != nil {
		panic(err)
	}
	r.written = nil
	r.inFlight = nil
	r.geometryEpoch = next(r.geometryEpoch)
	r.published.Store(nil)
	previous := r.capture
	r.capture.revoked = true
	r.signal()
	if n := r.resolve(previous.target); n != nil && !previous.revoked {
		previous.cancel(n)
	}
}

type writtenHistory struct{ layouts []*layoutSnapshot }

func (r *Root) publishWritten(snapshot *layoutSnapshot, at time.Time) {
	snapshot.writtenAt = at
	r.written = snapshot
	history := r.published.Load()
	var layouts []*layoutSnapshot
	if history != nil {
		start := max(0, len(history.layouts)-r.inputHistory+1)
		layouts = append(layouts, history.layouts[start:]...)
	}
	layouts = append(layouts, snapshot)
	r.published.Store(&writtenHistory{layouts: layouts})
}

func (r *Root) layoutRevision() uint64 {
	if r.layout == nil {
		return 0
	}
	return r.layout.revision
}
