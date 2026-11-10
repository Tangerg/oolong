// Package text shares terminal-column layout across measurement and drawing so
// wrapping, carets, and cells cannot disagree. Display atoms remain indivisible,
// including wide grapheme clusters and zero-width combining marks. Decoder turns
// command output into the same styled spans after removing terminal commands.
package text

import (
	"iter"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/internal/grapheme"
	"github.com/Tangerg/oolong/ui/core/layout"
)

// TabStop matches conventional terminal tab stops so previously formatted
// command output retains its column relationships.
const TabStop = 8

type Span struct {
	Text  string
	Style grid.Style
	// Links travel with spans through wrapping and clipping; original byte
	// offsets alone cannot identify the resulting terminal cells.
	Link string
	// BreakBefore keeps this span's first grapheme independent of preceding text.
	// Ordinary appearance changes leave it false so they cannot split a grapheme.
	BreakBefore bool
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

func Of(s string, style grid.Style) Line {
	if s == "" {
		return nil
	}
	return Line{{Text: s, Style: style}}
}

// Highlight styles each display grapheme containing a matched source byte offset.
// The original bytes remain intact so Line layout owns tab stops and clipping.
// Dropped and out-of-range byte offsets have no visible effect.
func Highlight(value string, offsets []int, normal, matched grid.Style) Line {
	if len(offsets) == 0 || normal == matched {
		return Of(value, normal)
	}
	if !slices.IsSorted(offsets) {
		offsets = slices.Clone(offsets)
		slices.Sort(offsets)
	}
	var line Line
	start := 0
	style := normal
	projection := projectDisplay(value)
	for cluster := range projection.clusters() {
		at := cluster.From
		hit, remaining := projection.match(cluster, offsets)
		offsets = remaining
		next := normal
		if hit {
			next = matched
		}
		if next != style {
			if start < at {
				line = append(line, Span{Text: value[start:at], Style: style})
			}
			start, style = at, next
		}
		if len(offsets) == 0 && style == normal {
			break
		}
	}
	if start < len(value) {
		line = append(line, Span{Text: value[start:], Style: style})
	}
	return line
}

// String serializes the content for copying. Explicit grapheme boundaries belong
// to the Line and are not encoded in the returned bytes; geometry uses Line methods.
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
	asciiWidth := 0
	ascii := true
	for _, span := range l {
		if !plainASCII(span.Text) {
			ascii = false
			break
		}
		asciiWidth = layout.Sum(asciiWidth, len(span.Text))
	}
	if ascii {
		return asciiWidth
	}
	width := 0
	for u := range l.eachUnit() {
		width = layout.Sum(width, u.width)
	}
	return width
}

// Draw writes the line onto v at (x, y) and returns how many columns it advanced.
// Tabs are expanded from the line's own start, not from the view's, so a line
// drawn at an indent keeps the column relationships it was written with.
//
// A span that points somewhere is stamped onto the columns it took, so the text a
// terminal shows is the text a terminal will open — and a link that a wrap broke in
// two is stamped on both halves, which is how one hyperlink covers two rows.
// The line owns its geometry: a standalone zero-width cluster cannot borrow a
// preceding canvas cell, including an unpainted cell skipped by a tab.
func (l Line) Draw(v grid.View, x, y int) int {
	col, linkStart := 0, 0
	var target string
	endLink := func() {
		if target != "" {
			v.Link(layout.Translate(x, linkStart), y, col-linkStart, target)
		}
	}
	for u := range l.eachUnit() {
		if link := u.link(); link != target {
			endLink()
			linkStart, target = col, link
		}
		if u.linked && u.width > 0 {
			v.Text(layout.Translate(x, col), y, u.cluster, u.style())
		}
		col = layout.Sum(col, u.width)
	}
	endLink()
	return col
}

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
	heldFrom int
	heldTo   int
	hasHeld  bool
	heldW    int
	// rowGap is what was consumed before the row now accumulating; nextGap is what
	// has been consumed since it started, and belongs to whatever row follows the
	// break.
	rowGap  string
	nextGap string
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
	w.clearHeld()
}

