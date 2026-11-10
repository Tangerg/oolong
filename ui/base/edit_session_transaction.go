package base

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/text"
)

type editHistoryPolicy uint8

const (
	recordEdit editHistoryPolicy = iota
	coalesceEdit
	discardEdits
)

type editReplacement struct {
	start, end     Caret
	text           string
	history        editHistoryPolicy
	whole          bool
	expandClusters bool
	mark           *elementMark
}

// Text, marks and history settle before the view callback, which may only settle
// its local interaction state. Publication is last so a subscriber's newer edit
// cannot be overwritten by unfinished work from this transaction.
func (s *EditSession) replace(view *Editor, replacement editReplacement, settleView func(Caret, bool)) {
	s.requireMutable()
	s.ensure()
	start, end := replacement.start, replacement.end
	if replacement.whole {
		start, end = Caret{}, s.end()
	}
	start, end = s.replacementBounds(start, end)
	// Yank owns inserted bytes even when they join a preceding or following grapheme.
	// Only an arbitrary caller-supplied replacement range needs outward expansion.
	if replacement.expandClusters {
		start.Byte, end.Byte = completeClusters(s.lines[start.Line], start.Byte, end.Byte)
	}
	value := s.canonicalText(replacement.text)
	changed := replacement.mark != nil || s.replacementChanges(start, end, value)
	if changed {
		s.requireRevision()
	}
	if view != nil {
		// Only a coalesced edit that changes text continues the interaction; a
		// handled no-op ends it like every other no-op path.
		view.prepareEdit(replacement.history == coalesceEdit && changed)
	}
	s.prepareHistory(view, replacement.history, changed)
	if !changed {
		if settleView != nil {
			settleView(end, false)
		}
		return
	}
	var wholeChange ChangeMap
	if replacement.whole {
		// Whole replacements have no edit location; retaining their unchanged edges
		// keeps peer positions stable. Range edits keep their actual coordinates.
		previous := s.Text()
		wholeChange.Start = commonRunePrefix(previous, value)
		wholeChange.OldEnd, wholeChange.NewEnd = commonRuneSuffixStarts(previous, value, wholeChange.Start)
	}
	before, change, caret := s.applyRange(start, end, value)
	if replacement.whole {
		change = wholeChange
	}
	if replacement.mark != nil {
		mark := *replacement.mark
		where, _ := slices.BinarySearchFunc(s.marks, mark, func(a, b elementMark) int {
			return a.Start - b.Start
		})
		s.marks = slices.Insert(s.marks, where, mark)
	}
	if settleView != nil {
		settleView(caret, true)
	}
	s.commit(view, before, change, replacement.history != discardEdits)
}

func (s *EditSession) replacementBounds(start, end Caret) (Caret, Caret) {
	start.Line = min(max(start.Line, 0), len(s.lines)-1)
	end.Line = min(max(end.Line, start.Line), len(s.lines)-1)
	start.Byte = min(max(start.Byte, 0), len(s.lines[start.Line]))
	end.Byte = min(max(end.Byte, 0), len(s.lines[end.Line]))
	if start.Line == end.Line {
		end.Byte = max(start.Byte, end.Byte)
	}
	return start, end
}

// Equal text can still replace an atomic identity; an empty insertion cannot.
func (s *EditSession) replacementChanges(start, end Caret, value string) bool {
	if s.textBetween(start, end) != value {
		return true
	}
	if start == end && value == "" {
		return false
	}
	from, to := s.offsetOf(start), s.offsetOf(end)
	for _, mark := range s.marks {
		if from < mark.End && to > mark.Start {
			return true
		}
	}
	return false
}

