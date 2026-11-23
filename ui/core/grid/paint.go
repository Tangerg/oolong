package grid

import (
	"image"
	"slices"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/ansi"
	"github.com/Tangerg/oolong/ui/core/internal/grapheme"
)

const (
	sgrReset = ansi.CSI + "0" + string(ansi.SGR)

	// Rendition changes do not end a grapheme. A relative cursor round trip
	// preserves an atom boundary without assuming the row's screen position.
	atomBoundary = ansi.CSI + string(ansi.CUB) + ansi.CSI + string(ansi.CUF)
)

var (
	osc8Open  = ansi.OSCPrefix(ansi.OSCHyperlink) + ";"
	osc8Close = osc8Open + ansi.ST

	// Synchronized output. A frame wrapped in these is applied by the terminal in
	// one go, which is what keeps a multiplexer from showing a half-drawn frame.
	beginSync = ansi.ModeSynchronizedOutput.Set()
	endSync   = ansi.ModeSynchronizedOutput.Reset()

	showCursor = ansi.ModeCursorVisible.Set()
	hideCursor = ansi.ModeCursorVisible.Reset()
)

// painter owns established style, hyperlink, and cursor state while encoding
// cells. Full redraws, diffs, and scroll repairs share it so their wire state
// transitions cannot diverge.
type painter struct {
	out []byte
	// Color depth is applied only at this wire boundary; stored cells stay
	// truecolor so rendering never depends on the destination palette.
	depth Depth
	wire  wireState
}

// wireState is what the terminal is believed to hold after the bytes in out.
type wireState struct {
	// sourceStyle avoids quantizing an unchanged logical style for every cell;
	// style is the state actually established after that quantization.
	sourceStyle Style
	style       wireStyle
	link        string
	// previous retains the last atom on an uninterrupted text run. Cursor
	// positioning ends that run; SGR and OSC do not.
	previous string
	at       image.Point
	known    bool
	begun    bool
}

func (p *painter) restart() {
	p.out = p.out[:0]
	p.wire = wireState{}
}

func (p *painter) adoptDepth(depth Depth) { p.depth = depth }

// Other programs may change terminal style between frames; establish the baseline before emitting cells.
func (p *painter) begin() {
	if p.wire.begun {
		return
	}
	p.out = append(p.out, osc8Close...)
	p.out = append(p.out, sgrReset...)
	p.wire = wireState{begun: true}
}

// end leaves the terminal with no hyperlink open and the default style, so
// whatever writes next — this program or the shell after it — starts clean.
func (p *painter) end() {
	if !p.wire.begun {
		return
	}
	p.closeLink()
	p.out = append(p.out, sgrReset...)
	p.wire.sourceStyle = Style{}
	p.wire.style = wireStyle{}
}

func (p *painter) adopt(other *painter) {
	begun := p.wire.begun || other.wire.begun
	p.wire = other.wire
	p.wire.begun = begun
}

// forcePos makes the next moveTo emit even if it targets where the cursor is
// believed to be. Sequences whose landing position is terminal-specific — a
// scrolling-region change, for one — leave that belief unfounded.
func (p *painter) forcePos() { p.wire.known = false }

func (p *painter) moveToPoint(at image.Point) { p.moveTo(at.X, at.Y) }

func (p *painter) moveTo(x, y int) {
	if p.wire.known && p.wire.at.X == x && p.wire.at.Y == y {
		return
	}
	p.out = ansi.AppendCSI(p.out, ansi.CUP, y+1, x+1)
	p.wire.at = image.Pt(x, y)
	p.wire.known = true
	p.wire.previous = ""
}

