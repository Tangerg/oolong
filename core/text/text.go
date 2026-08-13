// Package text lays styled text out in terminal columns: measuring it, wrapping
// it, truncating it, and drawing it onto a [grid.View].
//
// Everything here counts columns rather than bytes or runes. A CJK or emoji
// cluster is two columns wide and is never split; a combining mark is none. Text
// that is measured one way and drawn another is the source of every misaligned
// terminal UI, so measuring and drawing live in the same place and agree by
// construction.
//
// # Text that arrives already styled
//
// [Decoder] is the other direction: the output of a command an interface ran,
// which comes with the escape sequences that coloured it, read back into the same
// [Span]s everything here lays out. It is here rather than beside the terminal
// because what it produces is text — the sequences are how the styling was
// spelled, and this is the package that knows what styled text is.
package text

import (
	"iter"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
)

// TabStop is how far apart tab stops are.
//
// Eight, because that is where a terminal would have put them: the output being
// rendered was usually formatted by a program writing to a terminal, and lining
// its columns up means agreeing with the assumption it made.
const TabStop = 8

// Span is a run of text sharing one style, and — where it points at something —
// one address.
type Span struct {
	Text  string
	Style grid.Style
	// Link is where the run points: a URL, a file, whatever a terminal will open.
	// Empty is text that points nowhere, which is nearly all text.
	//
	// It is carried here rather than stamped onto cells afterwards because by then
	// the columns are gone: a line is wrapped, truncated and drawn wherever it fits,
	// and something holding byte offsets into the text it was made from cannot say
	// which cells the third word ended up on. A span survives all three, so the
	// address survives with it — see [Line.Draw], which is where it reaches the
	// cells, and [github.com/Tangerg/oolong/core/grid.Cell.Link], which is what a
	// terminal is told.
	Link string
}

// Line is one logical line of styled text — logical in that it has no width yet.
// Wrapping turns it into however many rows it needs.
type Line []Span

// Clone returns an independently owned copy of the line.
//
// It copies the span storage and detaches text and link strings from their source
// allocations. The latter matters at long-lived ownership boundaries: a short span
// sliced from a command's output must not keep the command's complete output alive.
func (l Line) Clone() Line {
	if len(l) == 0 {
		return nil
	}
	out := make(Line, len(l))
	for i, span := range l {
		span.Text = strings.Clone(span.Text)
		span.Link = strings.Clone(span.Link)
		out[i] = span
	}
	return out
}

// CloneLines returns an independently owned copy of lines and every line in it.
// It is the collection counterpart to [Line.Clone]; a caller taking ownership of a
// rendered document should not have to rebuild that deep-copy boundary itself.
func CloneLines(lines []Line) []Line {
	if len(lines) == 0 {
		return nil
	}
	out := make([]Line, len(lines))
	for i, line := range lines {
		out[i] = line.Clone()
	}
	return out
}

// Of is the one-span line for a piece of plain styled text.
func Of(s string, style grid.Style) Line {
	if s == "" {
		return nil
	}
	return Line{{Text: s, Style: style}}
}

// String is the line's text with the styling dropped.
func (l Line) String() string {
	if len(l) == 1 {
		return l[0].Text
	}
	var b strings.Builder
	for _, s := range l {
		b.WriteString(s.Text)
	}
	return b.String()
}

// Width is how many columns the line would occupy unwrapped, with tabs expanded.
func (l Line) Width() int {
	col := 0
	for _, s := range l {
		col = advance(s.Text, col)
	}
	return col
}

// Draw writes the line onto v at (x, y) and returns how many columns it advanced.
// Tabs are expanded from the line's own start, not from the view's, so a line
// drawn at an indent keeps the column relationships it was written with.
//
// A span that points somewhere is stamped onto the columns it took, so the text a
// terminal shows is the text a terminal will open — and a link that a wrap broke in
// two is stamped on both halves, which is how one hyperlink covers two rows.
func (l Line) Draw(v grid.View, x, y int) int {
	col := 0
	for _, s := range l {
		for _, piece := range expand(s.Text, &col) {
			at := layout.Translate(x, piece.at)
			width := v.Text(at, y, piece.text, s.Style)
			if s.Link != "" {
				v.Link(at, y, width, s.Link)
			}
		}
	}
	return col
}

