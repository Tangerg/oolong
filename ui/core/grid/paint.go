package grid

import (
	"image"
	"slices"
	"strconv"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/ansi"
	"github.com/Tangerg/oolong/ui/core/internal/grapheme"
)

const (
	sgrReset  = "\x1b[0m"
	osc8Open  = "\x1b]8;;"
	osc8Close = "\x1b]8;;\x1b\\"
	stringEnd = "\x1b\\"

	// Rendition changes do not end a grapheme. A relative cursor round trip
	// preserves an atom boundary without assuming the row's screen position.
	atomBoundary = "\x1b[D\x1b[C"

	// Synchronized output. A frame wrapped in these is applied by the terminal in
	// one go, which is what keeps a multiplexer from showing a half-drawn frame.
	beginSync = "\x1b[?2026h"
	endSync   = "\x1b[?2026l"
)

// painter owns established style, hyperlink, and cursor state while encoding
// cells. Full redraws, diffs, and scroll repairs share it so their wire state
// transitions cannot diverge.
type painter struct {
	out []byte
	// sourceStyle avoids quantizing an unchanged logical style for every cell;
	// style is the state actually established after that quantization.
	sourceStyle Style
	style       wireStyle
	link        string
	// Color depth is applied only at this wire boundary; stored cells stay
	// truecolor so rendering never depends on the destination palette.
	depth Depth
	// previous retains the last atom on an uninterrupted text run. Cursor
	// positioning ends that run; SGR and OSC do not.
	previous string
	at       image.Point
	known    bool
	begun    bool
}

func (p *painter) restart() {
	p.out = p.out[:0]
	p.sourceStyle = Style{}
	p.style = wireStyle{}
	p.link = ""
	p.previous = ""
	p.known = false
	p.begun = false
}

func (p *painter) adoptDepth(depth Depth) { p.depth = depth }

// Other programs may change terminal style between frames; establish the baseline before emitting cells.
func (p *painter) begin() {
	if p.begun {
		return
	}
	p.begun = true
	p.out = append(p.out, osc8Close...)
	p.out = append(p.out, sgrReset...)
	p.sourceStyle = Style{}
	p.style = wireStyle{}
	p.known = false
	p.previous = ""
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

func (p *painter) adopt(other *painter) {
	p.sourceStyle = other.sourceStyle
	p.style = other.style
	p.link = other.link
	p.previous = other.previous
	p.at = other.at
	p.known = other.known
	p.begun = p.begun || other.begun
}

// forcePos makes the next moveTo emit even if it targets where the cursor is
// believed to be. Sequences whose landing position is terminal-specific — a
// scrolling-region change, for one — leave that belief unfounded.
func (p *painter) forcePos() { p.known = false }

func (p *painter) moveToPoint(at image.Point) { p.moveTo(at.X, at.Y) }

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
	p.previous = ""
}

// The cursor must already be positioned at this atom's head.
func (p *painter) cell(c *Cell, width int) {
	content := c.content
	if content == "" {
		content = " "
	}
	if previous := p.previous; previous != "" &&
		(previous[len(previous)-1] >= utf8.RuneSelf || content[0] >= utf8.RuneSelf) && joinsAtoms(previous, content) {
		p.out = append(p.out, atomBoundary...)
	}
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
	p.previous = content
	p.at.X += width
}

func joinsAtoms(left, right string) bool {
	clusters := grapheme.New(left + right)
	for clusters.Next() {
		if clusters.End() >= len(left) {
			return clusters.End() > len(left)
		}
	}
	return false
}

