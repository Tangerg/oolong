package headless

import (
	"math"
	"slices"

	"github.com/Tangerg/oolong/core/text"
)

// ElementKind lets a host tell one family of atomic elements from another.
//
// The package assigns no meanings. What an element stands for — a file the user
// picked, an image they dropped, somebody they mentioned — is the program's business;
// what is this package's business is that the text behaves as one thing.
type ElementKind uint8

// Element is a run of an editor's text that behaves as one character.
//
// A prompt is often not only text. A dropped image, a picked file, a mentioned name:
// each is shown as a word or two and stands for something the word is not. Letting
// the cursor walk into the middle of one, or a backspace take a letter off the end of
// one, leaves a fragment that still looks like the thing and no longer is.
//
// So an element is atomic: the cursor steps over it, a delete takes all of it, and
// nothing lands inside. Its identity survives editing around it, which is what lets a
// program keep whatever the element stands for beside it.
//
// It is a [text.Mark] in the coordinates this editor speaks. The rule that keeps it
// over the same words while the text around it changes is that type's, and it is the
// same rule a highlight or a search result would need — see [text.Edit.Shift].
type Element struct {
	// ID is unique within one editor and stable for as long as the element exists.
	// It is what a program keys its own record of the element by.
	ID uint64
	// Kind is the program's own label.
	Kind ElementKind
	// Line is the logical line the element sits on, and Start and End its byte range
	// within that line, the end exclusive.
	//
	// An element never spans a line break. There is nothing to show for one that did:
	// what makes it one thing on screen is that it is one run of cells.
	Line       int
	Start, End int
}

// Text is the element's own text, given the editor it belongs to.
func (el Element) Text(e *Editor) string {
	e.ensure()
	if el.Line < 0 || el.Line >= len(e.lines) {
		return ""
	}
	line := e.lines[el.Line]
	if el.Start < 0 || el.End > len(line) || el.Start >= el.End {
		return ""
	}
	return line[el.Start:el.End]
}

// InsertElement puts text at the cursor as one atomic unit, and returns it.
//
// A separator space follows it, which is what makes a chip in a prompt something a
// user can type after. The space is ordinary text and not part of the element: it is
// there to be deleted.
func (e *Editor) InsertElement(kind ElementKind, body string) Element {
	if body == "" {
		return Element{}
	}
	e.ensure()
	e.endTyping()
	e.snapshot()
	e.dropSelection()

	at := e.offsetOf(Caret{Line: e.line, Col: e.col})
	e.splice(body + " ")
	e.nextElement++
	mark := text.Mark{
		ID:     e.nextElement,
		Kind:   int(kind),
		Start:  at,
		End:    at + len(body),
		Atomic: true,
	}
	// Put in order rather than appended and sorted: the marks are kept in the order
	// they appear, which is the order a caller expects and the order that makes them
	// readable in a test.
	where, _ := slices.BinarySearchFunc(e.marks, mark, func(a, b text.Mark) int {
		return a.Start - b.Start
	})
	e.marks = slices.Insert(e.marks, where, mark)
	return e.elementOf(mark)
}

// Elements is every element in the text, in the order they appear. The slice is a
// copy: a caller cannot move an element by writing to it.
func (e *Editor) Elements() []Element {
	out := make([]Element, 0, len(e.marks))
	for _, m := range e.marks {
		out = append(out, e.elementOf(m))
	}
	return out
}

// ElementAt is the element covering a position, and whether there is one. The end is
// exclusive, so the position just after an element is outside it.
func (e *Editor) ElementAt(line, col int) (Element, bool) {
	at := e.offsetOf(Caret{Line: line, Col: col})
	for _, m := range e.marks {
		if m.Covers(at) {
			return e.elementOf(m), true
		}
	}
	return Element{}, false
}

// RemoveElement deletes an element's text and forgets it, reporting whether it was
// there to remove.
func (e *Editor) RemoveElement(id uint64) bool {
	for _, m := range e.marks {
		if m.ID != id {
			continue
		}
		el := e.elementOf(m)
		e.endTyping()
		e.snapshot()
		// The space after it goes too, when there is one. It was put there with the
		// element and leaving it behind gives a prompt a gap where a chip used to be,
		// which is the sort of thing a user has to notice and tidy up by hand.
		end := el.End
		if line := e.lines[el.Line]; end < len(line) && line[end] == ' ' {
			end++
		}
		e.replaceRange(Caret{Line: el.Line, Col: el.Start}, Caret{Line: el.Line, Col: end}, "")
		return true
	}
	return false
}

