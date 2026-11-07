package base

import (
	"errors"
	"image"
	"math"
	"sort"

	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/internal/fifo"
	"github.com/Tangerg/oolong/ui/internal/identity"
)

// ErrTranscriptCapacity refuses another retained reference without discarding existing content.
var ErrTranscriptCapacity = errors.New("base: transcript item budget exhausted")

// BlockID is a stable, never-reused identity within one transcript; zero identifies no item.
type BlockID uint64

// Retention determines whether finalized content stays interactive or transfers to a publisher.
type Retention uint8

// Retention policies require an explicit publisher before permanent output is enabled.
const (
	Retain Retention = iota
	PublishCompleted
)

// ContentState describes a transcript reference, independently of source and writer progress.
type ContentState uint8

// Content reference states distinguish revision, retention and irreversible responsibility transfer.
const (
	ContentLive ContentState = iota
	ContentRetained
	ContentRetired
)

// TranscriptPublisher admits complete content and notifies the UI owner when progress
// may allow a refused prefix to resume. Only successful admission transfers the content.
// Subscribe returns an idempotent cancellation and does not call fn during registration.
type TranscriptPublisher interface {
	TryPublish(body grid.Drawable) (publication.Receipt, error)
	Subscribe(fn func()) func()
}

const defaultTranscriptItems = 64

// TranscriptConfig combines reading policy, bounded retention and an optional publication owner.
type TranscriptConfig struct {
	Key       string
	Name      string
	Publisher TranscriptPublisher
	Retention Retention
	// PublishPartial permits terminal failures too. A failed document with no
	// body transfers an empty capture while its item retains the failure cause.
	PublishPartial bool
	// KeepCompleted retains this many trailing items of the complete leading run.
	// It must leave at least one reference available within MaxItems.
	KeepCompleted int
	MaxItems      int
	Keys          *keymap.Map
}

// TranscriptItem is a read-only receipt for a content reference. Retirement clears
// its document and body while preserving identity and the publication receipt.
// Retaining this handle never retains the completed conversation's source bytes.
type TranscriptItem struct {
	id       BlockID
	observed content.Revision // last preparation announced to the selection owner
	body     Block
	document *content.StreamDocument
	cancel   func()
	receipt  publication.Receipt
	err      error
}

// ID survives retirement without keeping the document or body alive.
func (i *TranscriptItem) ID() BlockID {
	if i == nil {
		return 0
	}
	return i.id
}

// State reports whether this reference can still revise or participate in the live transcript.
func (i *TranscriptItem) State() ContentState {
	if i == nil || i.document == nil && i.body == nil {
		return ContentRetired
	}
	state := i.content()
	if state.Publishable() || state.State == content.Failed {
		return ContentRetained
	}
	return ContentLive
}

// Receipt reports permanent-output progress after admission; before admission its zero value is invalid.
func (i *TranscriptItem) Receipt() publication.Receipt {
	if i == nil {
		return publication.Receipt{}
	}
	return i.receipt
}

// Err preserves source, preparation and publication refusal causes while content remains readable.
func (i *TranscriptItem) Err() error {
	if i == nil {
		return nil
	}
	if i.document == nil {
		return i.err
	}
	state := i.document.Current()
	return errors.Join(state.SourceError, state.PrepareError, i.err)
}

func (i *TranscriptItem) content() content.DocumentChange {
	if i.document != nil {
		return i.document.Current()
	}
	return content.DocumentChange{Body: i.body, PreparedRevision: 1, State: content.Final}
}

func (i *TranscriptItem) retire() {
	i.err = i.Err()
	if i.cancel != nil {
		i.cancel()
		i.cancel = nil
	}
	i.body, i.document = nil, nil
}

