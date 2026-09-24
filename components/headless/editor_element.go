package headless

import (
	"math"
	"unicode/utf8"

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
// A dropped image, a picked file or a mentioned name is shown as a word or two and
// stands for something the word is not, so letting the cursor walk into the middle of
// one leaves a fragment that still looks like the thing. An element is therefore
// atomic: the cursor steps over it, a delete takes all of it, and nothing lands
// inside. Its identity ordinarily survives editing around it.
//
// # When an identity ends
//
// What makes an element atomic is that it begins and ends where a caret may sit, and
// an edit beside one can take that away without touching a byte of it: a regional
// indicator left next to another is one flag, and a grapheme cluster cannot be half
// an element and half the text around it.
//
// An element in that position loses its identity. The text stays exactly as the edit
// left it and the element is gone from [Editor.Elements], as though it had been
// deleted; undo restores both, because it restores the document the element was still
// an element in. A program keying a payload by [Element.ID] learns of it the way it
// learns of a deletion — see [Editor.RetainedElementIDs].
//
// It is a [text.Mark] in the coordinates this editor speaks, and the rule keeping it
// over the same words as the text changes is that type's — see [text.Edit.Shift].
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

// elementBody is as much of a body as can be a run of cells between two separators.
//
// Both ends, because an element has two: a combining character joins what is in
// front of it, and a prepended mark takes what follows into its own cluster. A label
// that cannot offer a boundary at either end is a fragment rather than a label, and
// it is projected to what it can be shown as, the same way line breaks are flattened
// and controls removed.
//
// What remains may still join a particular neighbour — two regional indicators are a
// flag — and that is a question about where it is going rather than about the body.
// See [Editor.joinsWhatPrecedes].
func elementBody(body string) string {
	body = oneLineText(body)
	for body != "" && clusters(" "+body) != 1+clusters(body) {
		_, size := utf8.DecodeRuneInString(body)
		body = body[size:]
	}
	for body != "" && clusters(body+" ") != clusters(body)+1 {
		_, size := utf8.DecodeLastRuneInString(body)
		body = body[:len(body)-size]
	}
	return body
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
	return at + min(max(c.Col, 0), len(lines[line]))
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
