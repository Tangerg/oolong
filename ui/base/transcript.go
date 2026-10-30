package base

import (
	"context"
	"errors"
	"image"
	"math"
	"slices"
	"sort"

	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/text"
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

// TranscriptConfig combines reading policy, bounded retention and an optional publication owner.
type TranscriptConfig struct {
	Key            string
	Name           string
	Publisher      *publication.Publisher
	Retention      Retention
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
	revision uint64
	body     Block
	document *content.StreamDocument
	cancel   func()
	state    ContentState
	final    bool
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
	if i == nil {
		return ContentRetired
	}
	return i.state
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
	return i.err
}

// Transcript owns retained references and reading behavior. Content semantics
// stay in StreamDocument; permanent-output responsibility moves only after the
// publisher accepts an immutable capture. Layout is a scene-owned projection.
// Commands belong to the UI owner and reject mutation during scene presentation
// or content capture. Revision and row counters panic rather than wrap identities.
// A Transcript must not be copied after first use.
type Transcript struct {
	noCopy          noCopy
	cfg             TranscriptConfig
	initialized     bool
	closed          bool
	entries         []*TranscriptItem
	next            BlockID
	start           int
	revision        uint64
	publishing      bool
	stopPublication func()
	element         scene.Element
	scroll          Scroll
	selection       Selection
	sticky          Sticky
	matches         []Match
	current         int
	geometry        scene.Snapshot[TranscriptLayout]
	presentation    scene.Snapshot[transcriptPresentation]
	clicked         uint64
	dragged         bool
	scrollAction    *scene.Action[ScrollRequest, int]
}

// ActionScroll is the typed transcript action shared by semantic callers and reading controls.
const ActionScroll scene.ActionID = "scroll"

// ScrollRequest moves by terminal rows; positive values read toward the end.
type ScrollRequest struct{ Rows int }

// NewTranscript validates retention dependencies and creates one stable scene element.
func NewTranscript(cfg TranscriptConfig) (*Transcript, error) {
	if cfg.MaxItems < 0 || cfg.KeepCompleted < 0 || cfg.Retention > PublishCompleted {
		return nil, errors.New("base: invalid transcript configuration")
	}
	if cfg.Retention == PublishCompleted && cfg.Publisher == nil {
		return nil, errors.New("base: publishing transcript requires a publisher")
	}
	maxItems := cfg.MaxItems
	if maxItems == 0 {
		maxItems = 64
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
	if t.cfg.MaxItems == 0 {
		t.cfg.MaxItems = 64
	}
	t.next = 1
	t.current = -1
	t.scroll.ToBottom()
	t.scrollAction = scene.NewAction(scene.ActionConfig[ScrollRequest, int]{
		ID: ActionScroll, Label: "Scroll document",
		Availability: func(ScrollRequest) scene.Availability { return scene.Availability{Disabled: t.closed} },
		Run: func(_ context.Context, request ScrollRequest) (scene.ActionResult[int], error) {
			before := t.scroll.Offset()
			t.scroll.By(request.Rows)
			state := scene.Unchanged
			if t.scroll.Offset() != before {
				state = scene.Applied
				t.element.Invalidate()
			}
			return scene.ActionResult[int]{State: state, Value: t.scroll.Offset()}, nil
		},
	})
	t.element = scene.NewElement(scene.ElementConfig{
		Key:       t.cfg.Key,
		Focusable: func() bool { return !t.closed },
		Semantics: func() scene.Semantic {
			return scene.Semantic{Role: scene.RoleDocument, Name: t.cfg.Name, ContentRevision: t.revision, Availability: scene.ContentLive}
		},
		Handle:  t.handle,
		Actions: []scene.ActionBinding{t.scrollAction.Binding()},
		Paint:   func(frame scene.Frame) { view := t.StageWindow(frame); view.Content.Draw(view.Body, view.From) },
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
	if t.publishing {
		panic("base: transcript mutation during content capture")
	}
}

func (t *Transcript) changed() {
	if t.revision == math.MaxUint64 {
		panic("base: transcript revisions exhausted")
	}
	t.revision++
	t.element.Invalidate()
}

// Element is the sole attachment identity shared by all transcript reading behaviors and skins.
func (t *Transcript) Element() scene.Element { t.initialize(); return t.element }

// Scroll returns the owned reading-intent controller, not a copy of its current projection.
func (t *Transcript) Scroll() *Scroll { return &t.scroll }

// Selection returns the owned logical selection shared by input, copying and appearance.
func (t *Transcript) Selection() *Selection { return &t.selection }

// Sticky returns the owned set of pinnable content identities.
func (t *Transcript) Sticky() *Sticky { return &t.sticky }

// Len counts current retained references, including documents awaiting preparation.
func (t *Transcript) Len() int { return len(t.entries) }

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
	if len(t.entries) > 0 {
		return t.entries[0].id
	}
	if t.next == 0 {
		return 1
	}
	return t.next
}

func (t *Transcript) add(body Block, document *content.StreamDocument) (*TranscriptItem, error) {
	t.assertMutable()
	if len(t.entries) >= t.cfg.MaxItems {
		return nil, ErrTranscriptCapacity
	}
	if t.next == BlockID(math.MaxUint64) {
		return nil, errors.New("base: transcript identities exhausted")
	}
	item := &TranscriptItem{id: t.next, revision: 1, body: body, document: document, state: ContentRetained, final: document == nil}
	t.next++
	t.entries = append(t.entries, item)
	t.changed()
	return item, nil
}

// Add takes immutable passive content that is already final. Mutable sources use
// AddDocument so revision and completion cannot be maintained by a second caller.
func (t *Transcript) Add(body Block) (*TranscriptItem, error) {
	if body == nil {
		return nil, errors.New("base: transcript content is nil")
	}
	item, err := t.add(body, nil)
	if err != nil {
		return nil, err
	}
	t.publish()
	return item, nil
}

// AddDocument subscribes once to source revisions, final preparation and automatic retention transitions.
func (t *Transcript) AddDocument(document *content.StreamDocument) (*TranscriptItem, error) {
	if document == nil {
		return nil, errors.New("base: transcript document is nil")
	}
	item, err := t.add(nil, document)
	if err != nil {
		return nil, err
	}
	item.state = ContentLive
	item.cancel = document.Subscribe(func(change content.DocumentChange) { t.accept(item, change) })
	t.accept(item, document.Current())
	return item, nil
}

func (t *Transcript) accept(item *TranscriptItem, change content.DocumentChange) {
	t.assertMutable()
	if item.state == ContentRetired {
		return
	}
	if item.revision != uint64(change.PreparedRevision) {
		t.selection.ContentChanged(item.id, uint64(change.PreparedRevision))
	}
	item.revision = uint64(change.PreparedRevision)
	item.body = change.Body
	item.final = change.State == content.Final || t.cfg.PublishPartial && change.State == content.Partial
	item.err = errors.Join(change.SourceError, change.PrepareError)
	if change.Publishable() || change.State == content.Failed {
		item.state = ContentRetained
	} else {
		item.state = ContentLive
	}
	t.changed()
	t.publish()
}

func (t *Transcript) publish() {
	if t.closed || t.cfg.Retention != PublishCompleted || t.cfg.Publisher == nil || t.publishing {
		return
	}
	t.assertMutable()
	t.publishing = true
	defer func() {
		t.publishing = false
		if cap(t.entries) > 2*len(t.entries)+64 {
			t.entries = slices.Clone(t.entries)
		}
	}()
	completed := 0
	for _, item := range t.entries {
		if !item.final || item.body == nil {
			break
		}
		completed++
	}
	for range max(completed-t.cfg.KeepCompleted, 0) {
		item := t.entries[0]
		_, rows, _ := t.geometry.Value().Extent(item.id)
		if rows > math.MaxInt-t.start {
			panic("base: transcript row origin exhausted")
		}
		receipt, err := t.cfg.Publisher.TryPublish(item.body)
		if err != nil {
			if !errors.Is(err, publication.ErrCapacity) {
				item.err = errors.Join(item.err, err)
			}
			return
		}
		item.receipt = receipt
		t.start += rows
		t.scroll.Discard(rows)
		t.selection.DiscardBefore(item.id + 1)
		item.state = ContentRetired
		if item.cancel != nil {
			item.cancel()
			item.cancel = nil
		}
		item.body, item.document = nil, nil
		t.entries[0] = nil
		t.entries = t.entries[1:]
		if len(t.entries) == 0 {
			t.entries = nil
		}
		t.sticky.DiscardBefore(t.FirstBlock())
		t.matches = nil
		t.current = -1
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
	if t.publishing {
		panic("base: transcript close during publication")
	}
	t.closed = true
	if t.stopPublication != nil {
		t.stopPublication()
		t.stopPublication = nil
	}
	for _, item := range t.entries {
		if item.cancel != nil {
			item.cancel()
			item.cancel = nil
		}
		item.body, item.document = nil, nil
		item.state = ContentRetired
	}
	clear(t.entries)
	t.entries = nil
	t.matches = nil
	t.selection.Clear()
	t.changed()
}

// Block resolves a currently retained identity, returning nil after retirement.
func (t *Transcript) Block(id BlockID) Block {
	for _, item := range t.entries {
		if item.id == id {
			return item.body
		}
	}
	return nil
}

// Last returns the final retained passive result, which can be nil before its first preparation.
func (t *Transcript) Last() Block {
	if len(t.entries) == 0 {
		return nil
	}
	return t.entries[len(t.entries)-1].body
}

// Stage measures into immutable candidate storage. It never updates content,
// scroll intention or subscriptions and a failed outer frame discards the layout.
// It panics if the cumulative row coordinates overflow int, or the frame is expired
// or has already staged this transcript projection.
func (t *Transcript) Stage(frame scene.Frame, width int) TranscriptLayout {
	width = max(width, 0)
	state := TranscriptLayout{width: width, start: t.start, first: t.FirstBlock()}
	top := t.start
	for _, item := range t.entries {
		rows := 0
		if item.body != nil && width > 0 {
			rows = max(item.body.HeightForWidth(width), 0)
		}
		if rows > math.MaxInt-top {
			panic("base: transcript row coordinates exhausted")
		}
		var mapping *transcriptTextMap
		if projector, ok := item.body.(TextProjector); ok {
			mapping = newTranscriptTextMap(projector.Rows(width))
		}
		state.blocks = append(state.blocks, placed{id: item.id, revision: item.revision, mapping: mapping, block: item.body, top: top, height: rows})
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
	blocks             []placed
	width, rows, start int
	first              BlockID
}

// Width is the column count used by every placement in this projection.
func (l TranscriptLayout) Width() int { return l.width }

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
	for _, b := range l.blocks {
		if b.id == id {
			return b.block
		}
	}
	return nil
}

// Extent returns the absolute top and height of a content identity in this layout.
func (l TranscriptLayout) Extent(id BlockID) (top, height int, ok bool) {
	for _, b := range l.blocks {
		if b.id == id {
			return b.top, b.height, true
		}
	}
	return 0, 0, false
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

// Draw paints only placements intersecting the supplied row window and leaves geometry unchanged.
func (l TranscriptLayout) Draw(view grid.View, from int) {
	width, height := view.Size()
	if width <= 0 || height <= 0 {
		return
	}
	first, last := l.visible(from, height)
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
		projector, ok := b.block.(TextProjector)
		if !ok {
			continue
		}
		rows := projector.Rows(l.width)
		for i, row := range rows {
			at := b.top + i - from
			if at >= 0 && at < count && i < b.height {
				out[at] = row
			}
		}
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
	first := t.FirstBlock() + BlockID(len(t.entries))
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

// Matches returns a detached copy of the current search projection.
func (t *Transcript) Matches() []Match {
	out := slices.Clone(t.matches)
	for i := range out {
		out[i].Spans = slices.Clone(out[i].Spans)
	}
	return out
}

// MatchesIn copies only the visible projection; archived search results do not
// make painting a fixed viewport proportional to the age of a transcript.
func (t *Transcript) MatchesIn(from, to int) (first int, matches []Match) {
	if len(t.matches) == 0 || from >= to {
		return 0, nil
	}
	first = sort.Search(len(t.matches), func(i int) bool { return t.matches[i].Row >= from })
	if first > 0 {
		previous := t.matches[first-1]
		if layout.Sum(previous.Row, len(previous.Spans)) > from {
			first--
		}
	}
	last := sort.Search(len(t.matches), func(i int) bool { return t.matches[i].Row >= to })
	matches = slices.Clone(t.matches[first:max(first, last)])
	for i := range matches {
		matches[i].Spans = slices.Clone(matches[i].Spans)
	}
	return first, matches
}

// CurrentMatch returns the revealed match index, or -1 when no match is current.
func (t *Transcript) CurrentMatch() int { return t.current }

// SetMatches copies a sorted search projection and clears its revealed match; callers must validate source revision.
func (t *Transcript) SetMatches(matches []Match) {
	t.assertMutable()
	t.matches = slices.Clone(matches)
	for i := range t.matches {
		t.matches[i].Spans = slices.Clone(t.matches[i].Spans)
	}
	t.current = -1
}

// RevealMatch selects a valid match and asks the scroll owner to reveal its complete row span.
func (t *Transcript) RevealMatch(index int) bool {
	t.assertMutable()
	if index < 0 || index >= len(t.matches) || len(t.matches[index].Spans) == 0 {
		return false
	}
	match := t.matches[index]
	t.current = index
	first := match.Row - t.StartRow()
	t.scroll.Reveal(first, layout.Sum(first, len(match.Spans)-1))
	return true
}

// TranscriptView is a single candidate shared by paint and pointer projection.
// The borrowed Body remains writable only during its scene Prepare call.
type TranscriptView struct {
	Content    TranscriptLayout
	Body       grid.View
	From       int
	Header     image.Rectangle
	HeaderFrom int
	Pinned     Pinned
}

// StageWindow composes layout, reading anchor, sticky header and pointer mapping into one frame candidate.
func (t *Transcript) StageWindow(frame scene.Frame) TranscriptView {
	width, height := frame.Size()
	state := t.Stage(frame, width)
	scroll := t.scroll.Stage(frame, state.Height(), height)
	if old := t.geometry.Value(); old.Width() != width {
		if anchor, ok := t.ReadingAnchor(); ok && !anchor.FollowingEnd {
			if at, valid := state.Caret(anchor.Point); valid {
				scroll.Anchor(at.Row - state.StartRow() - anchor.ViewportOffset)
			}
		}
	}
	from := state.StartRow() + scroll.Offset()
	view := TranscriptView{Content: state, Body: frame.View, From: from}
	body := grid.Area(0, 0, width, height)
	pinned, ok := t.sticky.At(state, from, height)
	if ok && pinned.Rows < height {
		scroll.Resize(layout.Remaining(height, pinned.Rows))
		from = state.StartRow() + scroll.Offset()
		body = grid.Area(0, pinned.Rows, width, layout.Remaining(height, pinned.Rows))
		view.Body = frame.Sub(body).View
		view.From = from
		view.Pinned = pinned
		if top, _, exists := state.Extent(pinned.Block); exists && pinned.Visible() > 0 {
			view.Header = grid.Area(0, 0, width, pinned.Visible())
			view.HeaderFrom = top + pinned.ClipTop
		}
	}
	t.presentation.Stage(frame, transcriptPresentation{layout: state, revision: t.revision, visible: frame.Visible(), body: body, header: view.Header, from: from, headerFrom: view.HeaderFrom})
	return view
}

func (t *Transcript) handle(event scene.Event) scene.EventResult {
	presented, valid := event.Read(&t.presentation)
	consumed := t.handleInput(event.Input, presented, valid)
	mouse, ok := event.Input.(input.Mouse)
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
		t.dragged = false
		t.selection.Done()
		return false
	case input.MouseUp:
		if mouse.Releases(input.ButtonLeft) {
			was := t.dragged
			t.dragged = false
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
	t.dragged = false
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
		t.selection.Clicks.Reset()
		t.clicked = p.revision
	}
	switch t.selection.Clicks.Press(mouse) {
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
	t.selection.Begin(selected.Start)
	t.selection.Extend(selected.End)
	t.dragged = true
	return true
}

func (t *Transcript) drag(mouse input.Mouse, p transcriptPresentation) bool {
	if !t.dragged || !mouse.Drags(input.ButtonLeft) {
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
	from, headerFrom      int
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
		return p.body, p.from + point.Y - p.body.Min.Y, true
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
		return p.body, p.from + at - p.body.Min.Y, true
	}
	if !header.Empty() {
		at := min(max(y, header.Min.Y), header.Max.Y-1)
		return p.header, p.headerFrom + at - p.header.Min.Y, true
	}
	return image.Rectangle{}, 0, false
}
