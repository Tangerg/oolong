package ptytest

import (
	"errors"
	"fmt"
	"image"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/ansi"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

// ErrUnsupportedOutput means output asked the screen assertion helper to model
// something outside its deliberate renderer-sized subset.
//
// Oolong's cell renderer, inline renderer, and ordinary SGR and OSC text are in the
// subset. Device-control strings and arbitrary terminal programs are not: accepting
// those would turn this test helper into the terminal emulator it is meant to avoid.
var ErrUnsupportedOutput = errors.New("ptytest: unsupported screen output")

const (
	maxGraphemeBytes = 64 << 10
	defaultTabWidth  = 8
)

type screenCell struct {
	text  string
	trail bool
}

// Screen is the visible cell text produced by renderer output.
//
// It is intentionally narrower than a terminal. Apply understands the cursor moves,
// erasure, bounded scrolling, SGR, OSC, synchronized-output and mode sequences emitted
// around Oolong frames. Styles, cursor visibility, terminal queries, input protocols,
// alternate-buffer ownership and non-cell painters are outside the model. The result
// answers one testing question: what text occupies each cell after these renderer
// writes?
//
// A Screen has a fixed positive size and is not safe for concurrent use. It must not
// be copied after first use. Obtain a snapshot from [Transcript.Screen] when output
// may still be arriving.
//
// Screen deliberately shares the renderer's text-width estimator. Its independent
// evidence is the terminal-side state machine that applies those widths: pending
// wrap, overwrite, erasure and scrolling. Width fixtures, not this model, are the
// estimator's oracle.
type Screen struct {
	noCopy noCopy

	size        Size
	cells       []screenCell
	at          image.Point
	pendingWrap bool
	top         int
	end         int
	scan        ansi.Scanner
	grapheme    string
}

// noCopy makes the assertion model's single-owner contract visible to go vet. Its
// methods are never called.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// NewScreen returns a blank screen-state assertion model. Like the program it
// models, it refuses allocations exceeding [grid.ValidateTerminalSize].
func NewScreen(size Size) (*Screen, error) {
	if err := grid.ValidateTerminalSize(size.Cols, size.Rows); err != nil {
		return nil, err
	}
	if err := size.check(); err != nil {
		return nil, err
	}
	return &Screen{
		size:  size,
		cells: make([]screenCell, size.Cols*size.Rows),
		end:   size.Rows,
	}, nil
}

func (s *Screen) Size() Size {
	if s == nil {
		return Size{}
	}
	return s.size
}

// Apply consumes the next renderer-output chunk.
//
// Chunks may split UTF-8, a grapheme or an escape sequence. A single grapheme
// may occupy at most 64 KiB regardless of chunking; ordinary text runs can be
// longer. The ANSI scanner bounds unfinished escapes. Apply either consumes the whole
// chunk or returns at the first unsupported complete piece; earlier complete pieces
// remain applied and callers should stop after an error.
func (s *Screen) Apply(chunk []byte) error {
	if s == nil || len(chunk) == 0 {
		return nil
	}
	err := s.scan.Feed(string(chunk), s.applySequence)
	if errors.Is(err, ansi.ErrSequenceTooLong) {
		return fmt.Errorf("%w: %w", ErrUnsupportedOutput, err)
	}
	return err
}

// Flush rejects an unfinished UTF-8 character or escape sequence. A complete stream
// has nothing buffered and returns nil.
func (s *Screen) Flush() error {
	if s == nil {
		return nil
	}
	s.settleGrapheme()
	held := s.scan.Pending()
	s.scan.Reset()
	if held == "" {
		return nil
	}
	return fmt.Errorf("%w: incomplete output %q", ErrUnsupportedOutput, held)
}

// At returns the grapheme whose head occupies a zero-based cell. Blank cells,
// continuation columns, and coordinates outside the screen return empty.
func (s *Screen) At(column, row int) string {
	if s == nil || column < 0 || row < 0 || column >= s.size.Cols || row >= s.size.Rows {
		return ""
	}
	if s.grapheme != "" {
		return s.projected().At(column, row)
	}
	return s.cells[s.index(column, row)].text
}

// Rows returns the visible text as fixed-width rows. Continuation columns contribute
// no extra bytes: the grapheme at their head already occupies the complete display
// atom. The returned slice owns its strings.
func (s *Screen) Rows() []string {
	if s == nil {
		return nil
	}
	if s.grapheme != "" {
		return s.projected().Rows()
	}
	rows := make([]string, s.size.Rows)
	for y := range s.size.Rows {
		var row strings.Builder
		row.Grow(s.size.Cols)
		for x := range s.size.Cols {
			cell := s.cells[s.index(x, y)]
			switch {
			case cell.trail:
			case cell.text != "":
				row.WriteString(cell.text)
			default:
				row.WriteByte(' ')
			}
		}
		rows[y] = row.String()
	}
	return rows
}

func (s *Screen) applySequence(piece ansi.Piece) error {
	if piece.Kind != ansi.Plain && (piece.Kind != ansi.Control || ansi.Function(piece.Final) != ansi.SGR) && piece.Kind != ansi.String {
		s.settleGrapheme()
	}
	switch piece.Kind {
	case ansi.Plain:
		if !utf8.ValidString(piece.Raw) {
			return fmt.Errorf("%w: malformed UTF-8", ErrUnsupportedOutput)
		}
		return s.writePlain(piece.Raw)
	case ansi.Control:
		return s.control(piece)
	case ansi.String:
		if nonPaintingString(piece) {
			return nil
		}
	case ansi.Other:
		// The ASCII designation preserves the text model. DEC line drawing and
		// other character sets would change the meaning of subsequent bytes.
		if piece.Raw == ansi.DesignateASCII {
			return nil
		}
	case ansi.Malformed:
	}
	return fmt.Errorf("%w: %q", ErrUnsupportedOutput, piece.Raw)
}

func nonPaintingString(piece ansi.Piece) bool {
	switch piece.Family {
	case ansi.DCS:
		return piece.Raw == ansi.QueryCursorStyle
	case ansi.APC:
		return kittyQuery(piece.Body)
	case ansi.OSC:
		number, _, _ := strings.Cut(piece.Body, ";")
		command, err := strconv.Atoi(number)
		if err != nil {
			return false
		}
		switch command {
		case ansi.OSCTitle, ansi.OSCDirectory, ansi.OSCHyperlink, ansi.OSCNotify,
			ansi.OSCForeground, ansi.OSCBackground, ansi.OSCClipboard:
			return true
		}
	case ansi.PM, ansi.SOS:
	}
	return false
}

// A graphics capability query leaves the cell grid untouched. Opaque painters
// and commands with conflicting actions remain outside this assertion model.
func kittyQuery(body string) bool {
	body, ok := strings.CutPrefix(body, "G")
	controls, _, payload := strings.Cut(body, ";")
	if !ok || !payload {
		return false
	}
	query := false
	for field := range strings.SplitSeq(controls, ",") {
		key, value, valid := strings.Cut(field, "=")
		if !valid || len(key) != 1 || value == "" {
			return false
		}
		if key == "a" {
			if query || value != "q" {
				return false
			}
			query = true
		}
	}
	return query
}

func (s *Screen) control(piece ansi.Piece) error {
	params := ansi.Parse(piece.Parameters)
	if !params.Valid() {
		return fmt.Errorf("%w: malformed parameters in %q", ErrUnsupportedOutput, piece.Raw)
	}
	if piece.Intermediates != "" {
		// DECSCUSR changes only cursor appearance. Other intermediate-bearing CSI
		// sequences are outside the renderer subset until a renderer emits one.
		if _, ok := piece.CursorStyle(); ok {
			return nil
		}
		return fmt.Errorf("%w: %q", ErrUnsupportedOutput, piece.Raw)
	}
	if ignoresControl(piece, params) {
		return nil
	}
	if params.Marker() != 0 {
		return fmt.Errorf("%w: %q", ErrUnsupportedOutput, piece.Raw)
	}
	return s.executeControl(piece.Final, params, piece.Raw)
}

// ignoresControl reports session traffic that paints no cells. The model accepts
// the question or mode change but never fabricates a terminal answer.
func ignoresControl(piece ansi.Piece, params ansi.Params) bool {
	switch piece.Raw {
	case ansi.QueryPrimaryAttributes, ansi.QuerySecondaryAttributes, ansi.QueryTerminalVersion,
		ansi.QueryKeyboard, ansi.PopKeyboard, ansi.PushTitle, ansi.PopTitle:
		// Other window commands can resize the terminal and invalidate this
		// model's fixed geometry.
		return true
	}
	if function := ansi.Function(piece.Final); function == ansi.SM || function == ansi.RM {
		return nonCellModes(params)
	}
	return params.Len() == 1 && piece.Raw == ansi.PushKeyboard(params.At(0))
}

func nonCellModes(params ansi.Params) bool {
	if params.Marker() != ansi.DECPrivate || params.Len() == 0 {
		return false
	}
	for i := range params.Len() {
		// Unmodelled origin, insert or autowrap modes change subsequent cells.
		if params.Group(i).Len() != 1 {
			return false
		}
		switch params.At(i) {
		case int(ansi.ModeCursorVisible), int(ansi.ModeAnyEventMouse), int(ansi.ModeFocusReports), int(ansi.ModeSGRMouse),
			int(ansi.ModeAltScreen), int(ansi.ModeBracketedPaste), int(ansi.ModeSynchronizedOutput):
		default:
			return false
		}
	}
	return true
}

func (s *Screen) executeControl(final byte, params ansi.Params, raw string) error {
	function := ansi.Function(final)
	if function == ansi.SGR {
		return nil
	}
	if !paramsFit(function, params) {
		return fmt.Errorf("%w: %q", ErrUnsupportedOutput, raw)
	}
	if s.moveCursor(function, params) {
		return nil
	}
	switch function {
	case ansi.EL:
		if err := s.eraseLine(params.At(0), raw); err != nil {
			return err
		}
		s.pendingWrap = false
	case ansi.ED:
		if err := s.eraseDisplay(params.At(0), raw); err != nil {
			return err
		}
		s.pendingWrap = false
	case ansi.SU:
		s.scrollUp(defaultOne(params.At(0)))
	case ansi.SD:
		s.scrollDown(defaultOne(params.At(0)))
	case ansi.DECSTBM:
		return s.setMargins(params, raw)
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedOutput, raw)
	}
	return nil
}

