package grid

import (
	"image"
	"slices"
	"strconv"
)

const (
	sgrReset  = "\x1b[0m"
	osc8Open  = "\x1b]8;;"
	osc8Close = "\x1b]8;;\x1b\\"
	stringEnd = "\x1b\\"

	// Synchronized output. A frame wrapped in these is applied by the terminal in
	// one go, which is what keeps a multiplexer from showing a half-drawn frame.
	beginSync = "\x1b[?2026h"
	endSync   = "\x1b[?2026l"
)

// painter accumulates one frame's escape stream while tracking the terminal
// state it has already established — style, hyperlink and cursor column — so
// nothing is re-stated that is already true.
//
// It exists because every way of emitting cells wants exactly this bookkeeping:
// a full repaint, a diff, and the rows a scroll exposed differ only in which
// cells they consider dirty, not in how a cell reaches the wire.
type painter struct {
	out []byte
	// sourceStyle avoids quantizing an unchanged logical style for every cell;
	// style is the state actually established after that quantization.
	sourceStyle Style
	style       wireStyle
	link        string
	// depth is how much colour the terminal is being asked to show. It is applied
	// here and nowhere else: a frame is built in truecolor all the way down, and
	// this is the one place a style becomes bytes.
	depth Depth
	// at is where the terminal's cursor sits, and known says whether that is
	// worth believing. A run of adjacent cells needs no positioning.
	at    image.Point
	known bool
	// begun records that the frame has stated its style baseline, which also
	// means the frame is going to write something.
	begun bool
}

// restart empties the buffer and forgets everything established for the previous
// frame.
func (p *painter) restart() {
	p.out = p.out[:0]
	p.sourceStyle = Style{}
	p.style = wireStyle{}
	p.link = ""
	p.known = false
	p.begun = false
}

// adoptDepth carries the colour depth over to another painter, so a frame built
// out of more than one of them speaks with one voice.
func (p *painter) adoptDepth(depth Depth) { p.depth = depth }

// begin states the default style, once per frame, before the first cell.
//
// The terminal's style at frame start is not knowable — another program may have
// written to it — so a frame that writes anything begins by making it knowable.
func (p *painter) begin() {
	if p.begun {
		return
	}
	p.begun = true
	p.out = append(p.out, sgrReset...)
	p.sourceStyle = Style{}
	p.style = wireStyle{}
	p.known = false
}

// end leaves the terminal with no hyperlink open and the default style, so
// whatever writes next — this program or the shell after it — starts clean.
func (p *painter) end() {
	if !p.begun {
		return
	}
	p.closeLink()
	p.out = append(p.out, sgrReset...)
	p.sourceStyle = Style{}
	p.style = wireStyle{}
}

// adopt continues from the terminal state another painter established.
func (p *painter) adopt(other *painter) {
	p.sourceStyle = other.sourceStyle
	p.style = other.style
	p.link = other.link
	p.at = other.at
	p.known = other.known
	p.begun = p.begun || other.begun
}

// forcePos makes the next moveTo emit even if it targets where the cursor is
// believed to be. Sequences whose landing position is terminal-specific — a
// scrolling-region change, for one — leave that belief unfounded.
func (p *painter) forcePos() { p.known = false }

// moveToPoint positions the cursor at a point, for a caller that has one — which is
// what a region something else paints is placed by.
func (p *painter) moveToPoint(at image.Point) { p.moveTo(at.X, at.Y) }

// moveTo positions the cursor unless it is already there.
func (p *painter) moveTo(x, y int) {
	if p.known && p.at.X == x && p.at.Y == y {
		return
	}
	p.out = append(p.out, '\x1b', '[')
	p.out = strconv.AppendInt(p.out, int64(y)+1, 10)
	p.out = append(p.out, ';')
	p.out = strconv.AppendInt(p.out, int64(x)+1, 10)
	p.out = append(p.out, 'H')
	p.at = image.Pt(x, y)
	p.known = true
}

// cell writes one cell unit spanning width columns, emitting only the state
// changes it needs first. It assumes the cursor is already in place.
func (p *painter) cell(c *Cell, width int) {
	if c.Style != p.sourceStyle {
		p.sourceStyle = c.Style
		next := styleOnWire(c.Style, p.depth)
		if next != p.style {
			p.appendStyle(next)
		}
	}
	p.appendLink(c.Link)
	if c.content == "" {
		for range width {
			p.out = append(p.out, ' ')
		}
	} else {
		p.out = append(p.out, c.content...)
	}
	p.at.X += width
}

