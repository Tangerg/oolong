package ptytest

import (
	"errors"
	"fmt"
	"image"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/oolong/core/ansi"
	"github.com/Tangerg/oolong/core/text"
)

// ErrUnsupportedOutput means output asked the screen assertion helper to model
// something outside its deliberate renderer-sized subset.
//
// Oolong's cell renderer, inline renderer, and ordinary SGR and OSC text are in the
// subset. Device-control strings and arbitrary terminal programs are not: accepting
// those would turn this test helper into the terminal emulator it is meant to avoid.
var ErrUnsupportedOutput = errors.New("ptytest: unsupported screen output")

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
// A Screen has a fixed positive size and is not safe for concurrent use. Obtain a
// snapshot from [Transcript.Screen] when output may still be arriving.
type Screen struct {
	size  Size
	cells []screenCell
	at    image.Point
	top   int
	end   int
	scan  ansi.Scanner
}

// NewScreen returns a blank screen-state assertion model.
func NewScreen(size Size) (*Screen, error) {
	if err := size.check(); err != nil {
		return nil, err
	}
	maxInt := int(^uint(0) >> 1)
	if size.Rows > maxInt/size.Cols {
		return nil, fmt.Errorf("ptytest: screen size %dx%d: cell count overflows int", size.Cols, size.Rows)
	}
	return &Screen{
		size:  size,
		cells: make([]screenCell, size.Cols*size.Rows),
		end:   size.Rows,
	}, nil
}

// Size returns the fixed dimensions of the assertion model.
func (s *Screen) Size() Size {
	if s == nil {
		return Size{}
	}
	return s.size
}

// Apply consumes the next renderer-output chunk.
//
// Chunks may split UTF-8 text or an escape sequence. Undecided suffixes are retained
// up to a fixed bound and completed by the next call. Apply either consumes the whole
// chunk or returns at the first unsupported complete piece; earlier complete pieces
// remain applied and callers should stop after an error.
func (s *Screen) Apply(chunk []byte) error {
	if s == nil || len(chunk) == 0 {
		return nil
	}
	err := s.scan.Feed(string(chunk), func(piece ansi.Piece) error {
		if piece.Kind == ansi.Plain {
			if !utf8.ValidString(piece.Raw) {
				return fmt.Errorf("%w: malformed UTF-8", ErrUnsupportedOutput)
			}
			s.writePlain(piece.Raw)
			return nil
		}
		return s.applySequence(piece)
	})
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
	held := s.scan.Pending()
	s.scan.Reset()
	if held == "" {
		return nil
	}
	return fmt.Errorf("%w: incomplete output %q", ErrUnsupportedOutput, held)
}

// At returns the grapheme whose head occupies a zero-based cell. Blank cells,
// trailing halves of wide graphemes, and coordinates outside the screen return empty.
func (s *Screen) At(column, row int) string {
	if s == nil || column < 0 || row < 0 || column >= s.size.Cols || row >= s.size.Rows {
		return ""
	}
	return s.cells[s.index(column, row)].text
}

// Rows returns the visible text as fixed-width rows. Trailing halves of wide
// graphemes contribute no extra bytes: the grapheme itself already occupies both
// cells. The returned slice owns its strings.
func (s *Screen) Rows() []string {
	if s == nil {
		return nil
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
	switch piece.Kind {
	case ansi.Plain:
		s.writePlain(piece.Raw)
		return nil
	case ansi.Control:
		return s.control(piece)
	case ansi.String:
		if piece.Final == ']' {
			return nil // OSC changes metadata or style, never cell text.
		}
	case ansi.Other:
		// Character-set selection is harmless for UTF-8 renderer output. Other
		// unparameterized escapes are not emitted by the cell renderer.
		if strings.HasPrefix(piece.Raw, "\x1b(") {
			return nil
		}
	case ansi.Malformed:
	}
	return fmt.Errorf("%w: %q", ErrUnsupportedOutput, piece.Raw)
}

func (s *Screen) control(piece ansi.Piece) error {
	params := ansi.Parse(piece.Body)
	if !params.Valid() {
		return fmt.Errorf("%w: malformed parameters in %q", ErrUnsupportedOutput, piece.Raw)
	}
	// Queries, mode changes, cursor shape and window metadata are real session
	// traffic but paint no cell. The model ignores the question; it never fabricates
	// the terminal's answer.
	switch piece.Final {
	case 'c', 'h', 'l', 'q', 't':
		return nil
	case 'u':
		if params.Marker() != 0 {
			return nil
		}
	}
	if params.Marker() != 0 {
		return fmt.Errorf("%w: %q", ErrUnsupportedOutput, piece.Raw)
	}
	switch piece.Final {
	case 'm':
		return nil
	case 'H', 'f':
		row := defaultOne(params.At(0)) - 1
		column := defaultOne(params.At(1)) - 1
		s.move(column, row)
	case 'A':
		s.move(s.at.X, s.at.Y-defaultOne(params.First()))
	case 'B':
		s.move(s.at.X, s.at.Y+defaultOne(params.First()))
	case 'C':
		s.move(s.at.X+defaultOne(params.First()), s.at.Y)
	case 'D':
		s.move(s.at.X-defaultOne(params.First()), s.at.Y)
	case 'G':
		s.move(defaultOne(params.First())-1, s.at.Y)
	case 'd':
		s.move(s.at.X, defaultOne(params.First())-1)
	case 'K':
		return s.eraseLine(params.First(), piece.Raw)
	case 'J':
		return s.eraseDisplay(params.First(), piece.Raw)
	case 'S':
		s.scrollUp(defaultOne(params.First()))
	case 'T':
		s.scrollDown(defaultOne(params.First()))
	case 'r':
		return s.setMargins(params, piece.Raw)
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedOutput, piece.Raw)
	}
	return nil
}

