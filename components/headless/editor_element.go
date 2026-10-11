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

// Element is an atomic run of editor text: movement skips it and deletion removes
// it whole. Editing may join its boundary to a neighboring grapheme, ending its
// identity without removing the text. Undo restores that identity; payload owners
// can track its lifetime with [Editor.RetainedElementIDs].
type Element struct {
	// ID is unique within one editor and stable for as long as the element exists.
	// It is what a program keys its own record of the element by.
	ID   uint64
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

// Both ends need grapheme boundaries so an element cannot partly own a neighbor.
// Actual neighboring text may still join the normalized body; the editor checks it
// at insertion.
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
