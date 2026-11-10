// Package diff keeps line boundaries in its edit script so consumers can render
// or apply changes without reconstructing them.
package diff

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

type Kind uint8

const (
	Context Kind = iota
	Added
	Removed
)

var ErrInvalidKind = errors.New("diff: invalid line kind")

func (k Kind) Validate() error {
	if k > Removed {
		return fmt.Errorf("%w: %d", ErrInvalidKind, k)
	}
	return nil
}

// String panics for an invalid kind so an unknown change cannot appear as context.
func (k Kind) String() string {
	if err := k.Validate(); err != nil {
		panic(err)
	}
	switch k {
	case Added:
		return "+"
	case Removed:
		return "-"
	default:
		return " "
	}
}

type Line struct {
	Kind Kind
	Text string
	// Old and New are the line's number in each text, counting from one, and zero
	// where the line is not in that text. A reader looking for what to open and where
	// needs the number in the text that still exists; a reader reading a change needs
	// both.
	Old, New int
}

// String panics for an invalid Kind so the marker cannot misrepresent the line.
func (l Line) String() string { return l.Kind.String() + l.Text }

// Script is a change to a text, as the lines that make it.
//
// It reads top to bottom as the change itself: every line of both texts appears once,
// in an order in which the removals and the additions of the same passage sit together.
// Same, String and Hunks panic for unknown line kinds so malformed input cannot be
// displayed as a different change. Validate supports error-returning input boundaries.
type Script []Line

func (s Script) Validate() error {
	for i, line := range s {
		if err := line.Kind.Validate(); err != nil {
			return fmt.Errorf("diff: line %d: %w", i, err)
		}
	}
	return nil
}

func Between(before, after []string) Script {
	// The ends first. Two versions of a file usually differ in the middle, and the
	// shortest edit script between them is found by not looking at the parts that are
	// the same.
	head := 0
	for head < len(before) && head < len(after) && before[head] == after[head] {
		head++
	}
	tail := 0
	for tail < len(before)-head && tail < len(after)-head &&
		before[len(before)-1-tail] == after[len(after)-1-tail] {
		tail++
	}

	out := make(Script, 0, len(before)+len(after))
	for i := range head {
		out = append(out, Line{Text: before[i], Old: i + 1, New: i + 1})
	}
	middle := myers{
		before: before[head : len(before)-tail],
		after:  after[head : len(after)-tail],
		from:   head,
	}
	out = append(out, middle.script()...)
	for i := range tail {
		at := len(before) - tail + i
		out = append(out, Line{Text: before[at], Old: at + 1, New: len(after) - tail + i + 1})
	}
	return out
}

// Same panics for any invalid line kind, even after an earlier change has already
// established that the script differs, so the answer covers the complete script.
func (s Script) Same() bool {
	if err := s.Validate(); err != nil {
		panic(err)
	}
	return !slices.ContainsFunc(s, func(line Line) bool { return line.Kind != Context })
}

// String panics for invalid line kinds so malformed input cannot acquire
// apparently valid change markers.
func (s Script) String() string {
	if err := s.Validate(); err != nil {
		panic(err)
	}
	var b strings.Builder
	for _, line := range s {
		b.WriteString(line.String())
		b.WriteByte('\n')
	}
	return b.String()
}

// Hunks is the changed parts of the script with context lines around them, and
// everything else left out.
//
// A file that changed in two places may have hundreds of unchanged lines. Overlapping runs of context
// are one hunk rather than two, because a gap of one unchanged line is not a gap worth
// drawing a break across.
//
// A context of zero is the changed lines alone. A script with nothing changed in it has
// no hunks at all, which is how "these are the same" is said.
//
// Each hunk owns its line storage and text. Retaining a small changed passage therefore
// does not retain the complete script or the source documents its lines were sliced
// from, and changing the script after this call cannot change an already-built hunk.
// Invalid line kinds panic before any hunk is constructed; every returned passage
// must describe a valid change.
func (s Script) Hunks(context int) []Hunk {
	if err := s.Validate(); err != nil {
		panic(err)
	}
	context = max(context, 0)
	var out []Hunk
	start, end := 0, 0
	for i, line := range s {
		if line.Kind == Context {
			continue
		}
		// Windows arrive in order. Merge each one once so dense changes
		// do not repeatedly mark the same context, even at MaxInt.
		left := i - min(context, i)
		right := i + 1 + min(context, len(s)-1-i)
		if end == 0 || left > end {
			if end != 0 {
				out = append(out, s.hunk(start, end))
			}
			start = left
		}
		end = right
	}
	if end != 0 {
		out = append(out, s.hunk(start, end))
	}
	return out
}

type Hunk struct {
	Lines Script
	// Old and New are where the hunk begins in each text, counting from one. A side
	// the hunk has no lines in, as for a pure insertion, begins one past the last
	// line of that text before the hunk.
	Old, New int
}

