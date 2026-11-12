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
	lease *paintLease
}

// Sub creates a clipped child frame sharing the same atomic preparation transaction.
func (f Frame) Sub(area image.Rectangle) Frame {
	return Frame{View: f.View.Sub(area), lease: f.lease}
}

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
// The returned handle can refine that producer's value until its paint callback returns.
// It panics outside an active Prepare, during a read-only projection, or when a
// second producer stages the same snapshot.
func (s *Snapshot[T]) Stage(frame Frame, value T) *Staged[T] {
	if err := frame.checkProjection(); err != nil {
		panic(err)
	}
	if s.staged != nil {
		panic("scene: snapshot has multiple candidate producers")
	}
	s.staged, s.pending = frame.lease.tx, value
	frame.lease.tx.states = append(frame.lease.tx.states, s)
	return &Staged[T]{snapshot: s, lease: frame.lease}
}

// Staged is a temporary capability for refining one projection during its producing paint callback.
type Staged[T any] struct {
	snapshot *Snapshot[T]
	lease    *paintLease
}

func (s *Staged[T]) valid(write bool) {
	if s == nil || s.lease == nil || s.snapshot == nil || s.snapshot.staged != s.lease.tx {
		panic(ErrInvalid)
	}
	if err := s.lease.check(write); err != nil {
		panic(err)
	}
}

// Value reads the candidate. It panics after its painter returns.
func (s *Staged[T]) Value() T { s.valid(false); return s.snapshot.pending }

// Set refines the candidate. It panics outside the issuing paint callback or
// while that callback has entered a read-only projection.
func (s *Staged[T]) Set(value T) { s.valid(true); s.snapshot.pending = value }

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
	root     *Root
	states   []stagedState
	painter  *paintLease
	revision uint64
}

// Cells and staged geometry carry the same borrow; neither can be changed by a
// later painter or by a nested measurement using a saved handle.
type paintLease struct{ tx *transaction }

func (p *paintLease) check(write bool) error {
	if p == nil || p.tx.root == nil {
		return ErrInvalid
	}
	if write && p.tx.root.observing {
		return ErrBusy
	}
	if p.tx.painter != p {
		return ErrInvalid
	}
	return nil
}

func (p *paintLease) writable() error { return p.check(true) }

func (f Frame) checkProjection() error { return f.lease.writable() }

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
	byNode      map[NodeID]int
	projections map[stagedState]any
	admittedAt  time.Time
	hasOutput   bool
	epoch       *byte
}

// The index addresses paint order in this frame. Remounting can reuse a node ID,
// so only the full attachment stored at that position can authorize a hit.
func (s *layoutSnapshot) lookup(target Target) (geometry, bool) {
	if s != nil {
		if i, ok := s.byNode[target.node]; ok && s.geometry[i].target == target {
			return s.geometry[i], true
		}
	}
	return geometry{}, false
}

// Prepared owns one complete candidate. Commit publishes only prepared values;
// it performs no user callbacks and cannot reject already-admitted output.
// A Prepared must not be copied after first use.
type Prepared struct {
	noCopy noCopy
	root   *Root
	tx     *transaction
	layout *layoutSnapshot
}

// Commit atomically publishes the candidate after writer admission. Zero inherits
// the previous writer sequence without creating transport completion. Physical
// input still needs the writer's evidence; Wrote updates owner-side progress.
// Completion already reported through Wrote also applies to this admission.
// It panics if the candidate has already settled, is foreign, regresses the writer sequence, or settlement interrupts a pure phase.
func (p *Prepared) Commit(sequence uint64) {
	if p == nil || p.root == nil {
		panic("scene: candidate already settled")
	}
	p.requireSettlement()
	r := p.root
	p.layout.hasOutput = sequence != 0
	committed := r.committedSequence()
	if sequence == 0 {
		sequence = committed
	}
	if sequence < committed {
		panic("scene: writer sequence moved backwards")
	}
	p.layout.sequence = sequence
	p.tx.commit()
	r.layout = p.layout
	r.frames.admit(p.layout)
	p.layout.admittedAt = time.Now()
	r.publishGeometry(p.layout)
	p.settle()
	r.signal()
}

// Abort discards an unsettled candidate without undoing model or attachment changes.
// It panics if called on a copy of a live candidate or during a pure projection.
func (p *Prepared) Abort() {
	if p == nil || p.root == nil {
		return
	}
	p.requireSettlement()
	p.tx.abort()
	p.settle()
}

// A settled handle owns no frame. Retaining it must not pin obsolete projections
// or the live scene, and losing the owner is itself the terminal state.
func (p *Prepared) settle() {
	p.root.pending = nil
	if history := p.root.published.Load(); history != nil && history.pending {
		p.root.published.Store(&inputGeometry{layouts: history.layouts})
	}
	p.root, p.tx, p.layout = nil, nil, nil
}

