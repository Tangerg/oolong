package base

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/text"
)

// ElementKind lets a host tell one family of atomic elements from another.
//
// The package assigns no meanings. What an element stands for — a file the user
// picked, an image they dropped, somebody they mentioned — is the program's business;
// what is this package's business is that the text behaves as one thing.
type ElementKind uint8

// ElementID names a retained mark within its originating edit session, including
// its undo history. It is comparable and does not retain the document.
type ElementID struct {
	owner  *byte
	serial uint64
}

// Element is an atomic run of editor text: movement skips it and deletion removes
// it whole. Editing may join its boundary to a neighboring grapheme, ending its
// identity without removing the text. Undo restores that identity; payload owners
// can track its lifetime with [Editor.RetainedElementIDs].
type Element struct {
	// ID is unique to the edit session and stable for as long as the element exists.
	// It is what a program keys its own record of the element by.
	ID   ElementID
	Kind ElementKind
	// Line is the logical line the element sits on, and Start and End its byte range
	// within that line, the end exclusive.
	//
	// An element never spans a line break. There is nothing to show for one that did:
	// what makes it one thing on screen is that it is one run of cells.
	Line       int
	Start, End int
	body       string
}

// Text returns the content captured with this element's coordinates. Further
// edits cannot make an old snapshot read a replacement range.
func (el Element) Text() string { return el.body }

// Sentinel spaces expose the edge clusters that would absorb a separator. One
// segmentation removes both edges without rescanning the body for each removed rune.
// Actual neighboring text may still join the body; insertion checks that boundary.
func elementBody(body string) string {
	body = oneLineText(body)
	framed := " " + body + " "
	start, end := 0, len(body)
	for at, cluster := range text.Clusters(framed) {
		if at == 0 {
			start = len(cluster) - 1
		}
		if at+len(cluster) == len(framed) {
			end = at - 1
		}
	}
	return body[start:end]
}

// clusterPosition is the closest caret position in direction when at falls inside a
// grapheme cluster. Editing can change segmentation across the insertion boundary,
// so start+len(inserted) is not by itself proof that a cursor can still sit there.
func clusterPosition(line string, at int, forward bool) int {
	at = min(max(at, 0), len(line))
	if at == 0 || at == len(line) {
		return at
	}
	// Editor storage contains no ASCII controls other than tab, so two adjacent ASCII
	// bytes always have a grapheme boundary between them. This is the ordinary typing
	// path and avoids walking the line from its start on every keystroke.
	if line[at-1] < utf8.RuneSelf && line[at] < utf8.RuneSelf {
		return at
	}
	for start, cluster := range text.Clusters(line) {
		after := start + len(cluster)
		if at == start || at == after {
			return at
		}
		if at < after {
			if forward {
				return after
			}
			return start
		}
	}
	return len(line)
}

// offsetInLines is the read-only form used by render projections that must not
// initialize or otherwise mutate the editor they reflect.
func offsetInLines(lines []string, c Caret) int {
	if len(lines) == 0 {
		return 0
	}
	line := min(max(c.Line, 0), len(lines)-1)
	at := 0
	for i := range line {
		at += len(lines[i]) + 1
	}
	return at + min(max(c.Byte, 0), len(lines[line]))
}

// Guard narrowing where mark labels return from the generic text model; this editor uses one byte.
func kindOf(label int) ElementKind {
	if label < 0 || label > math.MaxUint8 {
		return 0
	}
	return ElementKind(label)
}

func (s *EditSession) joinsWhatPrecedes(at Caret, body string) bool {
	if at.Line < 0 || at.Line >= len(s.lines) {
		return false
	}
	line := s.lines[at.Line]
	before := line[:min(max(at.Byte, 0), len(line))]
	if before == "" {
		return false
	}
	return clusters(before+body) != clusters(before)+clusters(body)
}

func (s *EditSession) insideElement(line, byteOffset int) (Element, bool) {
	at := s.offsetOf(Caret{Line: line, Byte: byteOffset})
	for _, m := range s.marks {
		if m.Within(at) {
			return s.elementRange(m), true
		}
	}
	return Element{}, false
}