// Wrapped is one physical row produced by wrapping a [Line].
type Wrapped struct {
	Line Line
	// Joined marks a row that continues the line above it rather than starting a
	// line of its own. Anything rejoining rows — copying a selection, say — needs
	// to know which line breaks were the text's and which were the width's.
	Joined bool
	// From and To are the byte range of the line this row came from, as [Line.String]
	// numbers it. They make the wrap invertible: anything that found something in the
	// text before it was wrapped — a URL, a search match, the ends of a selection —
	// can work out which rows it landed on and where along them.
	//
	// The range is not the same text as the row: the spaces a break consumed are
	// outside it at either end, a tab inside it became the spaces it stands for, and a
	// control character inside it was dropped. It is provenance, not content.
	From, To int
}

// Width is how many columns the row occupies.
func (w Wrapped) Width() int { return w.Line.Width() }

// Draw writes the row onto v at (x, y).
func (w Wrapped) Draw(v grid.View, x, y int) int { return w.Line.Draw(v, x, y) }

// Wrap breaks the line into rows of at most width columns.
//
// Breaks are preferred at spaces, and a word longer than the width is broken
// between grapheme clusters instead. The run of spaces at a break is consumed:
// it hangs off neither the end of one row nor the start of the next. Styles
// survive every break.
//
// A width of zero or less returns the line whole: a caller with no width to lay
// out in is better served by text it can measure than by text silently thrown
// away.
func (l Line) Wrap(width int) []Wrapped {
	if width <= 0 {
		return []Wrapped{{Line: l, To: l.bytes()}}
	}
	units := l.units()
	if len(units) == 0 {
		return []Wrapped{{Line: l, To: l.bytes()}}
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
		// A word is a maximal run of clusters with no break opportunity in it.
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
	heldFrom int
	heldTo   int
	hasHeld  bool
	heldW    int
}

func (w *wrapper) hold(at int, u unit) {
	if !w.hasHeld {
		w.heldFrom = at
		w.hasHeld = true
	}
	w.heldTo = at + 1
	w.heldW = layout.Sum(w.heldW, u.width)
}

func (w *wrapper) place(u unit) {
	w.row = append(w.row, u)
	w.rowWidth = layout.Sum(w.rowWidth, u.width)
}

func (w *wrapper) takeHeld(units []unit) {
	w.row = append(w.row, units[w.heldFrom:w.heldTo]...)
	w.rowWidth = layout.Sum(w.rowWidth, w.heldW)
	w.dropHeld()
}

func (w *wrapper) dropHeld() {
	w.heldFrom, w.heldTo, w.hasHeld = 0, 0, false
	w.heldW = 0
}

func (w *wrapper) breakRow() {
	row := Wrapped{Line: line(w.row), Joined: len(w.rows) > 0}
	if n := len(w.row); n > 0 {
		first, last := w.row[0], w.row[n-1]
		row.From, row.To = first.at, last.at+last.size
	}
	w.rows = append(w.rows, row)
	w.row = w.row[:0]
	w.rowWidth = 0
}

// word places units[from:to] and returns the index to continue from.
func (w *wrapper) word(units []unit, from, to, wordWidth int) int {
	switch {
	case layout.Sum(w.rowWidth, w.heldW, wordWidth) <= w.width:
		// Fits after the spaces that preceded it.
		w.takeHeld(units)
		for ; from < to; from++ {
			w.place(units[from])
		}
	case wordWidth <= w.width:
		// Fits on a row of its own: break before it and drop the spaces.
		w.dropHeld()
		if len(w.row) > 0 {
			w.breakRow()
		}
		for ; from < to; from++ {
			w.place(units[from])
		}
	default:
		from = w.hardBreak(units, from, to)
	}
	return from
}

// hardBreak splits a word that is wider than a whole row.
func (w *wrapper) hardBreak(units []unit, from, to int) int {
	if w.hasHeld {
		if layout.Sum(w.rowWidth, w.heldW, units[from].width) <= w.width {
			w.takeHeld(units)
		} else {
			w.dropHeld()
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
			// A cluster wider than the whole row — a double-width one where the
			// width is one. It gets a row to itself and overflows it, because the
			// alternative is dropping it forever.
			w.place(u)
			from++
			w.breakRow()
		default:
			// Leaves the row a column short rather than splitting a wide cluster.
			w.breakRow()
		}
	}
	return from
}

// finish takes whatever trailing spaces fit and closes the last row.
func (w *wrapper) finish(units []unit) []Wrapped {
	for _, u := range units[w.heldFrom:w.heldTo] {
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

// Truncate cuts the line to at most width columns, ending it with ellipsis when
// anything was cut. The ellipsis takes the style of the last text that survived,
// so it reads as part of the sentence it is ending.
//
// The result can fall a column short of width: a cut never splits a wide cluster.
func (l Line) Truncate(width int, ellipsis string) Line {
	if width <= 0 {
		return nil
	}
	if l.Width() <= width {
		return l
	}
	if Width(ellipsis) > width {
		ellipsis = prefix(ellipsis, width)
	}
	budget := width - Width(ellipsis)

	units := l.units()
	kept := make([]unit, 0, len(units))
	used := 0
	style := grid.Style{}
	for _, u := range units {
		next := layout.Sum(used, u.width)
		if next > budget {
			break
		}
		kept = append(kept, u)
		used = next
		style = u.style()
	}
	out := line(kept)
	if ellipsis == "" {
		return out
	}
	// The ellipsis takes the style of what survived and none of its address: it
	// stands for the text that was cut, and a hyperlink over "…" opens something the
	// reader was never shown.
	if n := len(out); n > 0 && out[n-1].Style == style && out[n-1].Link == "" {
		out[n-1].Text += ellipsis
		return out
	}
	return append(out, Span{Text: ellipsis, Style: style})
}

// Width is how many columns s would occupy, with tabs expanded from column zero.
func Width(s string) int { return advance(s, 0) }

// Truncate cuts plain text to at most width columns, ending it with ellipsis
// when anything was cut.
func Truncate(s string, width int, ellipsis string) string {
	if width <= 0 {
		return ""
	}
	if Width(s) <= width {
		return s
	}
	if Width(ellipsis) > width {
		return prefix(ellipsis, width)
	}
	return prefix(s, width-Width(ellipsis)) + ellipsis
}

// unit is one grapheme cluster with everything wrapping needs to know about it.
type unit struct {
	cluster string
	// source owns appearance and link identity. Carrying one pointer per cluster
	// instead of copying a Style and string header keeps wrapping proportional to
	// the text model rather than to the size of its presentation vocabulary.
	source *Span
	// linked is false only for a tab expansion. A tab advances through empty cells;
	// unlike an ordinary space, those cells are not painted and therefore cannot be
	// part of a terminal hyperlink.
	linked bool
	width  int
	// space marks a break opportunity. A tab is one, and is also the reason a
	// unit's width is not derivable from its cluster alone.
	space bool
	// at is where this cluster started in the line's text, so that a row can say
	// which part of the line it came from. A tab's expansion carries the tab's own
	// offset on every space it became.
	at int
	// size is how many bytes the cluster took, which is not len(cluster) for a tab
	// that became a space.
	size int
}

// units is the line as the things that occupy columns, with tabs expanded against the
// running column.
func (l Line) units() []unit {
	// A grapheme never contains more units than runes, except that one tab expands
	// to at most TabStop spaces. This exact upper bound for ordinary text avoids the
	// repeated growth and copying of the comparatively rich unit value. It remains
	// an upper bound for combining sequences and controls, which merely leave spare
	// capacity for this call and never escape it.
	capacity := 0
	for _, span := range l {
		for _, r := range span.Text {
			capacity = layout.Sum(capacity, 1)
			if r == '\t' {
				capacity = layout.Sum(capacity, TabStop-1)
			}
		}
	}
	units := make([]unit, 0, capacity)
	col, at := 0, 0
	for i := range l {
		s := &l[i]
		g := uniseg.NewGraphemes(s.Text)
		for g.Next() {
			cluster := g.Str()
			switch {
			case cluster == "\t":
				n := TabStop - col%TabStop
				for range n {
					units = append(units, unit{
						cluster: " ", source: s, width: 1, space: true,
						at: at, size: len(cluster),
					})
				}
				col = layout.Sum(col, n)
			case dropped(cluster):
				// A control character has no width to lay out and no business
				// reaching a cell.
			case cluster == " ":
				units = append(units, unit{
					cluster: " ", source: s, linked: true, width: 1, space: true,
					at: at, size: len(cluster),
				})
				col++
			default:
				w := clusterWidth(cluster)
				units = append(units, unit{
					cluster: cluster, source: s, linked: true, width: w,
					at: at, size: len(cluster),
				})
				col = layout.Sum(col, w)
			}
			at += len(cluster)
		}
	}
	return units
}

// bytes is how many bytes the line's text takes, without building it.
func (l Line) bytes() int {
	n := 0
	for _, s := range l {
		n += len(s.Text)
	}
	return n
}

// line rebuilds a line from units, merging neighbours that share a style and point
// at the same thing.
func line(units []unit) Line {
	if len(units) == 0 {
		return Line{}
	}
	// Every non-empty row has one style run. Let uncommon additional runs grow the
	// slice instead of reserving one Span for every grapheme in ordinary plain text.
	out := make(Line, 0, 1)
	for first := 0; first < len(units); {
		last := first + 1
		bytes := len(units[first].cluster)
		for last < len(units) && units[last].sameRun(units[first]) {
			bytes += len(units[last].cluster)
			last++
		}
		var text strings.Builder
		text.Grow(bytes)
		for _, u := range units[first:last] {
			text.WriteString(u.cluster)
		}
		out = append(out, Span{
			Text:  text.String(),
			Style: units[first].style(),
			Link:  units[first].link(),
		})
		first = last
	}
	return out
}

func (u unit) style() grid.Style { return u.source.Style }

func (u unit) link() string {
	if !u.linked {
		return ""
	}
	return u.source.Link
}

func (u unit) sameRun(other unit) bool {
	return u.style() == other.style() && u.link() == other.link()
}

// piece is a run of text and the column it starts at, after tab expansion.
type piece struct {
	at   int
	text string
}

// expand splits s into drawable pieces, turning tabs into the gaps they stand
// for and advancing col past everything it produced.
func expand(s string, col *int) []piece {
	if !strings.ContainsAny(s, "\t") {
		p := []piece{{at: *col, text: s}}
		*col = advance(s, *col)
		return p
	}
	var pieces []piece
	var run strings.Builder
	start := *col
	flush := func() {
		if run.Len() == 0 {
			return
		}
		pieces = append(pieces, piece{at: start, text: run.String()})
		run.Reset()
	}
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		cluster := g.Str()
		if cluster == "\t" {
			flush()
			*col = layout.Sum(*col, TabStop-*col%TabStop)
			start = *col
			continue
		}
		if run.Len() == 0 {
			start = *col
		}
		run.WriteString(cluster)
		*col = layout.Sum(*col, clusterWidth(cluster))
	}
	flush()
	return pieces
}

// advance is where col ends up after s, with tabs expanded.
func advance(s string, col int) int {
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		if cluster := g.Str(); cluster == "\t" {
			col = layout.Sum(col, TabStop-col%TabStop)
		} else {
			col = layout.Sum(col, clusterWidth(cluster))
		}
	}
	return col
}

// dropped reports whether a cluster is discarded rather than laid out. Measuring
// has to agree with drawing about this, or a line's reported width will not be the
// width it takes.
func dropped(cluster string) bool { return cluster != "\t" && isControl(cluster) }

// prefix is the longest prefix of s, cut between clusters, that fits in budget.
func prefix(s string, budget int) string {
	col, end := 0, 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		w := clusterWidth(g.Str())
		if layout.Sum(col, w) > budget {
			break
		}
		col = layout.Sum(col, w)
		_, end = g.Positions()
	}
	return s[:end]
}