// Transcript owns retained references and reading behavior. Content semantics
// stay in StreamDocument; permanent-output responsibility moves only after the
// publisher accepts an immutable capture. Layout is a scene-owned projection.
// Commands belong to the UI owner and reject mutation during scene presentation
// or content capture. Revision and row counters panic rather than wrap identities.
// A Transcript must not be copied after first use.
type Transcript struct {
	namespace       *byte // Does not retain content when an endpoint outlives the transcript.
	search          transcriptSearch
	noCopy          noCopy
	cfg             TranscriptConfig
	initialized     bool
	closed          bool
	entries         fifo.Queue[*TranscriptItem]
	next            BlockID
	start           int
	revision        uint64
	stopPublication func()
	element         scene.Element
	scroll          Scroll
	selection       Selection
	sticky          Sticky
	geometry        scene.Snapshot[TranscriptLayout]
	presentation    scene.Snapshot[transcriptPresentation]
	clicked         uint64
}

// NewTranscript validates retention dependencies and creates one stable scene element.
func NewTranscript(cfg TranscriptConfig) (*Transcript, error) {
	if cfg.MaxItems < 0 || cfg.KeepCompleted < 0 || cfg.Retention > PublishCompleted {
		return nil, errors.New("base: invalid transcript configuration")
	}
	if identity.IsNil(cfg.Publisher) {
		if cfg.Retention == PublishCompleted {
			return nil, errors.New("base: publishing transcript requires a publisher")
		}
		cfg.Publisher = nil
	}
	maxItems := cfg.MaxItems
	if maxItems == 0 {
		maxItems = defaultTranscriptItems
	}
	if cfg.KeepCompleted >= maxItems || cfg.KeepCompleted > 0 && cfg.Retention != PublishCompleted {
		return nil, errors.New("base: recent completion retention requires publication and space for new content")
	}
	t := &Transcript{cfg: cfg}
	t.initialize()
	return t, nil
}

func (t *Transcript) initialize() {
	if t.initialized {
		return
	}
	t.initialized = true
	t.namespace = new(byte)
	if t.cfg.MaxItems == 0 {
		t.cfg.MaxItems = defaultTranscriptItems
	}
	t.next = 1
	t.scroll.ToBottom()
	t.element = scene.NewElement(scene.ElementConfig{
		Key:       t.cfg.Key,
		Enabled:   func() bool { return !t.closed },
		Focusable: func() bool { return true },
		Semantics: func() scene.Semantic {
			availability := scene.ContentLive
			if t.closed {
				availability = scene.ContentRetired
			}
			return scene.Semantic{Role: scene.RoleDocument, Name: t.cfg.Name, ContentRevision: t.revision, Availability: availability}
		},
		Handle:  t.handle,
		Actions: []scene.ActionBinding{t.scroll.action("Scroll document", nil)},
		Paint: func(frame scene.Frame) {
			view := t.StageWindow(frame)
			view.Content.Draw(frame.Sub(view.Header).View, view.HeaderFrom)
			view.Content.Draw(view.Body, view.BodyFrom)
		},
	})
	t.scroll.element = t.element
	t.selection.element = t.element
	t.sticky.element = t.element
	if t.cfg.Publisher != nil {
		t.stopPublication = t.cfg.Publisher.Subscribe(func() { t.publish() })
	}
}

func (t *Transcript) assertMutable() {
	t.initialize()
	if err := t.element.CheckMutation(); err != nil {
		panic(err)
	}
	if t.closed {
		panic("base: transcript is closed")
	}
}

func (t *Transcript) changed() {
	if t.revision == math.MaxUint64 {
		panic("base: transcript revisions exhausted")
	}
	t.revision++
	t.search = transcriptSearch{}
	t.element.Invalidate()
}

func (t *Transcript) Element() scene.Element { t.initialize(); return t.element }

// Scroll returns the owned reading-intent controller, not a copy of its current projection.
func (t *Transcript) Scroll() *Scroll { t.initialize(); return &t.scroll }

// Selection returns the owned logical selection shared by input, copying and appearance.
func (t *Transcript) Selection() *Selection { return &t.selection }

// Sticky returns the owned set of pinnable content identities.
func (t *Transcript) Sticky() *Sticky { return &t.sticky }

func (t *Transcript) Len() int { return t.entries.Len() }

// ContentRevision is the content identity used by scene semantics and derived
// projections. Width changes never advance it.
func (t *Transcript) ContentRevision() uint64 { return t.revision }

// Width is the last committed layout width; preparing or aborting a frame does not advance it.
func (t *Transcript) Width() int { return t.geometry.Value().Width() }