// paramsFit reports whether a control's parameters are the single values this model
// reads: two for a cursor position or margins, one for everything else.
func paramsFit(function ansi.Function, params ansi.Params) bool {
	limit := 1
	if function == ansi.CUP || function == ansi.HVP || function == ansi.DECSTBM {
		limit = 2
	}
	return params.Len() <= limit && params.Group(0).Len() <= 1 && params.Group(1).Len() <= 1
}

func (s *Screen) moveCursor(function ansi.Function, params ansi.Params) bool {
	switch function {
	case ansi.CUP, ansi.HVP:
		row := defaultOne(params.At(0)) - 1
		column := defaultOne(params.At(1)) - 1
		s.move(column, row)
	case ansi.CUU:
		top := 0
		if s.at.Y >= s.top {
			top = s.top
		}
		s.move(s.at.X, s.at.Y-min(defaultOne(params.At(0)), s.at.Y-top))
	case ansi.CUD:
		bottom := s.size.Rows - 1
		if s.at.Y < s.end {
			bottom = s.end - 1
		}
		s.move(s.at.X, s.at.Y+min(defaultOne(params.At(0)), bottom-s.at.Y))
	case ansi.CUF:
		s.move(s.at.X+min(defaultOne(params.At(0)), s.size.Cols-1-s.at.X), s.at.Y)
	case ansi.CUB:
		s.move(s.at.X-min(defaultOne(params.At(0)), s.at.X), s.at.Y)
	case ansi.CHA:
		s.move(defaultOne(params.At(0))-1, s.at.Y)
	case ansi.VPA:
		s.move(s.at.X, defaultOne(params.At(0))-1)
	default:
		return false
	}
	return true
}