// clusterWidth is a cluster's column count. It defers to the grid, so text is
// measured here exactly as it will be drawn there.
func clusterWidth(cluster string) int { return grid.ClusterWidth(cluster) }

// isControl reports whether a cluster is a control character. A tab is one, and
// is handled before this is asked; the rest have no width and no business
// reaching a cell.
func isControl(cluster string) bool {
	return cluster != "" && (cluster[0] < 0x20 || cluster[0] == 0x7f)
}

// Clusters iterates the grapheme clusters of s with the byte offset each starts at.
//
// It is what anything holding a cursor into text needs. A cursor cannot live on a
// rune boundary: a letter and the accent that modifies it are two runes and one
// thing on screen, and a cursor between them has no position a terminal could show.
func Clusters(s string) iter.Seq2[int, string] {
	return func(yield func(int, string) bool) {
		at, state := 0, -1
		var cluster string
		for len(s) > 0 {
			cluster, s, _, state = uniseg.StepString(s, state)
			if !yield(at, cluster) {
				return
			}
			at += len(cluster)
		}
	}
}

// NextCluster is the byte offset after the cluster at i, or len(s) at the end.
func NextCluster(s string, i int) int {
	if i < 0 {
		return 0
	}
	if i >= len(s) {
		return len(s)
	}
	// Plain ASCII is one cluster per byte. Requiring ASCII on both sides keeps this
	// fast path out of the cases where adjacent Unicode can join it (Prepend, Extend
	// and ZWJ sequences). CRLF is the one multi-byte ASCII cluster.
	if s[i] < utf8.RuneSelf && (i == 0 || s[i-1] < utf8.RuneSelf) &&
		(i+1 == len(s) || s[i+1] < utf8.RuneSelf) {
		if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
			return i + 2
		}
		return i + 1
	}
	for at, cluster := range Clusters(s) {
		if after := at + len(cluster); i < after {
			return after
		}
	}
	return len(s)
}