func (s *EditSession) snapElement(line, byteOffset int, forward bool) int {
	// The zero document has a conceptual empty line without allocating during projection.
	if len(s.lines) == 0 {
		return 0
	}
	line = min(max(line, 0), len(s.lines)-1)
	byteOffset = clusterPosition(s.lines[line], byteOffset, forward)
	return s.snapElementBoundary(line, byteOffset, forward)
}

func (s *EditSession) snapElementBoundary(line, byteOffset int, forward bool) int {
	for {
		el, inside := s.insideElement(line, byteOffset)
		if !inside {
			return byteOffset
		}
		if forward {
			byteOffset = clusterPosition(s.lines[line], el.End, true)
		} else {
			byteOffset = clusterPosition(s.lines[line], el.Start, false)
		}
	}
}

func (s *EditSession) settleMarks() {
	s.marks = slices.DeleteFunc(s.marks, func(m text.Mark) bool {
		return !s.spansWholeClusters(m)
	})
}

func (s *EditSession) spansWholeClusters(m text.Mark) bool {
	start, end := s.caretAt(m.Start), s.caretAt(m.End)
	if start.Line != end.Line {
		return false
	}
	line := s.lines[start.Line]
	return clusterPosition(line, start.Byte, true) == start.Byte &&
		clusterPosition(line, end.Byte, true) == end.Byte
}

func (s *EditSession) byteLength() int {
	n := max(len(s.lines)-1, 0)
	for _, line := range s.lines {
		n += len(line)
	}
	return n
}

func (s *EditSession) offsetOf(c Caret) int {
	return offsetInLines(s.lines, c)
}

func (s *EditSession) caretAt(at int) Caret {
	for i, line := range s.lines {
		if at <= len(line) {
			return Caret{Line: i, Byte: max(at, 0)}
		}
		at -= len(line) + 1
	}
	return s.end()
}

func (s *EditSession) elementOf(m text.Mark) Element {
	el := s.elementRange(m)
	el.body = strings.Clone(s.lines[el.Line][el.Start:el.End])
	return el
}

func (s *EditSession) elementRange(m text.Mark) Element {
	start := s.caretAt(m.Start)
	end := s.caretAt(m.End)
	if end.Line != start.Line {
		// An element never spans a line break, so a mark that reads as though it does
		// is reported as far as the end of the line it began on. Nothing produces one:
		// an edit that put a break inside a mark destroyed it.
		end = Caret{Line: start.Line, Byte: len(s.lines[start.Line])}
	}
	return Element{
		ID:    ElementID{owner: s.elementOwner, serial: m.ID},
		Kind:  kindOf(m.Kind),
		Line:  start.Line,
		Start: start.Byte,
		End:   end.Byte,
	}
}

func (s *EditSession) textBetween(start, end Caret) string {
	if start.Line == end.Line {
		return s.lines[start.Line][start.Byte:end.Byte]
	}
	var b strings.Builder
	b.WriteString(s.lines[start.Line][start.Byte:])
	for i := start.Line + 1; i < end.Line; i++ {
		b.WriteByte('\n')
		b.WriteString(s.lines[i])
	}
	b.WriteByte('\n')
	b.WriteString(s.lines[end.Line][:end.Byte])
	return b.String()
}

func (s *EditSession) retainedElementIDs() []ElementID {
	ids := make(map[uint64]struct{})
	for _, mark := range s.marks {
		ids[mark.ID] = struct{}{}
	}
	for _, stack := range [][]editSnapshot{s.history.undo, s.history.redo} {
		for _, state := range stack {
			for _, mark := range state.marks {
				ids[mark.ID] = struct{}{}
			}
		}
	}
	out := make([]ElementID, 0, len(ids))
	for id := range ids {
		out = append(out, ElementID{owner: s.elementOwner, serial: id})
	}
	slices.SortFunc(out, func(a, b ElementID) int { return cmp.Compare(a.serial, b.serial) })
	return out
}