func defaultOne(n int) int {
	if n == 0 {
		return 1
	}
	return n
}

func (s *Screen) writePlain(raw string) error {
	for raw != "" {
		i := strings.IndexFunc(raw, unicode.IsControl)
		if i < 0 {
			return s.writeText(raw)
		}
		if err := s.writeText(raw[:i]); err != nil {
			return err
		}
		control, size := utf8.DecodeRuneInString(raw[i:])
		if err := s.controlRune(control); err != nil {
			return err
		}
		raw = raw[i+size:]
	}
	return nil
}

func (s *Screen) controlRune(control rune) error {
	if control == '\x00' || control == '\a' || control == '\x7f' {
		return nil
	}
	s.settleGrapheme()
	switch control {
	case '\b':
		s.at.X = max(s.at.X-1, 0)
	case '\t':
		// With no further tab stop the cursor stops at the last column of this row.
		// Clamping to the width instead would leave it one past the end, where the
		// next character wraps onto a line the terminal never moved to.
		s.at.X = min((s.at.X/defaultTabWidth+1)*defaultTabWidth, s.size.Cols-1)
		// A tab at the right margin leaves the next printable atom's wrap pending.
		return nil
	case '\n', '\v', '\f':
		s.lineFeed()
	case '\r':
		s.at.X = 0
	default:
		return fmt.Errorf("%w: control %U", ErrUnsupportedOutput, control)
	}
	s.pendingWrap = false
	return nil
}

func (s *Screen) writeText(raw string) error {
	if raw == "" {
		return nil
	}
	source := s.grapheme + raw
	s.grapheme = ""
	last := ""
	for _, cluster := range text.Clusters(source) {
		s.putText(last)
		// An arbitrary number of combining marks can extend even one cell.
		// Check before retaining it as either a displayed or undecided cluster.
		if len(cluster) > maxGraphemeBytes {
			return fmt.Errorf("%w: grapheme exceeds %d bytes", ErrUnsupportedOutput, maxGraphemeBytes)
		}
		last = cluster
	}
	s.grapheme = strings.Clone(last)
	return nil
}

func (s *Screen) settleGrapheme() {
	s.putText(s.grapheme)
	s.grapheme = ""
}