// PrevCluster is the byte offset of the cluster ending at i, or zero at the start.
func PrevCluster(s string, i int) int {
	if i <= 0 {
		return 0
	}
	i = min(i, len(s))
	if i == 0 {
		return 0
	}
	last := i - 1
	if s[last] < utf8.RuneSelf && (last == 0 || s[last-1] < utf8.RuneSelf) {
		if s[last] == '\n' && last > 0 && s[last-1] == '\r' {
			return last - 1
		}
		return last
	}
	for at, cluster := range Clusters(s) {
		if i <= at+len(cluster) {
			return at
		}
	}
	return len(s)
}

// ColumnOf is how many columns of s sit before the byte offset i.
func ColumnOf(s string, i int) int {
	col := 0
	for at, cluster := range Clusters(s) {
		if at >= i {
			break
		}
		if cluster == "\t" {
			col = layout.Sum(col, TabStop-col%TabStop)
			continue
		}
		col = layout.Sum(col, clusterWidth(cluster))
	}
	return col
}

// OffsetAt is the byte offset of the cluster boundary nearest to column col,
// without going past it. It is how a click, or a cursor moving between lines of
// different lengths, finds where it lands.
func OffsetAt(s string, col int) int {
	if col <= 0 {
		return 0
	}
	at, width := 0, 0
	for offset, cluster := range Clusters(s) {
		step := clusterWidth(cluster)
		if cluster == "\t" {
			step = TabStop - width%TabStop
		}
		next := layout.Sum(width, step)
		if next > col {
			return offset
		}
		width = next
		at = offset + len(cluster)
	}
	return at
}

