package text

import "strings"

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
