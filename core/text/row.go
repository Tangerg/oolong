package text

// Row is the meaningful text of one visual row and where that text begins.
//
// Text excludes gutters, markers and other decoration. Offset keeps its columns
// aligned with the rendered row without putting decoration into copied or searched
// content. Joined and Gap make a width-induced break reversible: a copy can rejoin a
// wrapped paragraph while preserving a consumed space and not inventing one inside a
// long word.
type Row struct {
	Text   string
	Offset int
	Joined bool
	Gap    string
}

// Separator is what goes between this row and the one above it in copied or searched
// logical text.
func (r Row) Separator() string {
	if r.Joined {
		return r.Gap
	}
	return "\n"
}