// StampLink turns the columns occupied by the byte range [start, end) of s into a
// hyperlink, on text already drawn at (x, y), and reports the columns it covered.
//
// The conversion is the whole point of it existing. Anything that finds links works
// in bytes, because bytes are what text is; a cell is a column, and the two counts
// are not the same one. A URL written after an emoji begins at a different column
// than at its byte offset, one containing a full-width character covers more columns
// than it has clusters, and a tab before it moves everything by however much was
// left to the next stop. Getting that wrong underlines the wrong text — visibly, and
// only for the people whose text is not ASCII.
//
// The returned width is what a caller records so that a click on those columns can
// be answered later.
func StampLink(v grid.View, x, y int, s string, start, end int, url string) (col, width int) {
	if start < 0 || end > len(s) || start >= end {
		return 0, 0
	}
	col = ColumnOf(s, start)
	width = ColumnOf(s, end) - col
	v.Link(layout.Translate(x, col), y, width, url)
	return col, width
}

// Class is the family of characters a cluster belongs to, for deciding where a word
// begins and ends.
type Class uint8

const (
	// Space is whitespace, which is not part of any word.
	Space Class = iota
	// Word is a letter, a digit or an underscore — the run a double-click takes in
	// text written in an alphabet.
	Word
	// Han is Chinese, and the first of the three scripts written without spaces.
	//
	// In those, a run of letters is not a word and the script itself is the only
	// boundary left. Double-clicking inside 中文词组 takes the whole run: taking the
	// alphabetic rule instead would swallow the Latin beside it, and taking one
	// character would select less than anybody meant.
	Han
	// Kana is Japanese hiragana and katakana, which are one boundary between them
	// because a word switches from one to the other inside itself.
	Kana
	// Hangul is Korean.
	Hangul
	// Punct is everything else. It selects only itself, because a run of punctuation
	// is not a thing anybody means to have selected.
	Punct
)

