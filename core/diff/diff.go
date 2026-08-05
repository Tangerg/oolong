// Package diff says what changed between two texts, line by line.
//
// It is here rather than beside the thing that draws a diff for the same reason
// [github.com/Tangerg/oolong/core/fuzzy] is: what changed is a fact about two strings
// and has nothing to do with a terminal. Something that only knew how to draw a diff
// would leave every caller to work out what the diff was, and there is one answer.
//
// Lines and not characters. A terminal shows a diff a line at a time, a reader reads it
// a line at a time, and the line is the unit every tool that produces one already
// speaks in.
package diff

import "slices"

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

// maxEdits bounds how much work a single comparison may do.
//
// The algorithm's cost grows with the number of differing lines, not with the size of
// the texts, so two nearly identical files are cheap however long they are. Past this
// many differences the two texts have nothing much in common and the answer is the
// honest one: what was there is gone and what is there is new. Nobody reads a
// five-hundred-line diff line by line anyway.
const maxEdits = 512

// Lines is what changed between two texts.
//
// The result reads top to bottom as the change itself: every line of both texts appears
// once, in an order in which the removals and the additions of the same passage sit
// together.
func Lines(before, after []string) []Line {
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

	out := make([]Line, 0, len(before)+len(after))
	for i := range head {
		out = append(out, Line{Text: before[i], Old: i + 1, New: i + 1})
	}
	out = append(out, middle(before[head:len(before)-tail], after[head:len(after)-tail], head)...)
	for i := range tail {
		at := len(before) - tail + i
		out = append(out, Line{Text: before[at], Old: at + 1, New: len(after) - tail + i + 1})
	}
	return out
}

// middle diffs the parts that are not shared ends, numbering from an offset.
func middle(before, after []string, from int) []Line {
	switch {
	case len(before) == 0 && len(after) == 0:
		return nil
	case len(before) == 0 || len(after) == 0:
		return replaced(before, after, from)
	}
	path, ok := trace(before, after)
	if !ok {
		return replaced(before, after, from)
	}
	return walk(path, before, after, from)
}

// replaced is the answer when nothing useful is shared: all of one, then all of the
// other.
func replaced(before, after []string, from int) []Line {
	out := make([]Line, 0, len(before)+len(after))
	for i, line := range before {
		out = append(out, Line{Kind: Removed, Text: line, Old: from + i + 1})
	}
	for i, line := range after {
		out = append(out, Line{Kind: Added, Text: line, New: from + i + 1})
	}
	return out
}

// trace runs Myers' algorithm, keeping the furthest point reached on each diagonal
// after every edit, and reports false when the two texts differ by more than
// [maxEdits].
//
// The furthest points are what the answer is reconstructed from: an edit script is a
// path through a grid, and the path is walked back out of these afterwards. Keeping
// them is the whole memory cost, which is why the number of edits is capped rather
// than the size of the texts.
func trace(before, after []string) ([][]int, bool) {
	n, m := len(before), len(after)
	limit := min(n+m, maxEdits)
	// v holds the furthest x reached on each diagonal k, offset so that k = 0 sits in
	// the middle.
	v := make([]int, 2*limit+3)
	offset := limit + 1
	var path [][]int

	for d := 0; d <= limit; d++ {
		path = append(path, slices.Clone(v))
		for k := -d; k <= d; k += 2 {
			var x int
			// Down when the diagonal above has got further, which is an insertion;
			// right otherwise, which is a deletion. The edges have only one choice.
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k
			for x < n && y < m && before[x] == after[y] {
				x, y = x+1, y+1
			}
			v[offset+k] = x
			if x >= n && y >= m {
				return path, true
			}
		}
	}
	return nil, false
}

// walk reads the edit script back out of the trace, from the end to the beginning, and
// turns it into lines in reading order.
func walk(path [][]int, before, after []string, from int) []Line {
	// The offset the trace was built with: each snapshot is 2*limit+3 long, centred on
	// the diagonal through the origin.
	offset := (len(path[0]) - 1) / 2
	x, y := len(before), len(after)
	var reversed []Line

	// path[d] is where every diagonal had got to before the d-th edit was made, so a
	// point reached by the d-th edit came from there.
	for d := len(path) - 1; d > 0; d-- {
		v := path[d]
		k := x - y
		var prevK int
		if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := v[offset+prevK]
		prevY := prevX - prevK

		// The run of matching lines this step ended with.
		for x > prevX && y > prevY {
			x, y = x-1, y-1
			reversed = append(reversed, Line{
				Text: before[x], Old: from + x + 1, New: from + y + 1,
			})
		}
		switch {
		case x > prevX:
			x--
			reversed = append(reversed, Line{Kind: Removed, Text: before[x], Old: from + x + 1})
		case y > prevY:
			y--
			reversed = append(reversed, Line{Kind: Added, Text: after[y], New: from + y + 1})
		}
	}
	// Whatever the first step matched, before any edit was made.
	for x > 0 && y > 0 {
		x, y = x-1, y-1
		reversed = append(reversed, Line{Text: before[x], Old: from + x + 1, New: from + y + 1})
	}

	slices.Reverse(reversed)
	return reversed
}

// Hunk is a run of a diff worth showing: what changed, and a few lines either side of
// it so a reader can see where they are.
type Hunk struct {
	Lines []Line
	// Old and New are where the hunk begins in each text, counting from one.
	Old, New int
}

// Hunks is the changed parts of a diff with context lines around them, and everything
// else left out.
//
// A file that changed in two places is two hundred lines of which six matter, and a
// view that shows all two hundred is a view nobody reads. Overlapping runs of context
// are one hunk rather than two, because a gap of one unchanged line is not a gap worth
// drawing a break across.
//
// A context of zero is the changed lines alone. A diff with nothing changed in it has
// no hunks at all, which is how "these are the same" is said.
func Hunks(lines []Line, context int) []Hunk {
	context = max(context, 0)
	keep := make([]bool, len(lines))
	changed := false
	for i, line := range lines {
		if line.Kind == Context {
			continue
		}
		changed = true
		for j := max(i-context, 0); j <= min(i+context, len(lines)-1); j++ {
			keep[j] = true
		}
	}
	if !changed {
		return nil
	}

	var out []Hunk
	for i := 0; i < len(lines); i++ {
		if !keep[i] {
			continue
		}
		start := i
		for i < len(lines) && keep[i] {
			i++
		}
		out = append(out, hunk(lines[start:i]))
	}
	return out
}

// hunk is one run of lines with its starting numbers worked out.
func hunk(lines []Line) Hunk {
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
