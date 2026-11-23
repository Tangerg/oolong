package base

import (
	"errors"
	"image"
	"math"
	"strings"

	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/internal/counter"
	"github.com/Tangerg/oolong/ui/internal/fifo"
	"github.com/Tangerg/oolong/ui/internal/identity"
)

// ErrTranscriptCapacity refuses another retained reference without discarding existing content.
var ErrTranscriptCapacity = errors.New("base: transcript item budget exhausted")

// errNilContent refuses absent content, which a transcript could neither measure nor draw.
var errNilContent = errors.New("base: transcript content is nil")

// ErrNotRetained refuses an identity the transcript no longer holds or never issued.
// Publication can retire an item before its owner revises it, so callers of a
// publishing transcript should expect it.
var ErrNotRetained = errors.New("base: transcript item is not retained")

// BlockID is a stable, never-reused identity within one transcript; zero identifies no item.
type BlockID uint64

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
	policy          transcriptPolicy
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
	// current is the item the reader acts on. Retention can retire it; Current reads
	// the oldest retained item in its place, so retirement needs no second update.
	current      BlockID
	matcher      keymap.Matcher
	sticky       Sticky
	geometry     scene.Snapshot[TranscriptLayout]
	presentation scene.Snapshot[transcriptPresentation]
}

// transcriptPolicy is the configuration a transcript keeps. Its key belongs to
// the element and its key map to the binding, so neither is retained here.
type transcriptPolicy struct {
	name           string
	publisher      TranscriptPublisher
	retention      Retention
	publishPartial bool
	keepCompleted  int
	maxItems       int
}

// maxItems is the item budget, with zero selecting the default.
func (c TranscriptConfig) maxItems() int {
	if c.MaxItems == 0 {
		return defaultTranscriptItems
	}
	return c.MaxItems
}

// Validate reports whether the retention settings depend on each other
// consistently, for settings that come from outside the program.
func (c TranscriptConfig) Validate() error {
	if c.MaxItems < 0 || c.KeepCompleted < 0 || c.Retention > PublishCompleted {
		return errors.New("base: invalid transcript configuration")
	}
	if c.Retention == PublishCompleted && identity.IsNil(c.Publisher) {
		return errors.New("base: publishing transcript requires a publisher")
	}
	if c.KeepCompleted >= c.maxItems() || c.KeepCompleted > 0 && c.Retention != PublishCompleted {
		return errors.New("base: recent completion retention requires publication and space for new content")
	}
	return nil
}

// NewTranscript creates one stable scene element. It panics for a configuration
// [TranscriptConfig.Validate] refuses.
func NewTranscript(cfg TranscriptConfig) *Transcript {
	if err := cfg.Validate(); err != nil {
		panic(err)
	}
	if identity.IsNil(cfg.Publisher) {
		cfg.Publisher = nil
	}
	t := &Transcript{
		policy: transcriptPolicy{
			name: strings.Clone(cfg.Name), publisher: cfg.Publisher, retention: cfg.Retention,
			publishPartial: cfg.PublishPartial, keepCompleted: cfg.KeepCompleted, maxItems: cfg.maxItems(),
		},
		keys: keyBinding{current: cfg.Keys},
	}
	t.initializeAs(strings.Clone(cfg.Key))
	return t
}

// initialize prepares a zero Transcript on first use, which has no element key.
func (t *Transcript) initialize() { t.initializeAs("") }