// discardHeld gives up the spaces a break consumed, keeping them for the row they
// now precede rather than the one they followed.
func (w *wrapper) discardHeld(units []unit) {
	var consumed strings.Builder
	for _, u := range units[w.heldFrom:w.heldTo] {
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
	for _, u := range units[w.heldFrom:w.heldTo] {
		if len(w.row) > 0 && layout.Sum(w.rowWidth, u.width) > w.width {
			w.breakRow()
		}
		w.place(u)
	}
	w.clearHeld()
}

func (w *wrapper) clearHeld() {
	w.heldFrom, w.heldTo, w.hasHeld = 0, 0, false
	w.heldW = 0
}

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
	case layout.Sum(w.rowWidth, w.heldW, wordWidth) <= w.width:
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
	if w.hasHeld {
		switch {
		case layout.Sum(w.rowWidth, w.heldW, units[from].width) <= w.width:
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

// Width is how many columns s would occupy, with tabs expanded from column zero.
func Width(s string) int { return Advance(s, 0) }

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

func (l Line) units() []unit {
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
	for u := range l.eachUnit() {
		units = append(units, u)
	}
	return units
}

func (l Line) eachUnit() iter.Seq[unit] {
	return func(yield func(unit) bool) {
		col := 0
		for cluster, s := range l.clusters() {
			switch cluster.Text {
			case "\t":
				n := TabStop - col%TabStop
				for range n {
					if !yield(unit{cluster: " ", source: s, width: 1, space: true, at: cluster.From, size: cluster.To - cluster.From}) {
						return
					}
				}
				col = layout.Sum(col, n)
			default:
				w := grid.ClusterWidth(cluster.Text)
				if !yield(unit{cluster: cluster.Text, source: s, linked: true, width: w, space: cluster.Text == " ", at: cluster.From, size: cluster.To - cluster.From}) {
					return
				}
				col = layout.Sum(col, w)
			}
		}
	}
}

func (l Line) ByteLen() int {
	n := 0
	for _, s := range l {
		n += len(s.Text)
	}
	return n
}

func line(units []unit) Line {
	if len(units) == 0 {
		return Line{}
	}
	// Every non-empty row has one style run. Let uncommon additional runs grow the
	// slice instead of reserving one Span for every grapheme in ordinary plain text.
	out := make(Line, 0, 1)
	previous := ""
	for first := 0; first < len(units); {
		boundary := joins(previous, units[first].cluster)
		last := first + 1
		bytes := len(units[first].cluster)
		for last < len(units) && units[last].sameRun(units[first]) && !joins(units[last-1].cluster, units[last].cluster) {
			bytes += len(units[last].cluster)
			last++
		}
		var text strings.Builder
		text.Grow(bytes)
		for _, u := range units[first:last] {
			text.WriteString(u.cluster)
		}
		out = append(out, Span{
			Text:        text.String(),
			Style:       units[first].style(),
			Link:        units[first].link(),
			BreakBefore: boundary,
		})
		previous = units[last-1].cluster
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

// Advance returns the final terminal column after drawing s from col. Tabs use
// their position in the whole line even when a caller measures it in pieces
// separated at display grapheme boundaries after filtering. Negative starting
// columns count as zero.
func Advance(s string, col int) int {
	col = max(0, col)
	if plainASCII(s) {
		return layout.Sum(col, len(s))
	}
	for cluster := range projectDisplay(s).clusters() {
		col = advanceCluster(cluster.Text, col)
	}
	return col
}

func advanceCluster(cluster string, col int) int {
	if cluster == "\t" {
		return layout.Sum(col, TabStop-col%TabStop)
	}
	return layout.Sum(col, grid.ClusterWidth(cluster))
}

// plainASCII reports whether every byte is one printable, one-column cluster.
// Keeping that fact here lets every byte/column conversion share the common fast
// path without weakening the Unicode path used as the authority for all other text.
func plainASCII(s string) bool {
	for i := range len(s) {
		if !plainASCIIByte(s[i]) {
			return false
		}
	}
	return true
}

func plainASCIIByte(b byte) bool { return b >= ' ' && b <= '~' }

// Measurement and drawing share dropped so their widths agree. Unlike
// Printable, display projection omits malformed bytes; stored text retains a
// replacement marker for them.
func dropped(cluster string) bool {
	if !utf8.ValidString(cluster) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(cluster)
	return droppedRune(r)
}

// Tabs are expanded by layout; other controls must not reach terminal output.
func droppedRune(r rune) bool { return r != '\t' && unicode.IsControl(r) }

// displayFilter decides, cluster by cluster in line order, which clusters reach a
// cell. Removing a control can leave a zero-width cluster against one that
// segmentation kept it apart from: a variation selector after a newline would
// turn the symbol before the newline into a wider emoji. Such a cluster shows
// nothing of its own, so it is removed with the control instead of being allowed
// to change its neighbor. Measurement is unaffected because it has no width.
type displayFilter struct {
	previous string
	removed  bool
}

func (d *displayFilter) hides(cluster string) bool {
	if dropped(cluster) || d.removed && grid.ClusterWidth(cluster) == 0 && joins(d.previous, cluster) {
		d.removed = true
		return true
	}
	d.previous, d.removed = cluster, false
	return false
}

// joins reports whether segmentation would make next part of previous once the
// two are adjacent.
func joins(previous, next string) bool {
	if previous == "" || next == "" || plainASCIIByte(previous[len(previous)-1]) && plainASCIIByte(next[0]) {
		return false
	}
	return !IsClusterBoundary(previous+next, len(previous))
}

// Display removes controls and malformed bytes while preserving independent
// boundaries and tabs. Its spans may share storage with the source; use Clone at
// ownership boundaries.
//
// It removes rather than replaces, which is where it parts company with [Printable]:
// that one normalizes text somebody will store and hands back a mark where invalid
// bytes were, and this one answers what was drawn, which is nothing.
func (l Line) Display() Line {
	// Long ASCII labels need no segmentation just to establish that every byte
	// survives. Clipping will segment only the prefix it can retain.
	ascii := true
	for _, span := range l {
		if !plainASCII(span.Text) {
			ascii = false
			break
		}
	}
	if ascii {
		return l
	}
	out := make(Line, 0, len(l))
	changed := false
	for _, section := range l.sections() {
		display := projectDisplay(section.String())
		if display.text == display.source {
			out = append(out, section...)
			continue
		}
		changed = true
		var value strings.Builder
		var from *Span
		boundary := section[0].BreakBefore
		end := func() {
			if value.Len() > 0 {
				out = append(out, Span{Text: value.String(), Style: from.Style, Link: from.Link, BreakBefore: boundary})
				value.Reset()
				boundary = false
			}
		}
		for cluster, source := range styledClusters(section, display) {
			if source != from {
				end()
				from = source
			}
			value.WriteString(cluster.Text)
		}
		end()
	}
	if !changed {
		return l
	}
	return out
}

func prefix(s string, budget int) string {
	col, end := 0, 0
	clusters := grapheme.New(s)
	for clusters.Next() {
		next := advanceCluster(clusters.Value(), col)
		if next > budget {
			break
		}
		col = next
		end = clusters.End()
	}
	return s[:end]
}

// Clusters preserves grapheme boundaries because a cursor between a base and
// its combining mark has no display position. Malformed bytes remain separate
// zero-width clusters so later offsets still address the original input.
func Clusters(s string) iter.Seq2[int, string] {
	return func(yield func(int, string) bool) {
		clusters := grapheme.New(s)
		for clusters.Next() {
			if !yield(clusters.Start(), clusters.Value()) {
				return
			}
		}
	}
}

// IsClusterBoundary tests a byte offset in the complete string. Concatenation can
// move boundaries without changing the total cluster count, as regional indicators
// pair with different neighbors. Out-of-range offsets are not boundaries.
func IsClusterBoundary(s string, at int) bool {
	return at >= 0 && at <= len(s) && (at == 0 || NextCluster(s, at-1) == at)
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

// ColumnOf maps original bytes onto the filtered display. An offset inside a
// display grapheme maps after the whole atom, even when filtering joined it.
func ColumnOf(s string, i int) int {
	i = min(max(i, 0), len(s))
	// Only the prefix before i contributes to the answer. The byte at i need only be
	// ASCII, not printable: a control there occupies no preceding column, while a
	// Unicode Extend or ZWJ may still join the cluster before the boundary.
	if plainASCII(s[:i]) && (i == len(s) || s[i] < utf8.RuneSelf) {
		return i
	}
	return Of(s, grid.Style{}).ColumnOf(i)
}

// OffsetAt is the original byte offset of the display boundary nearest to col,
// without going past it. It is how a click, or a cursor moving between lines of
// different lengths, finds where it lands.
func OffsetAt(s string, col int) int {
	if col <= 0 {
		return 0
	}
	end := min(col, len(s))
	// Unlike ColumnOf, the byte at the candidate boundary must itself occupy one
	// column. A tab or zero-width control belongs to this column and must be consumed;
	// a Unicode byte may join or widen the preceding cluster.
	if plainASCII(s[:end]) && (end == len(s) || plainASCIIByte(s[end])) {
		return end
	}
	return Of(s, grid.Style{}).OffsetAt(col)
}

// StampLink converts source byte ranges to terminal columns so wide clusters
// and preceding tabs cannot shift a link onto unrelated text. The returned
// width lets the caller use those same columns for hit testing.
func StampLink(v grid.View, x, y int, s string, start, end int, url string) (col, width int) {
	if start < 0 || end > len(s) || start >= end {
		return 0, 0
	}
	col = ColumnOf(s, start)
	width = ColumnOf(s, end) - col
	v.Link(layout.Translate(x, col), y, width, url)
	return col, width
}

type Class uint8

const (
	Space Class = iota
	// Word is a letter, a digit or an underscore — the run a double-click takes in
	// text written in an alphabet.
	Word
	// Han stays separate from adjacent alphabetic text: script runs provide a
	// selection boundary where spaces do not.
	Han
	// Kana is Japanese hiragana and katakana, which are one boundary between them
	// because a word switches from one to the other inside itself.
	Kana
	Hangul
	// Punct selects one cluster to avoid claiming adjacent symbols.
	Punct
)

func ClassOf(cluster string) Class {
	if cluster == "" {
		return Space
	}
	// A cluster is classified by its base rune; marks joined to it do not change
	// what kind of word it belongs to.
	r, _ := utf8.DecodeRuneInString(cluster)
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
	return Of(s, grid.Style{}).WordAt(at)
}