// Height is the retained row count in the last committed layout.
func (t *Transcript) Height() int { return t.geometry.Value().Height() }

// StartRow is the retained row origin in the last committed layout.
func (t *Transcript) StartRow() int { return t.geometry.Value().StartRow() }

// EndRow is the exclusive end of the last committed row space.
func (t *Transcript) EndRow() int { return t.geometry.Value().EndRow() }

// FirstBlock is the first retained identity, or the next identity when the transcript is empty.
func (t *Transcript) FirstBlock() BlockID {
	if t.entries.Len() > 0 {
		return t.entries.At(0).id
	}
	if t.next == 0 {
		return 1
	}
	return t.next
}

func (t *Transcript) add(body Block, document *content.StreamDocument) (*TranscriptItem, error) {
	t.assertMutable()
	if t.entries.Len() >= t.cfg.MaxItems {
		return nil, ErrTranscriptCapacity
	}
	if t.next == BlockID(math.MaxUint64) {
		return nil, errors.New("base: transcript identities exhausted")
	}
	item := &TranscriptItem{id: t.next, body: body, document: document}
	if document != nil {
		item.observed = document.Current().PreparedRevision
		item.cancel = document.Subscribe(func(change content.DocumentChange) { t.accept(item, change) })
	}
	t.next++
	t.entries.Push(item)
	t.changed()
	t.publish()
	return item, nil
}

// Add takes immutable passive content that is already final. Mutable sources use
// AddDocument so revision and completion cannot be maintained by a second caller.
func (t *Transcript) Add(body Block) (*TranscriptItem, error) {
	if grid.IsNilDrawable(body) {
		return nil, errors.New("base: transcript content is nil")
	}
	return t.add(body, nil)
}

// AddDocument subscribes once to source revisions, final preparation and automatic retention transitions.
func (t *Transcript) AddDocument(document *content.StreamDocument) (*TranscriptItem, error) {
	if document == nil {
		return nil, errors.New("base: transcript document is nil")
	}
	return t.add(nil, document)
}

func (t *Transcript) accept(item *TranscriptItem, change content.DocumentChange) {
	t.assertMutable()
	if item.observed != change.PreparedRevision {
		t.selection.contentChanged(item.id, uint64(change.PreparedRevision))
		item.observed = change.PreparedRevision
	}
	t.changed()
	t.publish()
}

func (t *Transcript) publish() {
	if t.closed || t.cfg.Retention != PublishCompleted || t.cfg.Publisher == nil {
		return
	}
	t.assertMutable()
	completed := 0
	for item := range t.entries.Values() {
		change := item.content()
		complete := change.State == content.Final || t.cfg.PublishPartial && (change.State == content.Partial || change.State == content.Failed)
		if !complete {
			break
		}
		completed++
	}
	for range max(completed-t.cfg.KeepCompleted, 0) {
		item := t.entries.At(0)
		_, rows, _ := t.geometry.Value().Extent(item.id)
		if rows > math.MaxInt-t.start {
			panic("base: transcript row origin exhausted")
		}
		var receipt publication.Receipt
		var err error
		t.element.ReadOnly(func() { receipt, err = t.cfg.Publisher.TryPublish(item.content().Body) })
		if err != nil {
			if !errors.Is(err, publication.ErrCapacity) && !identity.Same(item.err, err) {
				item.err = err
				t.element.Invalidate()
			}
			return
		}
		item.receipt = receipt
		t.start += rows
		t.scroll.discard(rows)
		t.selection.discardBefore(item.id + 1)
		item.err = nil
		item.retire()
		t.entries.Discard(1)
		t.sticky.discardBefore(t.FirstBlock())
		t.changed()
	}
}

// Close disconnects subscriptions and releases retained references; it does not cancel accepted publications.
// It panics if called during content capture, scene presentation or an uncommitted scene frame.
func (t *Transcript) Close() {
	if t.closed {
		return
	}
	t.initialize()
	if err := t.element.CheckMutation(); err != nil {
		panic(err)
	}
	t.closed = true
	if t.stopPublication != nil {
		defer t.stopPublication()
		t.stopPublication = nil
	}
	for item := range t.entries.Values() {
		item.retire()
	}
	t.entries.Clear()
	t.selection.clear()
	t.sticky.setBlocks(nil)
	// Close must release its projections even if no later frame is painted.
	t.geometry = scene.Snapshot[TranscriptLayout]{}
	t.presentation = scene.Snapshot[transcriptPresentation]{}
	t.changed()
}