func (t *Transcript) initializeAs(key string) {
	if t.initialized {
		return
	}
	t.initialized = true
	t.namespace = new(byte)
	if t.policy.maxItems == 0 {
		t.policy.maxItems = defaultTranscriptItems
	}
	t.next = 1
	t.scroll.ToBottom()
	t.element = scene.NewElement(scene.ElementConfig{
		Key:       key,
		Enabled:   func() bool { return !t.closed },
		Focusable: func() bool { return true },
		Semantics: func() scene.Semantic {
			availability := scene.ContentLive
			if t.closed {
				availability = scene.ContentRetired
			}
			return scene.Semantic{Role: scene.RoleDocument, Name: t.policy.name, ContentRevision: t.revision, Availability: availability}
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
	t.keys.attach(t.element, &t.matcher)
	if t.policy.publisher != nil {
		t.stopPublication = t.policy.publisher.Subscribe(func() { t.publish() })
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
	const exhausted = "base: transcript revisions exhausted"
	remaining := math.MaxUint64 - t.revision
	for _, count := range counts {
		if count < 0 {
			panic(exhausted)
		}
		required := uint64(count)
		if required > remaining {
			panic(exhausted)
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

// KeysFor is what the effective key map binds to action, defaults included.
func (t *Transcript) KeysFor(action keymap.Action) []input.Keys {
	return t.keys.bound(scrollKeys, action)
}

// Current is the item commands act on, such as the one a copy reads, and whether
// there is one. Retention retires the oldest items first; when it retires the
// current one, the oldest item still retained becomes current.
func (t *Transcript) Current() (BlockID, bool) {
	if t.current == 0 || t.entries.Len() == 0 {
		return 0, false
	}
	return max(t.current, t.FirstBlock()), true
}

// SetCurrent makes a retained item current, reporting whether id is retained. It
// does not scroll; [Transcript.RevealBlock] does. It panics if the transcript is
// closed or its scene forbids model mutation.
func (t *Transcript) SetCurrent(id BlockID) bool {
	t.assertMutable()
	if _, ok := t.retained(id); !ok {
		return false
	}
	if t.current != id {
		t.current = id
		t.element.Invalidate()
	}
	return true
}

// Do moves the current item for SelectPrev, SelectNext, SelectFirst and SelectLast,
// revealing where it lands, and scrolls for the scroll actions. A step with no
// current item lands on what the reader can see: SelectPrev on the last item in
// view and SelectNext on the first, or on the last and first item before any frame.
func (t *Transcript) Do(action keymap.Action) bool {
	t.assertMutable()
	if t.entries.Len() == 0 {
		return t.scroll.Do(action)
	}
	first, last := t.FirstBlock(), t.entries.At(t.entries.Len()-1).id
	current, ok := t.Current()
	shownFirst, shownLast := t.inView(first, last)
	var next BlockID
	switch action {
	case SelectPrev:
		next = shownLast
		if ok {
			next = max(current-1, first)
		}
	case SelectNext:
		next = shownFirst
		if ok {
			next = min(current+1, last)
		}
	case SelectFirst:
		next = first
	case SelectLast:
		next = last
	default:
		return t.scroll.Do(action)
	}
	t.SetCurrent(next)
	t.RevealBlock(next)
	return true
}

// inView is the retained span the last presented frame showed in its body, or the
// whole retained span before a frame has shown any of it.
func (t *Transcript) inView(first, last BlockID) (BlockID, BlockID) {
	shown := t.presentation.Value()
	from, end := shown.layout.Visible(shown.bodyFrom, shown.body.Dy())
	if end <= from || end <= first || from > last {
		return first, last
	}
	return max(from, first), min(end-1, last)
}

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
	if t.entries.Len() >= t.policy.maxItems {
		return nil, ErrTranscriptCapacity
	}
	if _, ok := counter.Next(t.next); !ok {
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

// Add takes passive content that is already final. The body must not change behind
// the transcript's back: revise it with [Transcript.Replace], and give text that is still
// arriving to AddDocument, whose document owns its revision and completion.
// It panics before admission if adding and retiring the completed prefix could
// exhaust content revisions; revision identities must never wrap.
func (t *Transcript) Add(body Block) (*TranscriptItem, error) {
	if grid.IsNilDrawable(body) {
		return nil, errNilContent
	}
	return t.add(passiveContent{body: body, revision: 1}, nil)
}

// Replace gives retained passive content a new body in place, such as a disclosure
// opening or a result arriving. Its identity, its position and the reader's place in
// the transcript all stay; a selection touching it ends, because the text it named is
// gone. Content added with AddDocument belongs to its document and is refused. A retired
// or never-issued identity returns [ErrNotRetained]. It panics like Add when the
// change could exhaust content revisions.
func (t *Transcript) Replace(id BlockID, body Block) error {
	t.assertMutable()
	if grid.IsNilDrawable(body) {
		return errNilContent
	}
	item, ok := t.retained(id)
	if !ok {
		return ErrNotRetained
	}
	passive, ok := item.source.(passiveContent)
	if !ok {
		return errors.New("base: document content is revised by its document")
	}
	revision, ok := counter.Next(passive.revision)
	if !ok {
		return errors.New("base: transcript item revisions exhausted")
	}
	t.requireRevisions(1, t.publicationCount(nil))
	item.source = passiveContent{body: body, revision: revision}
	t.accept(item, item.content())
	return nil
}

// retained resolves an identity in O(1): identities are issued in order and leave
// only from the start, so the retained ones are exactly [FirstBlock, next).
func (t *Transcript) retained(id BlockID) (*TranscriptItem, bool) {
	first := t.FirstBlock()
	if id < first || id >= t.next {
		return nil, false
	}
	return t.entries.At(int(id - first)), true //nolint:gosec // G115: below next, so within the retained length.
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
	t.current = 0
	t.matcher.Clear()
	t.selection.clear()
	t.sticky.setBlocks(nil)
	// Close must release its projections even if no later frame is painted.
	t.geometry = scene.Snapshot[TranscriptLayout]{}
	t.presentation = scene.Snapshot[transcriptPresentation]{}
	t.changed()
}

// Block resolves a currently retained identity, returning nil after retirement.
func (t *Transcript) Block(id BlockID) Block {
	item, ok := t.retained(id)
	if !ok {
		return nil
	}
	return item.content().Body
}

// RevealBlock asks the scroll owner to bring a retained item into view. It measures
// the current content at the width last drawn, so a revision since that frame, such
// as a replacement that grew the item, is what gets revealed. It reports false for an
// identity that is not retained and before the first frame gives it a width.
func (t *Transcript) RevealBlock(id BlockID) bool {
	t.assertMutable()
	width := t.geometry.Value().Width()
	if _, ok := t.retained(id); !ok || width == 0 {
		return false
	}
	top := 0
	for item := range t.entries.Values() {
		rows := itemRows(item.content().Body, width)
		if item.id == id {
			t.scroll.Reveal(top, layout.Sum(top, max(rows-1, 0)))
			return true
		}
		top = layout.Sum(top, rows)
	}
	return false
}

// itemRows is how tall retained content is at width; a body still being prepared has
// no rows yet.
func itemRows(body Block, width int) int {
	if body == nil || width <= 0 {
		return 0
	}
	return max(body.HeightForWidth(width), 0)
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
		rows := itemRows(change.Body, width)
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
	// Content that changed above the reader, a reflow and a retired prefix all move
	// rows; the text the reader was looking at must not move with them.
	if old := t.geometry.Value(); old.key != state.key {
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