// ClassOf is the family the first character of a cluster belongs to.
func ClassOf(cluster string) Class {
	for _, r := range cluster {
		switch {
		case unicode.IsSpace(r):
			return Space
		case r == '_' || unicode.IsLetter(r) && !cjk(r) || unicode.IsDigit(r) && !cjk(r):
			return Word
		case unicode.Is(unicode.Han, r):
			return Han
		case unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
			return Kana
		case unicode.Is(unicode.Hangul, r):
			return Hangul
		default:
			return Punct
		}
	}
	return Space
}

// cjk reports whether a rune belongs to a script written without spaces.
func cjk(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r)
}

// WordAt is the byte range of the word containing an offset, and whether there is one.
//
// A word is the run of clusters sharing the class of the one under the offset — see
// [Class]. Whitespace is not a word and reports false, so a double-click in the margin
// selects nothing rather than selecting the gap. Punctuation is its own word of one
// cluster, because a run of it is not something anybody means to have selected.
//
// The offset is pulled to a cluster boundary first, so a caller working in columns
// cannot land inside a character and get half of it.
func WordAt(s string, at int) (start, end int, ok bool) {
	if s == "" || at < 0 || at >= len(s) {
		return 0, 0, false
	}
	// The clusters, with the class of each, and which of them the offset is inside.
	// Materialised rather than walked twice: the walk is the expensive part, this
	// runs once per double-click, and expanding in both directions from the middle of
	// an iterator is how the first version of it came to be wrong.
	type span struct {
		from, to int
		class    Class
	}
	var spans []span
	hit := -1
	for offset, cluster := range Clusters(s) {
		if at >= offset && at < offset+len(cluster) {
			hit = len(spans)
		}
		spans = append(spans, span{from: offset, to: offset + len(cluster), class: ClassOf(cluster)})
	}
	if hit < 0 {
		return 0, 0, false
	}

	switch class := spans[hit].class; class {
	case Space:
		return 0, 0, false
	case Punct:
		return spans[hit].from, spans[hit].to, true
	default:
		first, last := hit, hit
		for first > 0 && spans[first-1].class == class {
			first--
		}
		for last+1 < len(spans) && spans[last+1].class == class {
			last++
		}
		return spans[first].from, spans[last].to, true
	}
}