// Block resolves a currently retained identity, returning nil after retirement.
func (t *Transcript) Block(id BlockID) Block {
	for item := range t.entries.Values() {
		if item.id == id {
			return item.content().Body
		}
	}
	return nil
}

// Last returns the final retained passive result, which can be nil before its first preparation.
func (t *Transcript) Last() Block {
	if t.entries.Len() == 0 {
		return nil
	}
	return t.entries.At(t.entries.Len() - 1).content().Body
}

// Stage measures into immutable candidate storage. It never updates content,
// scroll intention or subscriptions and a failed outer frame discards the layout.
// It panics if the cumulative row coordinates overflow int, or the frame is expired
// or has already staged this transcript projection. Its Element must be mounted in
// the frame's scene, outside a read-only projection; violations panic before measurement.
func (t *Transcript) Stage(frame scene.Frame, width int) TranscriptLayout {
	requireProjection(t.element, frame)
	width = max(width, 0)
	key := transcriptLayoutKey{owner: t.namespace, revision: t.revision, width: width}
	if previous := t.geometry.Value(); previous.key == key {
		// Only an admitted immutable projection can be reused; an aborted frame
		// must not replace the geometry or retain a second cache of its content.
		t.geometry.Stage(frame, previous)
		return previous
	}
	state := TranscriptLayout{key: key, start: t.start, first: t.FirstBlock()}
	top := t.start
	for item := range t.entries.Values() {
		change := item.content()
		rows := 0
		if change.Body != nil && width > 0 {
			rows = max(change.Body.HeightForWidth(width), 0)
		}
		if rows > math.MaxInt-top {
			panic("base: transcript row coordinates exhausted")
		}
		var mapping *transcriptTextMap
		if projector, ok := change.Body.(TextProjector); ok && rows > 0 {
			projected := projector.Rows(width)
			mapping = newTranscriptTextMap(projected[:min(rows, len(projected))])
		}
		state.blocks = append(state.blocks, placed{id: item.id, revision: uint64(change.PreparedRevision), mapping: mapping, block: change.Body, top: top, height: rows})
		top += rows
	}
	state.rows = top - state.start
	t.geometry.Stage(frame, state)
	return state
}

type placed struct {
	revision    uint64
	mapping     *transcriptTextMap
	id          BlockID
	block       Block
	height, top int
}

// TranscriptLayout is immutable geometry and text mapping for one complete scene candidate.
type TranscriptLayout struct {
	key         transcriptLayoutKey
	blocks      []placed
	rows, start int
	first       BlockID
}

type transcriptLayoutKey struct {
	owner    *byte
	revision uint64
	width    int
}

// Width is the column count used by every placement in this projection.
func (l TranscriptLayout) Width() int { return l.key.width }

// Height is the total retained row span in this projection.
func (l TranscriptLayout) Height() int { return l.rows }

// StartRow is the inclusive origin shared by drawing, selection and hit testing.
func (l TranscriptLayout) StartRow() int { return l.start }

// EndRow is the exclusive end of the checked row span.
func (l TranscriptLayout) EndRow() int { return l.start + l.rows }

// FirstBlock identifies the first retained content at preparation time.
func (l TranscriptLayout) FirstBlock() BlockID { return l.first }

// Block resolves the passive content held by this immutable layout, including earlier committed frames.
func (l TranscriptLayout) Block(id BlockID) Block {
	b, _ := l.placed(id)
	return b.block
}

// Extent reads the placement in this immutable frame.
func (l TranscriptLayout) Extent(id BlockID) (top, height int, ok bool) {
	b, ok := l.placed(id)
	return b.top, b.height, ok
}