// appendStyle emits the SGR that establishes next.
//
// Reset-then-set rather than a minimal difference: one deterministic sequence per
// change is shorter to reason about than a per-attribute diff, and terminals
// disagree about how to turn individual attributes off.
func (p *painter) appendStyle(next wireStyle) {
	p.style = next
	if next == (wireStyle{}) {
		p.out = append(p.out, sgrReset...)
		return
	}
	p.out = append(p.out, "\x1b[0"...)
	for _, a := range attrCodes {
		if next.attr.Has(a.attr) {
			p.out = append(p.out, ';')
			p.out = strconv.AppendInt(p.out, a.code, 10)
		}
	}
	if next.fg.kind != defaultWireColor {
		p.appendColor(foreground, next.fg)
	}
	if next.bg.kind != defaultWireColor {
		p.appendColor(background, next.bg)
	}
	p.out = append(p.out, 'm')
}

// wireStyle is the terminal state established by one SGR at a particular depth.
// It deliberately is not Style: two truecolor values can become one palette index,
// and NoColor drops both colours. Tracking the representation the terminal receives
// makes equality mean that another SGR would have no effect.
type wireStyle struct {
	fg, bg wireColor
	attr   Attr
}

type wireColor struct {
	kind  wireColorKind
	rgb   RGB
	index uint8
}

type wireColorKind uint8

const (
	defaultWireColor wireColorKind = iota
	truecolorWireColor
	indexed256WireColor
	indexed16WireColor
)

func styleOnWire(style Style, depth Depth) wireStyle {
	return wireStyle{
		fg:   colorOnWire(style.FG, depth),
		bg:   colorOnWire(style.BG, depth),
		attr: attrsOnWire(style.Attr),
	}
}

func attrsOnWire(attr Attr) Attr {
	var supported Attr
	for _, code := range attrCodes {
		supported |= attr & code.attr
	}
	return supported
}

func colorOnWire(color Color, depth Depth) wireColor {
	if color.Default() || depth == NoColor {
		return wireColor{}
	}
	switch depth {
	case Depth16:
		return wireColor{kind: indexed16WireColor, index: color.rgb.Index16()}
	case Depth256:
		return wireColor{kind: indexed256WireColor, index: color.rgb.Index256()}
	default:
		return wireColor{kind: truecolorWireColor, rgb: color.rgb}
	}
}

// The SGR bases for the two colours a cell has. The sixteen-colour forms are
// numbered from them rather than written out: 30 and 40 begin the ANSI eight, and
// 90 and 100 begin their bright counterparts.
const (
	foreground int64 = 38
	background int64 = 48
)

// appendColor writes one colour after depth has chosen its terminal representation.
//
// The parameters are appended to an SGR that is already open, which is why every
// branch starts with a semicolon and none of them ends the sequence.
func (p *painter) appendColor(base int64, color wireColor) {
	switch color.kind {
	case defaultWireColor:
		// appendStyle excludes defaults; keep the representation exhaustive so a
		// new wire colour kind cannot silently acquire truecolor semantics.
	case indexed16WireColor:
		index := color.index
		// 30–37 and 40–47 are the eight; 90–97 and 100–107 are the bright ones,
		// which are 60 above their plain forms in both cases.
		code := base - 8 + int64(index%8)
		if index >= 8 {
			code += 60
		}
		p.out = append(p.out, ';')
		p.out = strconv.AppendInt(p.out, code, 10)

	case indexed256WireColor:
		p.out = append(p.out, ';')
		p.out = strconv.AppendInt(p.out, base, 10)
		p.out = append(p.out, ";5;"...)
		p.out = strconv.AppendInt(p.out, int64(color.index), 10)

	case truecolorWireColor:
		p.out = append(p.out, ';')
		p.out = strconv.AppendInt(p.out, base, 10)
		p.out = append(p.out, ";2;"...)
		p.out = strconv.AppendInt(p.out, int64(color.rgb.R), 10)
		p.out = append(p.out, ';')
		p.out = strconv.AppendInt(p.out, int64(color.rgb.G), 10)
		p.out = append(p.out, ';')
		p.out = strconv.AppendInt(p.out, int64(color.rgb.B), 10)
	}
}

// appendLink closes the open hyperlink and opens target.
//
// A target carrying control bytes is dropped rather than escaped: the OSC 8
// payload is terminated by a control byte, so a target containing one could close
// the sequence early and have the rest read as terminal commands. Cells can be
// filled from tool output, which makes this a trust boundary.
func (p *painter) appendLink(target string) {
	if target == p.link {
		return
	}
	if p.link != "" {
		p.out = append(p.out, osc8Close...)
	}
	p.link = ""
	if target == "" || !printableTarget(target) {
		return
	}
	p.out = append(p.out, osc8Open...)
	p.out = append(p.out, target...)
	p.out = append(p.out, stringEnd...)
	p.link = target
}

