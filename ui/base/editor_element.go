package base

import (
	"math"
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
