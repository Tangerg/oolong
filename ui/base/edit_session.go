package base

import (
	"math"
	"strings"
	"weak"

	"github.com/Tangerg/oolong/ui/core/text"
)

// ChangeMap maps byte positions from one document revision to its successor.
// The replacement interval is derived once by the edit owner; every connected
// view uses it before the new revision becomes observable.
type ChangeMap struct {
	Before, After         uint64
	Start, OldEnd, NewEnd int
}

// Position maps a byte offset through the committed replacement. Interior positions
// retain as much distance from the start as fits; an insertion boundary follows
// the inserted text. The consuming text layout snaps the result to its boundaries.
func (c ChangeMap) Position(offset int) int {
	switch {
	case offset < c.Start:
		return max(offset, 0)
	case offset >= c.OldEnd:
		return c.NewEnd + offset - c.OldEnd
	default:
		return c.Start + min(offset-c.Start, c.NewEnd-c.Start)
	}
}

// EditSession owns document bytes, element identities, the revision and one undo
// sequence. Editors only own their cursor, selection and scroll position.
// It is used on one UI owner sequence and must not be copied after first use.
type EditSession struct {
	changes    subscriptions[ChangeMap]
	noCopy     noCopy
	lines      []string
	marks      []text.Mark
	elementIDs identitySequence
	history    editorHistory
	revision   uint64
	singleLine bool
	published  string
	views      []weak.Pointer[Editor]
	lastChange ChangeMap
}

// EditSessionConfig supplies the initial document and its single-line policy. Views sharing a session cannot configure a second document policy.
type EditSessionConfig struct {
	Initial    string
	SingleLine bool
}

// NewEditSession creates the sole document, revision and undo-history owner. Each Editor keeps its own selection and scroll intent.
func NewEditSession(config EditSessionConfig) *EditSession {
	s := &EditSession{singleLine: config.SingleLine}
	s.Reset(config.Initial)
	return s
}

// Text returns the complete current document without mutating its revision.
func (s *EditSession) Text() string {
	if s == nil {
		return ""
	}
	return strings.Join(s.lines, "\n")
}

// Revision identifies one committed document state; caret and scroll changes do not advance it.
func (s *EditSession) Revision() uint64 {
	if s == nil {
		return 0
	}
	return s.revision
}

// LastChange returns the mapping for the most recent committed text transaction.
func (s *EditSession) LastChange() ChangeMap {
	if s == nil {
		return ChangeMap{}
	}
	return s.lastChange
}

// Reset replaces the document and starts a new undo sequence. Existing views are
// mapped by the same transaction as a normal edit, never synchronized afterward.
func (s *EditSession) Reset(value string) {
	if s == nil {
		return
	}
	e := Editor{session: s}
	e.replaceDocument(value, false)
}

// EditorConfig chooses the document owner. Initial is used only when Session is
// absent; supplying both is a construction error rather than an arbitration rule.
type EditorConfig struct {
	Session     *EditSession
	Initial     string
	Placeholder string
	MaxRows     int
	SingleLine  bool
}

// NewEditor constructs one view. It panics if Session is combined with Initial or SingleLine, which would supply competing document owners.
func NewEditor(config EditorConfig) *Editor {
	if config.Session != nil && (config.Initial != "" || config.SingleLine) {
		panic("base: editor Session cannot be combined with Initial or SingleLine")
	}
	e := &Editor{session: config.Session, Placeholder: strings.Clone(config.Placeholder), MaxRows: config.MaxRows}
	if e.session == nil {
		e.local.singleLine = config.SingleLine
		e.replaceDocument(config.Initial, false)
	}
	e.connect()
	return e
}

func (e *Editor) document() *EditSession {
	if e.session != nil {
		return e.session
	}
	return &e.local
}

// Session returns the document owner used by this view. Its subscriptions observe completed document transactions.
func (e *Editor) Session() *EditSession { e.connect(); return e.document() }

func (e *Editor) connect() {
	if e.registered {
		return
	}
	e.registered = true
	session := e.document()
	if len(session.views) == cap(session.views) {
		live := session.views[:0]
		for _, reference := range session.views {
			if reference.Value() != nil {
				live = append(live, reference)
			}
		}
		clear(session.views[len(live):])
		session.views = live
	}
	session.views = append(session.views, weak.Make(e))
}

func (s *EditSession) commit(initiator *Editor) {
	if s.revision == math.MaxUint64 {
		panic("base: edit session exhausted revisions")
	}
	before, after := s.published, s.Text()
	start := commonRunePrefix(before, after)
	oldEnd, newEnd := commonRuneSuffixStarts(before, after, start)
	change := ChangeMap{Before: s.revision, After: s.revision + 1, Start: start, OldEnd: oldEnd, NewEnd: newEnd}
	oldLines := strings.Split(before, "\n")
	kept := s.views[:0]
	for _, reference := range s.views {
		view := reference.Value()
		if view == nil {
			continue
		}
		kept = append(kept, reference)
		if view == initiator {
			view.layout.stale = true
			view.wantColumn = -1
			continue
		}
		cursor := change.Position(offsetInLines(oldLines, Caret{Line: view.line, Col: view.col}))
		anchor := change.Position(offsetInLines(oldLines, view.anchor))
		position := view.caretAt(cursor)
		view.line, view.col = position.Line, view.snapElement(position.Line, position.Col, true)
		view.anchor = view.caretAt(anchor)
		view.anchor.Col = view.snapElement(view.anchor.Line, view.anchor.Col, false)
		view.endTyping()
		view.layout.stale = true
		view.wantColumn = -1
		view.revealCursor()
	}
	clear(s.views[len(kept):])
	s.views = kept
	s.published = after
	s.lastChange = change
	s.revision++
	initiator.revealCursor()
	for _, reference := range kept {
		if view := reference.Value(); view != nil {
			view.element.Invalidate()
		}
	}
	s.changes.publish(change)
}

func (e *Editor) requireMutable() {
	requireMutable(e.element)
	for _, reference := range e.document().views {
		if view := reference.Value(); view != nil {
			requireMutable(view.element)
		}
	}
}

// Subscribe receives the committed change after all connected view anchors have
// been mapped. Callbacks run synchronously on the UI owner sequence.
// Subscribe panics for a nil callback.
func (s *EditSession) Subscribe(callback func(ChangeMap)) func() {
	if callback == nil {
		panic("base: nil edit session subscriber")
	}
	return s.changes.add(callback)
}
