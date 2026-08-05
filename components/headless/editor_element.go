package headless

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
func (e *Editor) InsertElement(kind ElementKind, text string) Element {
	if text == "" {
		return Element{}
	}
	e.ensure()
	e.endTyping()
	e.snapshot()
	e.dropSelection()

	line, start := e.line, e.col
	e.splice(text + " ")
	e.nextElement++
	el := Element{ID: e.nextElement, Kind: kind, Line: line, Start: start, End: start + len(text)}
	e.elements = append(e.elements, el)
	e.sortElements()
	return el
}

// Elements is every element in the text, in the order they appear. The slice is a
// copy: a caller cannot move an element by writing to it.
func (e *Editor) Elements() []Element {
	out := make([]Element, len(e.elements))
	copy(out, e.elements)
	return out
}

// ElementAt is the element covering a position, and whether there is one. The end is
// exclusive, so the position just after an element is outside it.
func (e *Editor) ElementAt(line, col int) (Element, bool) {
	for _, el := range e.elements {
		if el.Line == line && col >= el.Start && col < el.End {
			return el, true
		}
	}
	return Element{}, false
}

// RemoveElement deletes an element's text and forgets it, reporting whether it was
// there to remove.
func (e *Editor) RemoveElement(id uint64) bool {
	for _, el := range e.elements {
		if el.ID != id {
			continue
		}
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
	for _, el := range e.elements {
		if el.Line == line && col > el.Start && col < el.End {
			return el, true
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

// spliceElements adjusts every element for text inserted at (line, col).
//
// added is how many line breaks the text carried, and tailCol where what used to
// follow the cursor now begins on the last of the new lines. Between them they say
// where everything after the insertion went.
//
// An element the insertion landed inside is dropped rather than stretched. It stood
// for something its text named, and text typed into the middle of it names something
// else — a chip that still looks like a file and points at half of one is worse than
// no chip.
func (e *Editor) spliceElements(line, col, added, tailCol int) {
	kept := e.elements[:0]
	for _, el := range e.elements {
		switch {
		case el.Line > line:
			el.Line += added
		case el.Line < line || el.End <= col:
			// Before the insertion, and unmoved.
		case el.Start < col:
			continue // the insertion landed inside it
		default:
			el.Line += added
			el.Start += tailCol - col
			el.End += tailCol - col
		}
		kept = append(kept, el)
	}
	e.elements = kept
}

// cutElements adjusts every element for the range [start, end) having been removed.
//
// Anything the range touched at all is dropped, for the same reason: half an element
// is not a smaller element.
func (e *Editor) cutElements(start, end Caret) {
	removed := end.Line - start.Line
	kept := e.elements[:0]
	for _, el := range e.elements {
		switch {
		case el.Line < start.Line || (el.Line == start.Line && el.End <= start.Col):
			// Entirely before the cut.
		case el.Line > end.Line:
			el.Line -= removed
		case el.Line == end.Line && el.Start >= end.Col:
			el.Line = start.Line
			el.Start += start.Col - end.Col
			el.End += start.Col - end.Col
		default:
			continue // the cut touched it
		}
		kept = append(kept, el)
	}
	e.elements = kept
}

// sortElements keeps them in the order they appear, which is the order a caller
// expects and the order that makes them readable in a test.
func (e *Editor) sortElements() {
	for i := 1; i < len(e.elements); i++ {
		for j := i; j > 0; j-- {
			a, b := e.elements[j-1], e.elements[j]
			if a.Line < b.Line || (a.Line == b.Line && a.Start <= b.Start) {
				break
			}
			e.elements[j-1], e.elements[j] = b, a
		}
	}
}