// String panics for invalid line kinds so the passage cannot misstate a change.
func (h Hunk) String() string { return h.Lines.String() }

// hunk copies s[start:end]. Its positions come from the lines before it, so a side
// with no lines in the hunk still has a place in its text.
func (s Script) hunk(start, end int) Hunk {
	h := Hunk{Lines: make(Script, end-start), Old: 1, New: 1}
	for i, line := range s[start:end] {
		line.Text = strings.Clone(line.Text)
		h.Lines[i] = line
	}
	for _, line := range slices.Backward(s[:start]) {
		if h.Old == 1 && line.Old > 0 {
			h.Old = line.Old + 1
		}
		if h.New == 1 && line.New > 0 {
			h.New = line.New + 1
		}
		if h.Old > 1 && h.New > 1 {
			break
		}
	}
	return h
}

// maxEdits bounds the Myers search, whose cost grows with differing lines.
// Beyond this limit, the changed middle is represented as removal and addition.
const maxEdits = 512

// myers retains one frontier history for the forward search and backtracking,
// preventing the two phases from reconstructing different paths.
type myers struct {
	before, after []string
	// from is how many lines were the same before these, which is what turns a
	// position here into a line number there.
	from int

	// path is where every diagonal had got to before each edit was made. It is what
	// the answer is reconstructed from: an edit script is a path through a grid, and
	// this is the grid's frontier at each step.
	path [][]int
	// offset is where the diagonal through the origin sits in each snapshot, since
	// diagonals are numbered from negative to positive and a slice is not.
	offset int
}

func (m *myers) script() Script {
	switch {
	case len(m.before) == 0 && len(m.after) == 0:
		return nil
	case len(m.before) == 0 || len(m.after) == 0 || !m.trace():
		return m.replaced()
	}
	return m.walk()
}

func (m *myers) replaced() Script {
	out := make(Script, 0, len(m.before)+len(m.after))
	for i, line := range m.before {
		out = append(out, Line{Kind: Removed, Text: line, Old: m.from + i + 1})
	}
	for i, line := range m.after {
		out = append(out, Line{Kind: Added, Text: line, New: m.from + i + 1})
	}
	return out
}

func (m *myers) trace() bool {
	n, size := len(m.before), len(m.after)
	// Bound each operand before adding it. The limit is deliberately tiny, while
	// the inputs are caller-sized; their unbounded sum is neither needed nor safe.
	limit := min(min(n, maxEdits)+min(size, maxEdits), maxEdits)
	// v holds the furthest x reached on each diagonal k, offset so that k = 0 sits in
	// the middle.
	v := make([]int, 2*limit+3)
	m.offset, m.path = limit+1, nil

	for d := 0; d <= limit; d++ {
		m.path = append(m.path, slices.Clone(v))
		for k := -d; k <= d; k += 2 {
			var x int
			// Down when the diagonal above has got further, which is an insertion;
			// right otherwise, which is a deletion. The edges have only one choice.
			if k == -d || (k != d && v[m.offset+k-1] < v[m.offset+k+1]) {
				x = v[m.offset+k+1]
			} else {
				x = v[m.offset+k-1] + 1
			}
			y := x - k
			for x < n && y < size && m.before[x] == m.after[y] {
				x, y = x+1, y+1
			}
			v[m.offset+k] = x
			if x >= n && y >= size {
				return true
			}
		}
	}
	return false
}

func (m *myers) walk() Script {
	x, y := len(m.before), len(m.after)
	var reversed Script

	// path[d] is where every diagonal had got to before the d-th edit was made, so a
	// point reached by the d-th edit came from there.
	for d := len(m.path) - 1; d > 0; d-- {
		v := m.path[d]
		k := x - y
		prevK := k - 1
		if k == -d || (k != d && v[m.offset+k-1] < v[m.offset+k+1]) {
			prevK = k + 1
		}
		prevX := v[m.offset+prevK]
		prevY := prevX - prevK

		for x > prevX && y > prevY {
			x, y = x-1, y-1
			reversed = append(reversed, m.kept(x, y))
		}
		switch {
		case x > prevX:
			x--
			reversed = append(reversed, Line{Kind: Removed, Text: m.before[x], Old: m.from + x + 1})
		case y > prevY:
			y--
			reversed = append(reversed, Line{Kind: Added, Text: m.after[y], New: m.from + y + 1})
		}
	}
	for x > 0 && y > 0 {
		x, y = x-1, y-1
		reversed = append(reversed, m.kept(x, y))
	}

	slices.Reverse(reversed)
	return reversed
}

func (m *myers) kept(x, y int) Line {
	return Line{Text: m.before[x], Old: m.from + x + 1, New: m.from + y + 1}
}