// The cursor must already be positioned at this atom's head.
func (p *painter) cell(c *Cell, width int) {
	content := c.content
	if content == "" {
		content = " "
	}
	if previous := p.wire.previous; previous != "" &&
		(previous[len(previous)-1] >= utf8.RuneSelf || content[0] >= utf8.RuneSelf) && joinsAtoms(previous, content) {
		p.out = append(p.out, atomBoundary...)
	}
	if c.Style != p.wire.sourceStyle {
		p.wire.sourceStyle = c.Style
		next := styleOnWire(c.Style, p.depth)
		if next != p.wire.style {
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
	p.wire.previous = content
	p.wire.at.X += width
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
	p.wire.style = next
	if next == (wireStyle{}) {
		p.out = append(p.out, sgrReset...)
		return
	}
	params := make([]int, 0, maxSGRParams)
	params = append(params, ansi.SGRReset)
	for _, a := range attrCodes {
		if next.attr.Has(a.attr) {
			params = append(params, a.code)
		}
	}
	params = foregroundLayer.append(params, next.fg)
	params = backgroundLayer.append(params, next.bg)
	p.out = ansi.AppendCSI(p.out, ansi.SGR, params...)
}

const (
	// truecolorParams is an extended code, a colour mode and three channels.
	truecolorParams = 5
	// maxSGRParams is a reset, every attribute, and both layers in truecolor.
	maxSGRParams = 1 + len(attrCodes) + 2*truecolorParams
)

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
	if index, ok := color.Palette(); ok {
		return paletteOnWire(index, depth)
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

// paletteOnWire keeps a palette entry an index at every depth that has it, so the
// terminal's theme still decides how it looks; only an entry beyond sixteen
// colours is approximated, and only for a sixteen-colour terminal.
func paletteOnWire(index uint8, depth Depth) wireColor {
	switch {
	case index < cubeStart:
		return wireColor{kind: indexed16WireColor, index: index}
	case depth == Depth16:
		return wireColor{kind: indexed16WireColor, index: PaletteRGB(index).Index16()}
	default:
		return wireColor{kind: indexed256WireColor, index: index}
	}
}

// colorLayer names the SGR codes of one colour layer. Foreground and background
// each have their own standard, bright and extended codes; none is derived from
// another by arithmetic over the ECMA-48 table.
type colorLayer struct{ standard, bright, extended int }

var (
	foregroundLayer = colorLayer{standard: ansi.SGRForegroundFirst, bright: ansi.SGRBrightForegroundFirst, extended: ansi.SGRForeground}
	backgroundLayer = colorLayer{standard: ansi.SGRBackgroundFirst, bright: ansi.SGRBrightBackgroundFirst, extended: ansi.SGRBackground}
)

func (layer colorLayer) append(params []int, color wireColor) []int {
	switch color.kind {
	case indexed16WireColor:
		first := layer.standard
		if color.index >= ansi.ColorsPerIntensity {
			first = layer.bright
		}
		return append(params, first+int(color.index%ansi.ColorsPerIntensity))
	case indexed256WireColor:
		return append(params, layer.extended, ansi.SGRColorIndexed, int(color.index))
	case truecolorWireColor:
		return append(params, layer.extended, ansi.SGRColorRGB, int(color.rgb.R), int(color.rgb.G), int(color.rgb.B))
	default:
		return params
	}
}

// Control bytes can terminate OSC 8 and inject terminal commands; refuse such hyperlink targets.
func (p *painter) appendLink(target string) {
	if target == p.wire.link {
		return
	}
	if p.wire.link != "" {
		p.out = append(p.out, osc8Close...)
	}
	p.wire.link = ""
	if LinkTarget(target) == "" {
		return
	}
	p.out = append(p.out, osc8Open...)
	p.out = append(p.out, target...)
	p.out = append(p.out, ansi.ST...)
	p.wire.link = target
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
	code int
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
		if unicode.IsControl(r) {
			return ""
		}
	}
	return target
}

// EncodeRow uses only relative, position-preserving cursor barriers so separate
// atoms cannot merge on the wire. The caller must position the cursor before
// output to end any preceding grapheme; the row must fit to the terminal margin.
// No absolute screen position is assumed. It closes links and resets style before
// return. Partial atoms become styled blanks. An invalid depth panics before encoding.
func EncodeRow(cells []Cell, depth Depth) string {
	if err := depth.Validate(); err != nil {
		panic(err)
	}
	p := painter{depth: depth}
	p.begin()
	p.paintCells(trimBlankTail(cells))
	p.end()
	return string(p.out)
}

// encodeCells preserves the supplied columns. A reflow chunk can end in default
// blanks that still precede accepted content, so only its caller may trim them.
// The inline compositor has already established default rendition and closed links.
func encodeCells(cells []Cell, depth Depth) string {
	p := painter{depth: depth}
	p.paintCells(cells)
	p.closeLink()
	if p.wire.style != (wireStyle{}) {
		p.out = append(p.out, sgrReset...)
	}
	return string(p.out)
}

func (p *painter) paintCells(cells []Cell) {
	for i := 0; i < len(cells); {
		unit := paintUnitAt(cells, i, len(cells)-i)
		unit.paint(p)
		i += unit.width
	}
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
		p.out = ansi.AppendCSI(p.out, ansi.SU, shift.delta)
	} else {
		p.out = ansi.AppendCSI(p.out, ansi.SD, -shift.delta)
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
	p.out = ansi.AppendCSI(p.out, ansi.DECSTBM, top+1, bottom)
}

func (p *painter) dropScrollRegion() { p.out = ansi.AppendCSI(p.out, ansi.DECSTBM) }
