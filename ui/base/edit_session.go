package base

import (
	"math"
	"slices"
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
		return c.NewEnd + (offset - c.OldEnd)
	default:
		return c.Start + min(offset-c.Start, c.NewEnd-c.Start)
	}
}

// EditSession owns document bytes, element identities, input and masking policy,
// the revision and one undo sequence. Editors own their caret, selection and scroll.
// It is used on one UI owner sequence and must not be copied after first use.
type EditSession struct {
	changes      subscriptions[ChangeMap]
	noCopy       noCopy
	lines        []string
	marks        []text.Mark
	elementIDs   identitySequence
	elementOwner *byte
	history      editHistory
	revision     uint64
	singleLine   bool
	mask         string
	views        []weak.Pointer[Editor]
	lastChange   ChangeMap
}

// EditSessionConfig supplies the initial document and its single-line policy. Views sharing a session cannot configure a second document policy.
type EditSessionConfig struct {
	Initial    string
	SingleLine bool
}

// NewEditSession creates the sole document, revision and undo-history owner. Each Editor keeps its own selection and scroll intent.
func NewEditSession(config EditSessionConfig) *EditSession {
	s := &EditSession{}
	s.initialize(config)
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
	s.replace(nil, editReplacement{text: value, whole: true, history: discardEdits}, nil)
}

func (s *EditSession) commit(initiator *Editor, oldLines []string, change ChangeMap, extendHistory bool) {
	if s.revision == math.MaxUint64 {
		panic("base: edit session exhausted revisions")
	}
	change.Before, change.After = s.revision, s.revision+1
	if extendHistory {
		s.history.extend(change)
	}
	kept := s.views[:0]
	for _, reference := range s.views {
		view := reference.Value()
		if view == nil {
			continue
		}
		kept = append(kept, reference)
		view.documentChanged(oldLines, change, view == initiator)
	}
	clear(s.views[len(kept):])
	s.views = kept
	s.lastChange = change
	s.revision = change.After
	s.changes.publish(change, s.invalidateViews)
}

func (s *EditSession) invalidateViews() {
	// Register all wakes before the first focus predicate can interrupt delivery.
	for _, reference := range slices.Backward(s.views) {
		if view := reference.Value(); view != nil {
			defer view.publishViewUpdate()
		}
	}
}

func (s *EditSession) requireMutable() {
	for _, reference := range s.views {
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

func (s *EditSession) initialize(config EditSessionConfig) {
	s.singleLine = config.SingleLine
	s.Reset(config.Initial)
}

func (s *EditSession) attach(view *Editor) {
	if len(s.views) == cap(s.views) {
		live := s.views[:0]
		for _, reference := range s.views {
			if reference.Value() != nil {
				live = append(live, reference)
			}
		}
		clear(s.views[len(live):])
		s.views = live
	}
	s.views = append(s.views, weak.Make(view))
}

func (s *EditSession) ensure() {
	if len(s.lines) == 0 {
		s.lines = []string{""}
	}
}

func (s *EditSession) end() Caret {
	if len(s.lines) == 0 {
		return Caret{}
	}
	last := len(s.lines) - 1
	return Caret{Line: last, Byte: len(s.lines[last])}
}

func (s *EditSession) oneLine() bool { return s.singleLine || s.mask != "" }

// Refusal must precede every owned change, including history and element identity,
// so recovering a panic cannot expose a partial transaction under an old revision.
func (s *EditSession) requireRevision() {
	s.requireMutable()
	if s.revision == math.MaxUint64 {
		panic("base: edit session exhausted revisions")
	}
}