func (l TranscriptLayout) window(from, count int) (int, int, bool) {
	if count <= 0 || l.rows == 0 {
		return 0, 0, false
	}
	end := min(layout.Translate(from, count), l.EndRow())
	from = max(from, l.start)
	return from, max(end-from, 0), end > from
}

func (l TranscriptLayout) visible(from, rows int) (first, last int) {
	from, rows, ok := l.window(from, rows)
	if !ok {
		return len(l.blocks), len(l.blocks)
	}
	first = sort.Search(len(l.blocks), func(i int) bool { return l.blocks[i].top+l.blocks[i].height > from })
	last = first
	end := from + rows
	for last < len(l.blocks) && l.blocks[last].top < end {
		last++
	}
	return first, last
}

// At resolves a row to its content identity and local row offset, skipping empty placements.
func (l TranscriptLayout) At(row int) (BlockID, int, bool) {
	first, last := l.visible(row, 1)
	if first == last {
		return 0, 0, false
	}
	b := l.blocks[first]
	return b.id, row - b.top, true
}

func (l TranscriptLayout) Draw(view grid.View, from int) {
	width, _ := view.Size()
	visible := view.Visible()
	if visible.Empty() {
		return
	}
	first, last := l.visible(layout.Translate(from, visible.Min.Y), visible.Dy())
	for _, b := range l.blocks[first:last] {
		if b.block != nil && b.height > 0 {
			b.block.Draw(view.Sub(grid.Area(0, b.top-from, width, b.height)))
		}
	}
}

// TextProjector supplies safe rendered rows and soft-wrap metadata; it does not promise a format-source map.
type TextProjector interface{ Rows(width int) []text.Row }

// Rows returns a bounded physical row window, preserving empty slots for content without text.
func (l TranscriptLayout) Rows(from, count int) []text.Row {
	from, count, ok := l.window(from, count)
	if !ok {
		return nil
	}
	out := make([]text.Row, count)
	first, last := l.visible(from, count)
	for _, b := range l.blocks[first:last] {
		if b.mapping == nil {
			continue
		}
		source := min(max(from-b.top, 0), len(b.mapping.rows))
		destination := max(b.top-from, 0)
		copy(out[destination:], b.mapping.rows[source:])
	}
	return out
}

// Extent reads the last committed placement for the requested content identity.
func (t *Transcript) Extent(id BlockID) (int, int, bool) { return t.geometry.Value().Extent(id) }

// At resolves a row against the last committed complete layout.
func (t *Transcript) At(row int) (BlockID, int, bool) { return t.geometry.Value().At(row) }

// Visible returns the half-open identity span touched by a committed row window.
func (t *Transcript) Visible(from, rows int) (BlockID, BlockID) {
	l := t.geometry.Value()
	a, b := l.visible(from, rows)
	first := l.first + BlockID(len(l.blocks))
	if a < len(l.blocks) {
		first = l.blocks[a].id
	}
	last := first
	if b > a {
		last = l.blocks[b-1].id + 1
	}
	return first, last
}

// Rows projects text from the last committed row space without advancing document state.
func (t *Transcript) Rows(from, count int) []text.Row { return t.geometry.Value().Rows(from, count) }

// TranscriptView is a single candidate shared by paint and pointer projection.
// The borrowed Body remains writable only during its scene Prepare call.
type TranscriptView struct {
	Content TranscriptLayout
	Body    grid.View
	// BodyFrom is an absolute content row aligned with Body's first local row.
	// The Scroll offset instead addresses the full viewport, including its overlay.
	BodyFrom   int
	Header     image.Rectangle
	HeaderFrom int
	Pinned     Pinned
}

// Sticky furniture covers part of the full scroll viewport; it never changes the
// scroll range or page size. BodyFrom skips exactly the rows that the overlay hides.
type transcriptViewport struct {
	viewportFrom int
	pinned       Pinned
	headerFrom   int
}

func (t *Transcript) viewport(content TranscriptLayout, offset, height int) transcriptViewport {
	window := transcriptViewport{viewportFrom: content.StartRow() + offset}
	pinned, ok := t.sticky.At(content, window.viewportFrom, height)
	if !ok || pinned.Rows >= height {
		return window
	}
	window.pinned = pinned
	if top, _, exists := content.Extent(pinned.Block); exists {
		window.headerFrom = top + pinned.ClipTop
	}
	return window
}

