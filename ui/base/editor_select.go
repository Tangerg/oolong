package base

import (
	"context"
	"strings"
)

// Caret is a position in an editor's text: a logical line, and a byte offset into it.
//
// It is not a [Point]. A transcript numbers visual rows, because everything it
// answers is about what is on the screen; an editor's text has lines of its own that
// wrapping turns into rows, and a position in the text has to survive the window
// changing width. The two coordinate spaces are different questions, and one type for
// both would let an answer to one be passed as an answer to the other.
type Caret struct{ Line, Byte int }

func (c Caret) Before(d Caret) bool {
	return c.Line < d.Line || (c.Line == d.Line && c.Byte < d.Byte)
}

// Clipboard supplies the user-facing copy and read capability.
//
// A runtime adapter commonly provides it, but the interface is declared here where
// it is consumed. A caller with somewhere else to put text is equally valid.
type Clipboard interface {
	// Copy puts text where a paste would find it, reporting false for text it will
	// not carry.
	Copy(text string) bool
	// Read accepts a directed read. receive runs on the caller's UI owner sequence,
	// including for failure; it must never be invoked by a background worker.
	// Caller cancellation or this provider's owner ending revokes receive; no
	// callback is promised afterward. A returned error means no read was accepted.
	Read(ctx context.Context, receive func(string, error)) error
}

// ownedEditorLine joins pieces into storage owned by the surviving document. A range
// edit commonly keeps only the prefix or suffix of a large source line; retaining the
// source allocation would make deleting text keep the deleted bytes alive.
func ownedEditorLine(parts ...string) string {
	length := 0
	for _, part := range parts {
		length += len(part)
	}
	var line strings.Builder
	line.Grow(length)
	for _, part := range parts {
		line.WriteString(part)
	}
	return line.String()
}