// closeLink leaves no hyperlink open at the end of a frame or a row.
func (p *painter) closeLink() { p.appendLink("") }

// paint emits every cell of rows [y0, y1) that dirty accepts, positioning the
// cursor only where a run breaks.
//
// dirty is given the index of the unit's head cell and its width so it can judge
// a multi-column atom as one thing: if any column changed, the head glyph has to
// be rewritten, and repainting only a continuation would print nothing.
func (p *painter) paint(s *Surface, y0, y1 int, dirty func(i, width int) bool) {
	for y := max(y0, 0); y < min(y1, s.h); y++ {
		for x := 0; x < s.w; {
			i := y*s.w + x
			unit := paintUnitAt(s.cells, i, s.w-x)
			if !dirty(i, unit.width) {
				x += unit.width
				continue
			}
			p.begin()
			p.moveTo(x, y)
			unit.paint(p)
			x += unit.width
		}
	}
}

// changedAgainst reports cells that differ from the same position in prev.
func changedAgainst(prev, next *Surface) func(i, width int) bool {
	return func(i, width int) bool {
		return !slices.Equal(next.cells[i:i+width], prev.cells[i:i+width])
	}
}

// changedAgainstBlank reports cells that are not already blank, for rows the
// terminal has just cleared for us.
func changedAgainstBlank(next *Surface) func(i, width int) bool {
	return func(i, width int) bool {
		for offset := range width {
			if next.cells[i+offset] != (Cell{}) {
				return true
			}
		}
		return false
	}
}

// paintUnit is one atom as it can be represented by the cells available to a
// painter. A complete unit emits its head; a truncated or malformed unit emits
// styled blanks for exactly the columns it owns. That distinction keeps the
// painter's cursor accounting and the terminal's advance identical.
type paintUnit struct {
	cell  *Cell
	width int
}

func paintUnitAt(cells []Cell, i, remaining int) paintUnit {
	cell := &cells[i]
	available := min(remaining, len(cells)-i)
	width := cell.Width()
	if width < 1 {
		width = 1
	} else if width > available {
		width = available
	}
	return paintUnit{cell: cell, width: width}
}

func (u paintUnit) complete() bool { return u.cell.Width() == u.width }

func (u paintUnit) paint(p *painter) {
	if u.complete() {
		p.cell(u.cell, u.width)
		return
	}
	blank := Cell{Style: u.cell.Style}
	p.cell(&blank, u.width)
}

func (u paintUnit) byteCost() int {
	if u.complete() && u.cell.content != "" {
		return len(u.cell.content)
	}
	return u.width
}

// everything accepts every cell, for a full repaint.
func everything(int, int) bool { return true }

var attrCodes = [...]struct {
	attr Attr
	code int64
}{
	{Bold, 1},
	{Dim, 2},
	{Italic, 3},
	{Underline, 4},
	{Reverse, 7},
	{Strike, 9},
}

func printableTarget(target string) bool {
	for i := range len(target) {
		if target[i] < 0x20 || target[i] == 0x7f {
			return false
		}
	}
	return true
}

// EncodeRow renders one row of cells as inline terminal text: style and
// hyperlink transitions and printable graphemes, and nothing that moves the
// cursor or erases anything.
//
// It is how a finished transcript line is printed into the terminal's own
// scrollback, where the line must survive on its own with no screen to address.
// The result always closes an open hyperlink and returns to the default style, so
// rows can be concatenated safely. If cells ends inside a multi-column display
// atom, EncodeRow emits styled blanks for the visible columns rather than a
// partial atom that would advance the terminal beyond the slice.
func EncodeRow(cells []Cell, depth Depth) string {
	cells = trimBlankTail(cells)
	p := painter{depth: depth}
	for i := 0; i < len(cells); {
		c := &cells[i]
		if c.span < 0 {
			i++
			continue
		}
		unit := paintUnitAt(cells, i, len(cells)-i)
		unit.paint(&p)
		i += unit.width
	}
	p.closeLink()
	if p.style != (wireStyle{}) {
		p.out = append(p.out, sgrReset...)
	}
	return string(p.out)
}

// trimBlankTail drops the run of wholly default blank cells at the end of a row.
//
// Printing them costs bytes for nothing, and on a row as wide as the terminal it
// pushes the cursor onto the next line before the caller asked for one. Only the
// zero cell is dropped: a blank carrying a background colour or a hyperlink is
// visible, and a continuation column must stay with its atom's head.
func trimBlankTail(cells []Cell) []Cell {
	end := len(cells)
	for end > 0 && cells[end-1] == (Cell{}) {
		end--
	}
	return cells[:end]
}