func (v transcriptViewport) bodyFrom() int { return layout.Sum(v.viewportFrom, v.pinned.Rows) }

// Body starts stay ordered as the viewport advances, including collapsed and
// pushed headers. Their inverse gives reveal and reflow the latest offset whose
// visible body has not passed the requested row, without duplicating Sticky's
// geometry policy. Scroll remains the sole position owner.
func (t *Transcript) offsetForBodyRow(content TranscriptLayout, relativeRow, height int) int {
	row := layout.Sum(content.StartRow(), max(relativeRow, 0))
	low, high := 0, min(max(relativeRow, 0), layout.Remaining(content.Height(), height))
	for low < high {
		middle := high - (high-low)/2
		if t.viewport(content, middle, height).bodyFrom() <= row {
			low = middle
		} else {
			high = middle - 1
		}
	}
	return low
}

// StageWindow composes layout, reading anchor, sticky header and pointer mapping into one frame candidate.
// It has the same frame-owner and panic contract as Stage.
func (t *Transcript) StageWindow(frame scene.Frame) TranscriptView {
	width, height := frame.Size()
	state := t.Stage(frame, width)
	scroll := t.scroll.Stage(frame, state.Height(), height)
	if reveal := scroll.reveal; reveal != nil {
		first, last := min(reveal.first, reveal.last), max(reveal.first, reveal.last)
		scroll.Reveal(t.offsetForBodyRow(state, first, height), last)
	}
	if old := t.geometry.Value(); old.Width() != width {
		if anchor, ok := t.ReadingAnchor(); ok && !anchor.FollowingEnd {
			if at, valid := state.Caret(anchor.Point); valid {
				relativeRow := at.Row - state.StartRow()
				scroll.Anchor(relativeRow - anchor.ViewportOffset)
				// Preserve an attainable screen offset during push-off. Reflowed
				// furniture otherwise needs the new visible-body boundary.
				if t.viewport(state, scroll.Offset(), height).bodyFrom() != at.Row {
					scroll.Anchor(t.offsetForBodyRow(state, relativeRow, height))
				}
			}
		}
	}
	window := t.viewport(state, scroll.Offset(), height)
	body := grid.Area(0, window.pinned.Rows, width, layout.Remaining(height, window.pinned.Rows))
	view := TranscriptView{
		Content: state, Body: frame.Sub(body).View, BodyFrom: window.bodyFrom(),
		Pinned: window.pinned, HeaderFrom: window.headerFrom,
	}
	if window.pinned.Visible() > 0 {
		view.Header = grid.Area(0, 0, width, window.pinned.Visible())
	}
	t.presentation.Stage(frame, transcriptPresentation{layout: state, revision: t.revision, visible: frame.Visible(), body: body, header: view.Header, bodyFrom: view.BodyFrom, headerFrom: view.HeaderFrom})
	return view
}

func (t *Transcript) handle(event scene.Event) scene.EventResult {
	if t.closed {
		return scene.EventResult{}
	}
	presented, valid := event.Read(&t.presentation)
	consumed := t.handleInput(event.Input(), presented, valid)
	mouse, ok := event.Input().(input.Mouse)
	return scene.EventResult{Consumed: consumed, Capture: consumed && ok && mouse.Action == input.MouseDown && mouse.Button == input.ButtonLeft, Focus: consumed && ok && mouse.Action == input.MouseDown}
}

// Handle changes reading intent through the same owner used by scene actions.
// It panics if the transcript is closed or its scene forbids model mutation.
func (t *Transcript) Handle(event input.Event) bool {
	return t.handleInput(event, t.presentation.Value(), true)
}