// insideElement is the element a cursor position falls strictly within.
//
// Strictly, unlike [Editor.ElementAt]: an element's two ends are places a cursor may
// sit, and only what is between them is not. They are different questions and the
// difference matters — treating the start as inside would mean a cursor arriving from
// the left skipped straight past the element, and nothing could be typed in front of
// one.
func (e *Editor) insideElement(line, col int) (Element, bool) {
	at := e.offsetOf(Caret{Line: line, Col: col})
	for _, m := range e.marks {
		if m.Within(at) {
			return e.elementOf(m), true
		}
	}
	return Element{}, false
}

// snapElement moves a position out of any element it lands inside.
//
// Which way out depends on which way the cursor was going, which is the only thing
// that makes stepping over an element feel like stepping over a character: moving
// right from inside one has to come out at the far side, and moving left at the near
// side. A position that is not inside anything is returned as it is.
func (e *Editor) snapElement(line, col int, forward bool) int {
	el, inside := e.insideElement(line, col)
	if !inside {
		return col
	}
	if forward {
		return el.End
	}
	return el.Start
}

// edited moves every element over a change to the text, dropping the ones the change
// destroyed.
//
// This is the whole of it. There used to be two of these — one for text going in and
// one for text coming out — each doing the same arithmetic in line and column space,
// each with its own edge cases and its own way of being wrong. An insertion, a
// deletion and a replacement are one thing said three ways, and [text.Edit] is that
// thing.
//
// It must be called with offsets into the text as it was before the change, which is
// why every caller works them out first.
func (e *Editor) edited(edit text.Edit) {
	e.marks = edit.Shift(e.marks, e.byteLength())
}

// byteLength is the length of the whole text without assembling it. Edits and
// marks speak in whole-document byte offsets even though the editor owns lines.
func (e *Editor) byteLength() int {
	e.ensure()
	n := len(e.lines) - 1 // the newlines between lines
	for _, line := range e.lines {
		n += len(line)
	}
	return n
}

// removed moves every element over a range of the text being replaced by s, which
// covers a plain deletion as the case where s is empty.
//
// It has to be called before the lines change, because the carets it is given are
// carets into the text as it was.
func (e *Editor) removed(start, end Caret, s string) {
	e.edited(text.Edit{Start: e.offsetOf(start), End: e.offsetOf(end), Text: s})
}

// offsetOf is a caret as a byte offset into the whole text.
//
// The editor keeps its content as lines because that is what wrapping, vertical
// movement and the cursor are all expressed in. Marks are kept as offsets because
// that is what a change to text is expressed in, and translating between the two is
// this function and [Editor.caretAt]. Neither idea has to know about the other, which
// is the only reason the shifting rule could move out of this package at all.
func (e *Editor) offsetOf(c Caret) int {
	e.ensure()
	line := min(max(c.Line, 0), len(e.lines)-1)
	at := 0
	for i := range line {
		at += len(e.lines[i]) + 1
	}
	return at + min(max(c.Col, 0), len(e.lines[line]))
}

// caretAt is a byte offset as a line and a column.
func (e *Editor) caretAt(at int) Caret {
	e.ensure()
	for i, line := range e.lines {
		if at <= len(line) {
			return Caret{Line: i, Col: max(at, 0)}
		}
		at -= len(line) + 1
	}
	last := len(e.lines) - 1
	return Caret{Line: last, Col: len(e.lines[last])}
}

// elementOf is a mark in the coordinates the editor's callers speak.
func (e *Editor) elementOf(m text.Mark) Element {
	start := e.caretAt(m.Start)
	end := e.caretAt(m.End)
	if end.Line != start.Line {
		// An element never spans a line break, so a mark that reads as though it does
		// is reported as far as the end of the line it began on. Nothing produces one:
		// an edit that put a break inside a mark destroyed it.
		end = Caret{Line: start.Line, Col: len(e.lines[start.Line])}
	}
	return Element{
		ID:    m.ID,
		Kind:  kindOf(m.Kind),
		Line:  start.Line,
		Start: start.Col,
		End:   end.Col,
	}
}

// kindOf is a mark's label as this package's own.
//
// A mark carries whatever label its owner uses, so the label is an int; this owner
// uses one byte of it. Narrowing it here, where the value comes back, is what makes
// the conversion something a reader and an analyser can both follow — the alternative
// is a conversion whose safety has to be reconstructed by finding who put the value
// in.
func kindOf(label int) ElementKind {
	if label < 0 || label > math.MaxUint8 {
		return 0
	}
	return ElementKind(label)
}
