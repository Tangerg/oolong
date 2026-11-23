package text

import (
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
)

type Wrapped struct {
	Line Line
	// Joined marks a row that continues the line above it rather than starting a
	// line of its own. Anything rejoining rows — copying a selection, say — needs
	// to know which line breaks were the text's and which were the width's.
	Joined bool
	// From and To retain the source byte interval so selection and search can
	// map through wrapping. They describe provenance: tabs may be expanded,
	// controls dropped, and break whitespace excluded from the displayed row.
	From, To int
	// Gap is the text the break before this row consumed, which rejoining it to the
	// row above has to put back.
	//
	// It is content, and it is here because the wrap is the only thing that knows it.
	// The bytes between one row's end and the next row's start are not it: that is
	// where everything the wrap decided not to lay out ends up. See [dropped].
	Gap string
}

func (w Wrapped) Width() int { return w.Line.Width() }

func (w Wrapped) Draw(v grid.View, x, y int) int { return w.Line.Draw(v, x, y) }

// Wrap breaks the line into rows of at most width columns.
//
// Breaks are preferred at spaces, and a word longer than the width is broken
// between grapheme clusters instead. The run of spaces at a break is consumed:
// it hangs off neither the end of one row nor the start of the next. Styles
// survive every break.
//
// A width of zero or less returns the line whole: a caller with no width to lay out
// in is better served by text it can measure than by text silently thrown away.
// Whole, but still laid out, because a row is what a terminal would be shown.
func (l Line) Wrap(width int) []Wrapped {
	units := l.units()
	if width <= 0 || len(units) == 0 {
		return []Wrapped{{Line: line(units), To: l.ByteLen()}}
	}
	rowCapacity := min(len(units), width)
	w := wrapper{
		width: width,
		rows:  make([]Wrapped, 0, len(units)/width+1),
		row:   make([]unit, 0, rowCapacity),
	}
	for i, n := 0, len(units); i < n; {
		if units[i].space {
			w.hold(i, units[i])
			i++
			continue
		}
		end, wordWidth := i, 0
		for end < n && !units[end].space {
			wordWidth = layout.Sum(wordWidth, units[end].width)
			end++
		}
		i = w.word(units, i, end, wordWidth)
	}
	return w.finish(units)
}

// wrapper accumulates rows. Held spaces are kept out of the current row until
// something follows them, which is what lets a break consume them.
type wrapper struct {
	width int
	rows  []Wrapped

	row      []unit
	rowWidth int
	held     heldRun
	// rowGap is what was consumed before the row now accumulating; nextGap is what
	// has been consumed since it started, and belongs to whatever row follows the
	// break.
	rowGap  string
	nextGap string
}

// heldRun is the units [from, to) held after the row, and their width.
type heldRun struct{ from, to, width int }

func (r heldRun) empty() bool { return r.to == r.from }

func (r heldRun) of(units []unit) []unit { return units[r.from:r.to] }

func (w *wrapper) hold(at int, u unit) {
	if w.held.empty() {
		w.held.from = at
	}
	w.held.to = at + 1
	w.held.width = layout.Sum(w.held.width, u.width)
}

func (w *wrapper) place(u unit) {
	w.row = append(w.row, u)
	w.rowWidth = layout.Sum(w.rowWidth, u.width)
}

func (w *wrapper) takeHeld(units []unit) {
	w.row = append(w.row, w.held.of(units)...)
	w.rowWidth = layout.Sum(w.rowWidth, w.held.width)
	w.clearHeld()
}

// discardHeld gives up the spaces a break consumed, keeping them for the row they
// now precede rather than the one they followed.
func (w *wrapper) discardHeld(units []unit) {
	var consumed strings.Builder
	for _, u := range w.held.of(units) {
		consumed.WriteString(u.cluster)
	}
	if len(w.row) == 0 {
		w.rowGap += consumed.String()
	} else {
		w.nextGap += consumed.String()
	}
	w.clearHeld()
}

// atLineStart reports that nothing of the line has been laid out yet. Spaces held
// there are indentation, not the separator of a break, so they must not become a
// row's Gap: a gap is reversible only on a row that continues the one above it.
func (w *wrapper) atLineStart() bool { return len(w.rows) == 0 && len(w.row) == 0 }

// layHeld places held spaces as content, breaking where they reach the width.
func (w *wrapper) layHeld(units []unit) {
	for _, u := range w.held.of(units) {
		if len(w.row) > 0 && layout.Sum(w.rowWidth, u.width) > w.width {
			w.breakRow()
		}
		w.place(u)
	}
	w.clearHeld()
}

func (w *wrapper) clearHeld() { w.held = heldRun{} }

func (w *wrapper) breakRow() {
	row := Wrapped{Line: line(w.row), Joined: len(w.rows) > 0, Gap: w.rowGap}
	if n := len(w.row); n > 0 {
		first, last := w.row[0], w.row[n-1]
		row.From, row.To = first.at, last.at+last.size
	}
	w.rows = append(w.rows, row)
	w.row = w.row[:0]
	w.rowWidth = 0
	w.rowGap, w.nextGap = w.nextGap, ""
}

func (w *wrapper) word(units []unit, from, to, wordWidth int) int {
	switch {
	case layout.Sum(w.rowWidth, w.held.width, wordWidth) <= w.width:
		w.takeHeld(units)
		for ; from < to; from++ {
			w.place(units[from])
		}
	case wordWidth <= w.width:
		if w.atLineStart() {
			w.layHeld(units)
			if layout.Sum(w.rowWidth, wordWidth) > w.width {
				w.breakRow()
			}
		} else {
			w.discardHeld(units)
			if len(w.row) > 0 {
				w.breakRow()
			}
		}
		for ; from < to; from++ {
			w.place(units[from])
		}
	default:
		from = w.hardBreak(units, from, to)
	}
	return from
}

func (w *wrapper) hardBreak(units []unit, from, to int) int {
	if !w.held.empty() {
		switch {
		case layout.Sum(w.rowWidth, w.held.width, units[from].width) <= w.width:
			w.takeHeld(units)
		case w.atLineStart():
			w.layHeld(units)
		default:
			w.discardHeld(units)
			if len(w.row) > 0 {
				w.breakRow()
			}
		}
	}
	for from < to {
		u := units[from]
		switch {
		case layout.Sum(w.rowWidth, u.width) <= w.width:
			w.place(u)
			from++
		case len(w.row) == 0:
			// A display atom wider than the whole row gets a row to itself and
			// overflows it, because the alternative is splitting or dropping it.
			w.place(u)
			from++
			w.breakRow()
		default:
			// Leaves the row short rather than splitting a display atom.
			w.breakRow()
		}
	}
	return from
}

func (w *wrapper) finish(units []unit) []Wrapped {
	for _, u := range w.held.of(units) {
		if layout.Sum(w.rowWidth, u.width) > w.width {
			break
		}
		w.place(u)
	}
	if len(w.row) > 0 {
		w.breakRow()
	}
	if len(w.rows) == 0 {
		w.rows = append(w.rows, Wrapped{})
	}
	return w.rows
}