func (t *Transcript) handleInput(event input.Event, presented transcriptPresentation, valid bool) bool {
	t.assertMutable()
	if t.scroll.Handle(event, t.cfg.Keys) {
		return true
	}
	mouse, ok := event.(input.Mouse)
	if !ok {
		return false
	}
	switch mouse.Action {
	case input.MouseCancel:
		t.clicked = 0
		t.selection.Done()
		return false
	case input.MouseUp:
		if mouse.Releases(input.ButtonLeft) {
			was := t.selection.Dragging()
			t.selection.Done()
			return was
		}
		return false
	case input.MouseDown:
		if !valid {
			return false
		}
		return t.press(mouse, presented)
	case input.MouseDrag:
		if !valid {
			return false
		}
		return t.drag(mouse, presented)
	default:
		return false
	}
}

func (t *Transcript) press(mouse input.Mouse, p transcriptPresentation) bool {
	t.selection.Done()
	if mouse.Button != input.ButtonLeft {
		return false
	}
	if p.revision != t.revision {
		return false
	}
	at, ok := p.pointAt(mouse.Pos, false)
	if !ok {
		return false
	}
	if t.clicked != p.revision {
		t.selection.clicks.Reset()
		t.clicked = p.revision
	}
	switch t.selection.clicks.Press(mouse) {
	case 2:
		if t.selection.SelectWord(p.layout, at) {
			return true
		}
	case 3:
		if t.selection.SelectLine(p.layout, at) {
			return true
		}
	}
	selected, ok := p.layout.hit(at, true)
	if !ok {
		return false
	}
	if t.selection.setRange(selected, true) {
		t.element.Invalidate()
	}
	return true
}

func (t *Transcript) drag(mouse input.Mouse, p transcriptPresentation) bool {
	if !t.selection.Dragging() || !mouse.Drags(input.ButtonLeft) {
		return false
	}
	if p.revision != t.revision {
		return false
	}
	at, ok := p.pointAt(mouse.Pos, true)
	if !ok {
		return false
	}
	selected, ok := p.layout.hit(at, true)
	if !ok {
		return false
	}
	if selected.Start.Before(t.selection.Anchor()) {
		t.selection.Extend(selected.Start)
	} else {
		t.selection.Extend(selected.End)
	}
	return true
}

type transcriptPresentation struct {
	layout                TranscriptLayout
	revision              uint64
	body, header, visible image.Rectangle
	bodyFrom, headerFrom  int
}

func (p transcriptPresentation) pointAt(point image.Point, nearest bool) (Point, bool) {
	if p.visible.Empty() {
		return Point{}, false
	}
	if nearest {
		point.X = min(max(point.X, p.visible.Min.X), p.visible.Max.X-1)
		point.Y = min(max(point.Y, p.visible.Min.Y), p.visible.Max.Y-1)
	} else if !point.In(p.visible) {
		return Point{}, false
	}
	area, row, ok := p.rowAt(point)
	if !ok && nearest {
		area, row, ok = p.nearestRow(point.Y)
	}
	if !ok || area.Empty() {
		return Point{}, false
	}
	col := point.X - area.Min.X
	if nearest {
		col = min(max(col, 0), area.Dx()-1)
	} else if col < 0 || col >= area.Dx() {
		return Point{}, false
	}
	return Point{Row: row, Col: col}, true
}

func (p transcriptPresentation) rowAt(point image.Point) (image.Rectangle, int, bool) {
	switch {
	case point.In(p.header):
		return p.header, p.headerFrom + point.Y - p.header.Min.Y, true
	case point.In(p.body):
		return p.body, p.bodyFrom + point.Y - p.body.Min.Y, true
	default:
		return image.Rectangle{}, 0, false
	}
}

func (p transcriptPresentation) nearestRow(y int) (image.Rectangle, int, bool) {
	header, body := p.header.Intersect(p.visible), p.body.Intersect(p.visible)
	if !header.Empty() && y < p.body.Min.Y {
		at := min(max(y, header.Min.Y), header.Max.Y-1)
		return p.header, p.headerFrom + at - p.header.Min.Y, true
	}
	if !body.Empty() {
		at := min(max(y, body.Min.Y), body.Max.Y-1)
		return p.body, p.bodyFrom + at - p.body.Min.Y, true
	}
	if !header.Empty() {
		at := min(max(y, header.Min.Y), header.Max.Y-1)
		return p.header, p.headerFrom + at - p.header.Min.Y, true
	}
	return image.Rectangle{}, 0, false
}
