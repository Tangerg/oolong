package text

import (
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
)

// Projector supplies a drawable's text as safe rendered rows with soft-wrap
// metadata at a width. It does not promise a map back to a format's source.
type Projector interface{ Rows(width int) []Row }

// Row is the meaningful text of one visual row and where that text begins.
//
// Content excludes gutters, markers and other decoration. Offset keeps its columns
// aligned with the rendered row without putting decoration into copied or searched
// content. SourceLine is the one-based logical source line, when the producer has one;
// zero means unspecified. Joined and Gap make a width-induced break reversible: a
// copy can rejoin a wrapped paragraph while preserving a consumed space and not
// inventing one inside a long word.
type Row struct {
	Content    Line
	Offset     int
	SourceLine int
	Joined     bool
	Gap        string
}

func CloneRows(rows []Row) []Row {
	if rows == nil {
		return nil
	}
	out := make([]Row, len(rows))
	for i, row := range rows {
		row.Content = row.Content.Clone()
		row.Gap = strings.Clone(row.Gap)
		out[i] = row
	}
	return out
}

// Separator is what goes between this row and the one above it in copied or searched
// logical text.
func (r Row) Separator() string {
	if r.Joined {
		return r.Gap
	}
	return "\n"
}

// Display is the row as it was drawn: its content and gap without what a terminal
// shows nothing for. Copied and searched text are made of displayed rows, so a
// control a producer left in cannot become text nobody saw.
func (r Row) Display() Row {
	r.Content = r.Content.Display()
	r.Gap = Line{{Text: r.Gap}}.Display().String()
	return r
}

// JoinRows is the logical text of rows as drawn, which is what copying them all
// takes: each displayed row after its [Row.Separator], so a soft wrap rejoins and a
// hard break stays a line break.
func JoinRows(rows []Row) string {
	var b strings.Builder
	for i, row := range rows {
		row = row.Display()
		if i > 0 {
			b.WriteString(row.Separator())
		}
		b.WriteString(row.Content.String())
	}
	return b.String()
}

// ProjectedRows lays d's text projection at width over the height rows d draws: its
// own rows where it projects text, cut to height, and blank rows everywhere else. A
// composition placing drawables one under another reports these so its rows stay
// aligned with what it draws, including around content that projects no text.
func ProjectedRows(d grid.Drawable, width, height int) []Row {
	if height <= 0 {
		return nil
	}
	rows := make([]Row, height)
	if projector, ok := d.(Projector); ok {
		copy(rows, CloneRows(projector.Rows(width)))
	}
	return rows
}