func (s *EditSession) applyRange(start, end Caret, value string) ([]string, ChangeMap, Caret) {
	before := slices.Clone(s.lines)
	edit := text.Edit{Start: s.offsetOf(start), End: s.offsetOf(end), Text: value}
	s.marks = edit.Shift(s.marks, s.byteLength())
	change := ChangeMap{Start: edit.Start, OldEnd: edit.End, NewEnd: edit.Start + len(value)}
	head := s.lines[start.Line][:start.Byte]
	tail := s.lines[end.Line][end.Byte:]
	caret := Caret{Line: start.Line, Byte: start.Byte + len(value)}
	if !strings.Contains(value, "\n") {
		s.lines = slices.Replace(s.lines, start.Line, end.Line+1, ownedEditLine(head, value, tail))
	} else {
		parts := strings.Split(value, "\n")
		inserted := make([]string, len(parts))
		inserted[0] = ownedEditLine(head, parts[0])
		for i := 1; i < len(parts); i++ {
			inserted[i] = strings.Clone(parts[i])
		}
		last := len(inserted) - 1
		caret = Caret{Line: start.Line + last, Byte: len(inserted[last])}
		inserted[last] = ownedEditLine(inserted[last], tail)
		s.lines = slices.Replace(s.lines, start.Line, end.Line+1, inserted...)
	}
	s.settleMarks()
	return before, change, caret
}

func (s *EditSession) setPolicy(view *Editor, singleLine bool, mask string) {
	s.requireMutable()
	if mask != "" && !maskable(mask) {
		panic("base: editor mask must be visible terminal text that does not join a copy of itself")
	}
	if s.singleLine == singleLine && s.mask == mask {
		return
	}
	entering := !s.oneLine() && (singleLine || mask != "")
	if entering && len(s.lines) > 1 {
		s.requireRevision()
	}
	s.singleLine, s.mask = singleLine, strings.Clone(mask)
	if entering {
		// Clearing history also prevents a later undo from restoring old line breaks.
		s.history.clear()
		if len(s.lines) > 1 {
			before := s.lines
			at := offsetInLines(before, Caret{Line: view.line, Byte: view.byteOffset})
			flattened := strings.Join(before, " ")
			s.lines = []string{flattened}
			s.settleMarks()
			view.endTyping()
			view.finishReplacement(s.caretAt(at), true)
			s.commit(view, before, ChangeMap{OldEnd: len(flattened), NewEnd: len(flattened)}, false)
			return
		}
	}
	for _, reference := range s.views {
		if current := reference.Value(); current != nil {
			current.policyChanged()
		}
	}
	s.invalidateViews()
}

func (s *EditSession) insertElement(view *Editor, start, end Caret, kind ElementKind, body string, settleView func(Caret, bool)) Element {
	body = elementBody(body)
	if body == "" {
		return Element{}
	}
	s.requireRevision()
	s.ensure()
	start, end = s.replacementBounds(start, end)
	id, ok := s.elementIDs.next()
	if !ok {
		panic("base: editor exhausted element identities")
	}
	if s.elementOwner == nil {
		s.elementOwner = new(byte)
	}
	lead := ""
	if s.joinsWhatPrecedes(start, body) {
		lead = " "
	}
	at := s.offsetOf(start) + len(lead)
	mark := elementMark{ID: id, Kind: kind, Start: at, End: at + len(body), Atomic: true}
	var inserted Element
	s.replace(view, editReplacement{start: start, end: end, text: lead + body + " ", mark: &mark}, func(caret Caret, changed bool) {
		inserted = s.elementOf(mark)
		settleView(caret, changed)
	})
	return inserted
}

func (s *EditSession) elementRemoval(id ElementID) (Caret, Caret, bool) {
	if id.owner != s.elementOwner || id.owner == nil {
		return Caret{}, Caret{}, false
	}
	for _, mark := range s.marks {
		if mark.ID != id.serial {
			continue
		}
		element := s.elementRange(mark)
		end := element.End
		if line := s.lines[element.Line]; end < len(line) && line[end] == ' ' {
			end++
		}
		return Caret{Line: element.Line, Byte: element.Start}, Caret{Line: element.Line, Byte: end}, true
	}
	return Caret{}, Caret{}, false
}