func defaultOne(n int) int {
	if n == 0 {
		return 1
	}
	return n
}

func (s *Screen) writePlain(raw string) {
	for raw != "" {
		i := strings.IndexAny(raw, "\b\t\n\r")
		if i < 0 {
			s.writeText(raw)
			return
		}
		s.writeText(raw[:i])
		s.controlByte(raw[i])
		raw = raw[i+1:]
	}
}

func (s *Screen) controlByte(b byte) {
	switch b {
	case '\b':
		s.at.X = max(s.at.X-1, 0)
	case '\t':
		s.at.X = min((s.at.X/8+1)*8, s.size.Cols)
	case '\n':
		s.lineFeed()
	case '\r':
		s.at.X = 0
	}
}

func (s *Screen) writeText(raw string) {
	for _, cluster := range text.Clusters(raw) {
		width := text.Width(cluster)
		if width <= 0 || width > s.size.Cols {
			continue
		}
		if s.at.X >= s.size.Cols || s.at.X+width > s.size.Cols {
			s.at.X = 0
			s.lineFeed()
		}
		s.clearGlyphAt(s.at.X, s.at.Y)
		cell := s.index(s.at.X, s.at.Y)
		s.cells[cell] = screenCell{text: strings.Clone(cluster)}
		for x := 1; x < width; x++ {
			s.clearGlyphAt(s.at.X+x, s.at.Y)
			s.cells[s.index(s.at.X+x, s.at.Y)] = screenCell{trail: true}
		}
		s.at.X += width
	}
}

func (s *Screen) move(x, y int) {
	s.at.X = min(max(x, 0), s.size.Cols-1)
	s.at.Y = min(max(y, 0), s.size.Rows-1)
}

func (s *Screen) lineFeed() {
	if s.at.Y == s.end-1 {
		s.scrollUp(1)
		return
	}
	s.at.Y = min(s.at.Y+1, s.size.Rows-1)
}

func (s *Screen) eraseLine(mode int, raw string) error {
	switch mode {
	case 0:
		s.blank(s.at.Y, s.at.X, s.size.Cols)
	case 1:
		s.blank(s.at.Y, 0, s.at.X+1)
	case 2:
		s.blank(s.at.Y, 0, s.size.Cols)
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedOutput, raw)
	}
	return nil
}

func (s *Screen) eraseDisplay(mode int, raw string) error {
	switch mode {
	case 0:
		s.blank(s.at.Y, s.at.X, s.size.Cols)
		for y := s.at.Y + 1; y < s.size.Rows; y++ {
			s.blank(y, 0, s.size.Cols)
		}
	case 1:
		for y := range s.at.Y {
			s.blank(y, 0, s.size.Cols)
		}
		s.blank(s.at.Y, 0, s.at.X+1)
	case 2:
		clear(s.cells)
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedOutput, raw)
	}
	return nil
}

func (s *Screen) setMargins(params ansi.Params, raw string) error {
	if params.Empty() {
		s.top, s.end = 0, s.size.Rows
		return nil
	}
	top := defaultOne(params.At(0)) - 1
	end := params.At(1)
	if end == 0 {
		end = s.size.Rows
	}
	if top < 0 || end > s.size.Rows || top >= end {
		return fmt.Errorf("%w: %q", ErrUnsupportedOutput, raw)
	}
	s.top, s.end = top, end
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
