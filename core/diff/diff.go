// Package diff says what changed between two texts, line by line.
//
// It computes a line-oriented edit script without deciding how that script is stored,
// rendered or applied. Lines are the unit because line-oriented producers and formats
// can consume the result without reconstructing their boundaries.
package diff

import (
	"slices"
	"strings"
)

// Kind is what happened to a line.
type Kind uint8

// What happened to a line. Context is the zero value, because a line that was left
// alone is what most lines are.
const (
	Context Kind = iota
	Added
	Removed
)

// String names the kind the way a diff marks it.
func (k Kind) String() string {
	switch k {
	case Added:
		return "+"
	case Removed:
		return "-"
	default:
		return " "
	}
}

// Line is one line of a diff.
type Line struct {
	Kind Kind
	Text string
	// Old and New are the line's number in each text, counting from one, and zero
	// where the line is not in that text. A reader looking for what to open and where
	// needs the number in the text that still exists; a reader reading a change needs
	// both.
	Old, New int
}

// String writes the line the way a diff writes one: its mark, then its text.
func (l Line) String() string { return l.Kind.String() + l.Text }

// Script is a change to a text, as the lines that make it.
//
// It reads top to bottom as the change itself: every line of both texts appears once,
// in an order in which the removals and the additions of the same passage sit together.
type Script []Line

// Between is what changed between two texts.
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

// Same reports whether the two texts were the same, which is a script with nothing
// changed in it.
func (s Script) Same() bool {
	return !slices.ContainsFunc(s, func(line Line) bool { return line.Kind != Context })
}

// String writes the script the way a diff is written: one line each, marked.
func (s Script) String() string {
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
func (s Script) Hunks(context int) []Hunk {
	context = max(context, 0)
	keep := make([]bool, len(s))
	changed := false
	for i, line := range s {
		if line.Kind == Context {
			continue
		}
		changed = true
		for j := max(i-context, 0); j <= min(i+context, len(s)-1); j++ {
			keep[j] = true
		}
	}
	if !changed {
		return nil
	}

	var out []Hunk
	for i := 0; i < len(s); i++ {
		if !keep[i] {
			continue
		}
		start := i
		for i < len(s) && keep[i] {
			i++
		}
		out = append(out, hunkOf(s[start:i]))
	}
	return out
}

// Hunk is a run of a script worth showing: what changed, and a few lines either side of
// it so a reader can see where they are.
type Hunk struct {
	Lines Script
	// Old and New are where the hunk begins in each text, counting from one.
	Old, New int
}

// String writes the hunk the way a diff writes one.
func (h Hunk) String() string { return h.Lines.String() }

// hunkOf is one run of lines with its starting numbers worked out.
func hunkOf(lines Script) Hunk {
	h := Hunk{Lines: lines}
	for _, line := range lines {
		if h.Old == 0 && line.Old > 0 {
			h.Old = line.Old
		}
		if h.New == 0 && line.New > 0 {
			h.New = line.New
		}
	}
	return h
}

// maxEdits bounds how much work one comparison may do.
//
// The algorithm's cost grows with the number of differing lines, not with the size of
// the texts, so two nearly identical files are cheap however long they are. Past this
// many differences the two texts have nothing much in common and the answer is the
// honest one: what was there is gone and what is there is new. Nobody reads a
// five-hundred-line diff line by line anyway.
const maxEdits = 512

// myers is one comparison, from the two texts to the lines that describe the change.
//
// It is a type rather than four functions passing the same three arguments between
// them, because the algorithm is a walk forward and then a walk back over one piece of
// state: how far each diagonal had got after every edit. That state is the whole of it,
// and naming it is what makes the two walks readable as the two halves of one thing.
type myers struct {
	// before and after are the parts of the two texts that are not shared ends.
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

// script is what changed, by whichever route can answer.
func (m *myers) script() Script {
	switch {
	case len(m.before) == 0 && len(m.after) == 0:
		return nil
	case len(m.before) == 0 || len(m.after) == 0 || !m.trace():
		return m.replaced()
	}
	return m.walk()
}

// replaced is the answer when nothing useful is shared: all of one, then all of the
// other.
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

// trace walks forward, keeping the furthest point reached on each diagonal after every
// edit, and reports false when the two texts differ by more than [maxEdits].
func (m *myers) trace() bool {
	n, size := len(m.before), len(m.after)
	limit := min(n+size, maxEdits)
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

// walk reads the edit script back out of the trace, from the end to the beginning, and
// turns it into lines in reading order.
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

		// The run of matching lines this step ended with.
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
	// Whatever the first step matched, before any edit was made.
	for x > 0 && y > 0 {
		x, y = x-1, y-1
		reversed = append(reversed, m.kept(x, y))
	}

	slices.Reverse(reversed)
	return reversed
}

// kept is a line that both texts have, numbered in each of them.
func (m *myers) kept(x, y int) Line {
	return Line{Text: m.before[x], Old: m.from + x + 1, New: m.from + y + 1}
}