// Reset complete styles because terminals disagree on per-attribute reset behavior.
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
		p.appendColor(ansi.SGRForeground, next.fg)
	}
	if next.bg.kind != defaultWireColor {
		p.appendColor(ansi.SGRBackground, next.bg)
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

func (p *painter) appendColor(base int64, color wireColor) {
	switch color.kind {
	case defaultWireColor:
		// appendStyle excludes defaults; keep the representation exhaustive so a
		// new wire colour kind cannot silently acquire truecolor semantics.
	case indexed16WireColor:
		index := color.index
		code := base - ansi.ColorsPerIntensity + int64(index%ansi.ColorsPerIntensity)
		if index >= ansi.ColorsPerIntensity {
			code += ansi.SGRBrightForegroundFirst - ansi.SGRForegroundFirst
		}
		p.out = append(p.out, ';')
		p.out = strconv.AppendInt(p.out, code, 10)

	case indexed256WireColor:
		p.out = append(p.out, ';')
		p.out = strconv.AppendInt(p.out, base, 10)
		p.out = append(p.out, ';')
		p.out = strconv.AppendInt(p.out, ansi.SGRColorIndexed, 10)
		p.out = append(p.out, ';')
		p.out = strconv.AppendInt(p.out, int64(color.index), 10)

	case truecolorWireColor:
		p.out = append(p.out, ';')
		p.out = strconv.AppendInt(p.out, base, 10)
		p.out = append(p.out, ';')
		p.out = strconv.AppendInt(p.out, ansi.SGRColorRGB, 10)
		p.out = append(p.out, ';')
		p.out = strconv.AppendInt(p.out, int64(color.rgb.R), 10)
		p.out = append(p.out, ';')
		p.out = strconv.AppendInt(p.out, int64(color.rgb.G), 10)
		p.out = append(p.out, ';')
		p.out = strconv.AppendInt(p.out, int64(color.rgb.B), 10)
	}
}

// Control bytes can terminate OSC 8 and inject terminal commands; refuse such hyperlink targets.
func (p *painter) appendLink(target string) {
	if target == p.link {
		return
	}
	if p.link != "" {
		p.out = append(p.out, osc8Close...)
	}
	p.link = ""
	if LinkTarget(target) == "" {
		return
	}
	p.out = append(p.out, osc8Open...)
	p.out = append(p.out, target...)
	p.out = append(p.out, stringEnd...)
	p.link = target
}

func (p *painter) closeLink() { p.appendLink("") }

// Dirty checks must treat a wide glyph and all continuation cells as one atom; continuations alone paint nothing.
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

func changedAgainst(prev, next *Surface) func(i, width int) bool {
	return func(i, width int) bool {
		return !slices.Equal(next.cells[i:i+width], prev.cells[i:i+width])
	}
}

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
	width := 1
	limit := min(cell.Width(), remaining, len(cells)-i)
	for width < limit && cells[i+width].span == span(-width) {
		width++
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

func everything(int, int) bool { return true }

var attrCodes = [...]struct {
	attr Attr
	code int64
}{
	{Bold, ansi.SGRBold},
	{Dim, ansi.SGRFaint},
	{Italic, ansi.SGRItalic},
	{Underline, ansi.SGRUnderline},
	{Reverse, ansi.SGRReverse},
	{Strike, ansi.SGRStrike},
}

// LinkTarget preserves an OSC 8 destination verbatim or returns the empty
// destination. Removing individual controls could create a different address.
func LinkTarget(target string) string {
	if !utf8.ValidString(target) {
		return ""
	}
	for _, r := range target {
		if r < 0x20 || r >= 0x7f && r <= 0x9f {
			return ""
		}
	}
	return target
}

// EncodeRow uses only relative, position-preserving cursor barriers so separate
// atoms cannot merge on the wire. The caller must position the cursor before
// output to end any preceding grapheme; the row must fit to the terminal margin.
// No absolute screen position is assumed. It closes links and resets style before
// return. Partial atoms become styled blanks.
func EncodeRow(cells []Cell, depth Depth) string {
	return encodeCells(trimBlankTail(cells), depth)
}

// encodeCells preserves the supplied columns. A reflow chunk can end in default
// blanks that still precede accepted content, so only its caller may trim them.
func encodeCells(cells []Cell, depth Depth) string {
	p := painter{depth: depth}
	for i := 0; i < len(cells); {
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

// Trailing default blanks can wrap the terminal cursor. Styled blanks and wide-glyph continuations must remain.
func trimBlankTail(cells []Cell) []Cell {
	end := len(cells)
	for end > 0 && cells[end-1] == (Cell{}) {
		end--
	}
	return cells[:end]
}

// Reset before scrolling so background-aware erasure produces the default blanks used by the diff.
func (p *painter) scroll(next *Surface, shift verticalShift) {
	p.begin()
	region := shift.top != 0 || shift.bottom != next.h
	if region {
		p.setScrollRegion(shift.top, shift.bottom)
	}
	if shift.delta > 0 {
		p.csi(shift.delta, 'S')
	} else {
		p.csi(-shift.delta, 'T')
	}
	if region {
		// Margins are dropped straight away: where the cursor ends up after a
		// scrolling-region change differs between terminals, so every write from
		// here on positions itself absolutely.
		p.dropScrollRegion()
	}
	p.forcePos()

	y0, y1 := shift.exposed()
	p.paint(next, y0, y1, changedAgainstBlank(next))
}

func (p *painter) setScrollRegion(top, bottom int) {
	p.out = append(p.out, '\x1b', '[')
	p.out = strconv.AppendInt(p.out, int64(top)+1, 10)
	p.out = append(p.out, ';')
	p.out = strconv.AppendInt(p.out, int64(bottom), 10)
	p.out = append(p.out, 'r')
}

func (p *painter) dropScrollRegion() { p.out = append(p.out, "\x1b[r"...) }

func (p *painter) csi(n int, final byte) {
	p.out = appendCSI(p.out, n, final)
}
