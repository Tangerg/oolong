package base

import (
	"errors"
	"image"
	"math"
	"strings"

	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
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

// Retention policies require an explicit publisher before permanent output is enabled.
const (
	Retain Retention = iota
	PublishCompleted
)

// Content reference states distinguish revision, retention and irreversible responsibility transfer.
const (
	ContentLive ContentState = iota
	ContentRetained
	ContentRetired
)

// TranscriptPublisher admits complete content and notifies the UI owner when progress
// may allow a refused prefix to resume. Only successful admission transfers the content.
// Subscribe follows [publication.Publisher.Subscribe]'s deferred notification contract:
// the consumer holds a read-only borrow until it can settle the returned receipt.
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
	keys            keyBinding
	initialized     bool
	closed          bool
	entries         fifo.Queue[*TranscriptItem]
	next            BlockID
	startRow        int
	revision        uint64
	stopPublication func()
	element         scene.Element
	scroll          Scroll
	selection       Selection
	sticky          Sticky
	geometry        scene.Snapshot[TranscriptLayout]
	presentation    scene.Snapshot[transcriptPresentation]
}

// maxItems is the item budget, with zero selecting the default.
func (c TranscriptConfig) maxItems() int {
	if c.MaxItems == 0 {
		return defaultTranscriptItems
	}
	return c.MaxItems
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
	if cfg.KeepCompleted >= cfg.maxItems() || cfg.KeepCompleted > 0 && cfg.Retention != PublishCompleted {
		return nil, errors.New("base: recent completion retention requires publication and space for new content")
	}
	cfg.Key, cfg.Name = strings.Clone(cfg.Key), strings.Clone(cfg.Name)
	t := &Transcript{cfg: cfg, keys: keyBinding{current: cfg.Keys}}
	// The key binding owns the map from here; the retained configuration must not
	// keep a second reference that SetKeys would leave stale.
	t.cfg.Keys = nil
	t.initialize()
	return t, nil
}

func (t *Transcript) initialize() {
	if t.initialized {
		return
	}
	t.initialized = true
	t.namespace = new(byte)
	t.cfg.MaxItems = t.cfg.maxItems()
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
	t.keys.attach(t.element, &t.scroll.matcher)
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

func (t *Transcript) requireRevisions(counts ...int) {
	remaining := math.MaxUint64 - t.revision
	for _, count := range counts {
		if count < 0 {
			panic("base: transcript revisions exhausted")
		}
		required := uint64(count)
		if required > remaining {
			panic("base: transcript revisions exhausted")
		}
		remaining -= required
	}
}

func (t *Transcript) changed() {
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

// add installs one item. A source that can revise supplies subscribe, through
// which its later revisions reach the transcript; passive content has none.
func (t *Transcript) add(source itemSource, subscribe func(func(content.DocumentChange)) func()) (*TranscriptItem, error) {
	t.assertMutable()
	if t.entries.Len() >= t.cfg.MaxItems {
		return nil, ErrTranscriptCapacity
	}
	if t.next == BlockID(math.MaxUint64) {
		return nil, errors.New("base: transcript identities exhausted")
	}
	item := &TranscriptItem{id: t.next, source: source}
	// Admission may immediately retire the completed prefix. Reserve every
	// possible publication before installing a reference or transferring content.
	t.requireRevisions(1, t.publicationCount(item))
	if subscribe != nil {
		item.observed = source.Current().PreparedRevision
		item.cancel = subscribe(func(change content.DocumentChange) { t.accept(item, change) })
	}
	t.next++
	t.entries.Push(item)
	t.changed()
	t.publish()
	return item, nil
}

// Add takes immutable passive content that is already final. Mutable sources use
// AddDocument so revision and completion cannot be maintained by a second caller.
// It panics before admission if adding and retiring the completed prefix could
// exhaust content revisions; revision identities must never wrap.
func (t *Transcript) Add(body Block) (*TranscriptItem, error) {
	if grid.IsNilDrawable(body) {
		return nil, errors.New("base: transcript content is nil")
	}
	return t.add(passiveContent{body: body}, nil)
}

// AddDocument subscribes once to source revisions, final preparation and automatic retention transitions.
// It panics before admission if the addition and possible retirements could exhaust
// revisions. Later source notifications refuse transcript changes on exhaustion;
// they cannot undo changes already committed by the source document.
func (t *Transcript) AddDocument(document *content.StreamDocument) (*TranscriptItem, error) {
	if document == nil {
		return nil, errors.New("base: transcript document is nil")
	}
	return t.add(document, document.Subscribe)
}

func (t *Transcript) accept(item *TranscriptItem, change content.DocumentChange) {
	t.assertMutable()
	t.requireRevisions(1, t.publicationCount(nil))
	if item.observed != change.PreparedRevision {
		t.selection.contentChanged(item.id, uint64(change.PreparedRevision))
		item.observed = change.PreparedRevision
	}
	t.changed()
	t.publish()
}

func (t *Transcript) publicationCount(extra *TranscriptItem) int {
	if t.closed || t.cfg.Retention != PublishCompleted || t.cfg.Publisher == nil {
		return 0
	}
	completed := 0
	for item := range t.entries.Values() {
		if !t.publishable(item) {
			break
		}
		completed++
	}
	if extra != nil && completed == t.entries.Len() && t.publishable(extra) {
		completed++
	}
	return max(completed-t.cfg.KeepCompleted, 0)
}

func (t *Transcript) publishable(item *TranscriptItem) bool {
	change := item.content()
	return change.State == content.Final || t.cfg.PublishPartial && (change.State == content.Partial || change.State == content.Failed)
}

func (t *Transcript) publish() {
	count := t.publicationCount(nil)
	if count == 0 {
		return
	}
	t.assertMutable()
	t.requireRevisions(count)
	for range count {
		item := t.entries.At(0)
		_, rows, _ := t.geometry.Value().Extent(item.id)
		if rows > math.MaxInt-t.startRow {
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
		t.startRow += rows
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
// Revision exhaustion also panics before any retained reference is released.
func (t *Transcript) Close() {
	if t.closed {
		return
	}
	t.initialize()
	if err := t.element.CheckMutation(); err != nil {
		panic(err)
	}
	t.requireRevisions(1)
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
	state := TranscriptLayout{key: key, start: t.startRow, first: t.FirstBlock()}
	top := t.startRow
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
		if projector, ok := change.Body.(text.Projector); ok && rows > 0 {
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