func (s *Screen) projected() *Screen {
	projection := &Screen{size: s.size, cells: slices.Clone(s.cells), at: s.at, pendingWrap: s.pendingWrap, top: s.top, end: s.end}
	projection.putText(s.grapheme)
	return projection
}

func (s *Screen) putText(raw string) {
	for _, cluster := range text.Clusters(raw) {
		width := text.Width(cluster)
		if width <= 0 || width > s.size.Cols {
			continue
		}
		if s.pendingWrap || s.at.X >= s.size.Cols || s.at.X+width > s.size.Cols {
			s.at.X = 0
			s.lineFeed()
			s.pendingWrap = false
		}
		s.clearGlyphAt(s.at.X, s.at.Y)
		cell := s.index(s.at.X, s.at.Y)
		s.cells[cell] = screenCell{text: strings.Clone(cluster)}
		for x := 1; x < width; x++ {
			s.clearGlyphAt(s.at.X+x, s.at.Y)
			s.cells[s.index(s.at.X+x, s.at.Y)] = screenCell{trail: true}
		}
		next := s.at.X + width
		if next == s.size.Cols {
			s.at.X = s.size.Cols - 1
			s.pendingWrap = true
			continue
		}
		s.at.X = next
	}
}

func (s *Screen) move(x, y int) {
	s.at.X = min(max(x, 0), s.size.Cols-1)
	s.at.Y = min(max(y, 0), s.size.Rows-1)
	s.pendingWrap = false
}

func (s *Screen) lineFeed() {
	if s.at.Y == s.end-1 {
		s.scrollUp(1)
		return
	}
	s.at.Y = min(s.at.Y+1, s.size.Rows-1)
}

// Erase controls take one of these extents; an omitted parameter means the first.
const (
	eraseToEnd = iota
	eraseToStart
	eraseAll
)

func (s *Screen) eraseLine(mode int, raw string) error {
	switch mode {
	case eraseToEnd:
		s.blank(s.at.Y, s.at.X, s.size.Cols)
	case eraseToStart:
		s.blank(s.at.Y, 0, s.at.X+1)
	case eraseAll:
		s.blank(s.at.Y, 0, s.size.Cols)
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedOutput, raw)
	}
	return nil
}

func (s *Screen) eraseDisplay(mode int, raw string) error {
	switch mode {
	case eraseToEnd:
		s.blank(s.at.Y, s.at.X, s.size.Cols)
		for y := s.at.Y + 1; y < s.size.Rows; y++ {
			s.blank(y, 0, s.size.Cols)
		}
	case eraseToStart:
		for y := range s.at.Y {
			s.blank(y, 0, s.size.Cols)
		}
		s.blank(s.at.Y, 0, s.at.X+1)
	case eraseAll:
		clear(s.cells)
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedOutput, raw)
	}
	return nil
}

func (s *Screen) setMargins(params ansi.Params, raw string) error {
	top := defaultOne(params.At(0)) - 1
	end := params.At(1)
	if end == 0 {
		end = s.size.Rows
	}
	if end > s.size.Rows || params.Len() > 0 && top+1 >= end {
		return fmt.Errorf("%w: %q", ErrUnsupportedOutput, raw)
	}
	s.top, s.end = top, end
	s.move(0, 0)
	return nil
}

func (s *Screen) scrollUp(n int) {
	n = min(max(n, 0), s.end-s.top)
	if n == 0 {
		return
	}
	width := s.size.Cols
	from := (s.top + n) * width
	to := s.end * width
	copy(s.cells[s.top*width:], s.cells[from:to])
	clear(s.cells[(s.end-n)*width : to])
}

func (s *Screen) scrollDown(n int) {
	n = min(max(n, 0), s.end-s.top)
	if n == 0 {
		return
	}
	width := s.size.Cols
	from := s.top * width
	to := (s.end - n) * width
	copy(s.cells[(s.top+n)*width:s.end*width], s.cells[from:to])
	clear(s.cells[from : from+n*width])
}

func (s *Screen) blank(row, start, end int) {
	start = min(max(start, 0), s.size.Cols)
	end = min(max(end, start), s.size.Cols)
	for x := start; x < end; x++ {
		s.clearGlyphAt(x, row)
	}
}

func (s *Screen) clearGlyphAt(x, y int) {
	if x < 0 || y < 0 || x >= s.size.Cols || y >= s.size.Rows {
		return
	}
	at := s.index(x, y)
	if s.cells[at].trail {
		for at > y*s.size.Cols && s.cells[at].trail {
			at--
		}
	}
	width := 1
	for at+width < (y+1)*s.size.Cols && s.cells[at+width].trail {
		width++
	}
	clear(s.cells[at : at+width])
}

func (s *Screen) index(x, y int) int { return y*s.size.Cols + x }