// A projection borrows candidate storage; settlement cannot revoke or publish
// that storage from inside the read that currently owns the borrow.
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
	if len(r.frames.inFlight) >= r.maxPendingFrames {
		return nil, ErrBusy
	}
	candidate := &layoutSnapshot{
		model:    r.modelRevision,
		revision: next(r.layoutRevision()),
		epoch:    r.geometryEpoch,
		geometry: make([]geometry, 0, len(r.nodes)),
		byNode:   make(map[NodeID]int, len(r.nodes)),
	}
	tx := &transaction{root: r, revision: candidate.revision}
	prepared = &Prepared{root: r, tx: tx, layout: candidate}
	succeeded := false
	defer func() {
		r.preparing = false
		tx.painter = nil
		tx.root = nil
		if !succeeded {
			tx.abort()
		}
	}()
	r.preparing = true
	if r.content.node != nil {
		r.paint(Item{Element: r.content}, view, tx, image.Point{}, candidate)
	}
	candidate.projections = make(map[stagedState]any, len(tx.states))
	for _, state := range tx.states {
		candidate.projections[state] = state.prepared()
	}
	r.pending = prepared
	history := r.published.Load()
	if history == nil {
		r.published.Store(&inputGeometry{pending: true})
	} else {
		r.published.Store(&inputGeometry{layouts: history.layouts, pending: true})
	}
	succeeded = true
	return prepared, nil
}

func (r *Root) paint(item Item, view grid.View, tx *transaction, origin image.Point, into *layoutSnapshot) {
	n := item.Element.node
	placed := geometry{
		target:  n.target(),
		area:    view.Bounds().Add(origin),
		clip:    view.Visible().Add(origin),
		hit:     r.enabled(n) && (n.config.Handle != nil || n.config.Preview != nil),
		content: r.contentVersion(n),
	}
	into.byNode[placed.target.node] = len(into.geometry)
	into.geometry = append(into.geometry, placed)
	paint := item.Paint
	if paint == nil {
		paint = n.config.Paint
	}
	if paint != nil {
		lease := &paintLease{tx: tx}
		tx.painter = lease
		paint(Frame{View: view.RestrictWrites(lease.writable), lease: lease})
		tx.painter = nil
	}
	if g := n.group; g != nil {
		width, height := view.Size()
		areas := r.project(func() []image.Rectangle { return g.rectangles(image.Pt(width, height)) })
		for i, item := range g.items {
			r.paint(item, view.Sub(areas[i]), tx, origin.Add(areas[i].Min), into)
		}
	}
}

// Wrote advances transport completion. It is distinct from logical layout and
// does not claim that a physical terminal has displayed the bytes. It panics during
// paint or observation; report writer progress between these pure phases.
func (r *Root) Wrote(sequence uint64) {
	if r == nil || r.closed {
		return
	}
	if r.projecting() {
		panic(ErrBusy)
	}
	if r.frames.wrote(sequence) {
		r.signal()
	}
}

// committedSequence is the writer sequence of the newest committed frame, which
// a later commit may not precede.
func (r *Root) committedSequence() uint64 {
	if r.layout == nil {
		return 0
	}
	return r.layout.sequence
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
	r.frames.revoke()
	r.geometryEpoch = new(byte)
	r.published.Store(nil)
	if n := r.resolve(r.hover); n != nil {
		r.revokeHover(n)
	}
	previous := r.capture
	r.capture.phase = captureRevoked
	r.signal()
	if n := r.resolve(previous.target); n != nil && previous.phase != captureRevoked {
		previous.cancel(n)
	}
}

// A candidate may reach the writer before Commit returns. Readers must not
// authorize an older map while that admission's geometry is still unsettled.
type inputGeometry struct {
	layouts []*layoutSnapshot
	pending bool
}

func (r *Root) publishGeometry(snapshot *layoutSnapshot) {
	history := r.published.Load()
	var layouts []*layoutSnapshot
	if history != nil {
		start := max(0, len(history.layouts)-r.inputHistory+1)
		layouts = append(layouts, history.layouts[start:]...)
	}
	layouts = append(layouts, snapshot)
	r.published.Store(&inputGeometry{layouts: layouts})
}

// current reports whether snapshot is admitted geometry that no invalidation has
// revoked. It is the one rule every coordinate reader applies.
func (r *Root) current(snapshot *layoutSnapshot) bool {
	return snapshot != nil && snapshot.epoch == r.geometryEpoch
}

func (r *Root) layoutRevision() uint64 {
	if r.layout == nil {
		return 0
	}
	return r.layout.revision
}
