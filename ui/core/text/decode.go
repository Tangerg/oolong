package text

import (
	"slices"
	"strings"

	"github.com/Tangerg/oolong/ui/core/ansi"
	"github.com/Tangerg/oolong/ui/core/grid"
)

// Decoder retains lines, styles and incomplete escapes across output chunks.
// It belongs to one interface goroutine and must not be copied after first use;
// background producers post bytes rather than writing into it directly. Supported
// styles and links become [Span]s; other escapes and carriage returns are discarded
// so output cannot issue commands when repainted. Named colors use [grid.PaletteRGB].
type Decoder struct {
	noCopy noCopy

	// Base supplies defaults for attributes and colors not explicitly overridden by
	// SGR. Changes affect subsequent text; existing spans keep their resolved style.
	Base grid.Style

	state rendition
	link  string
	scan  ansi.Scanner
	// Mutable runs absorb small chunks; Open publishes an immutable view only when needed.
	runs  []decodedRun
	open  Line
	dirty bool
}

// Make the single-owner constraint visible to go vet.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// Private mutable runs can extend without rebuilding a public immutable string.
type decodedRun struct {
	text  []byte
	style grid.Style
	link  string
}

// Feed returns newline-completed lines, retaining incomplete lines and escapes
// until later input or [Decoder.Flush].
func (d *Decoder) Feed(chunk string) []Line {
	if chunk == "" {
		return nil
	}
	var lines []Line
	err := d.scan.Feed(chunk, func(piece ansi.Piece) error {
		lines = append(lines, d.piece(piece)...)
		return nil
	})
	// Scanner consumes oversized escapes completely, so dropping its error cannot leak command text.
	_ = err
	return lines
}

// Open returns a detached snapshot of the unfinished line.
func (d *Decoder) Open() Line { return slices.Clone(d.openLine()) }

func (d *Decoder) openLine() Line {
	if !d.dirty {
		return d.open
	}
	d.open = make(Line, len(d.runs))
	for i, run := range d.runs {
		d.open[i] = Span{Text: string(run.text), Style: run.style, Link: run.link}
	}
	d.dirty = false
	return d.open
}

// Flush ends the stream. An incomplete escape is discarded; a partial UTF-8
// character is now known to be malformed text and follows the usual byte policy.
func (d *Decoder) Flush() []Line {
	if pending := d.scan.Pending(); pending != "" && pending[0] != ansi.Escape {
		d.append(pending)
	}
	d.scan.Reset()
	if len(d.runs) == 0 {
		return nil
	}
	return []Line{d.takeLine()}
}

// Reset discards stream state while preserving Base for the next stream.
func (d *Decoder) Reset() {
	d.state = rendition{}
	d.link = ""
	d.scan.Reset()
	d.runs = nil
	d.open = nil
	d.dirty = false
}

// Decode applies base only where SGR has not supplied an explicit override.
// An unfinished stream requires Decoder so partial sequences keep their state.
func Decode(s string, base grid.Style) []Line {
	d := Decoder{Base: base}
	return append(d.Feed(s), d.Flush()...)
}

func (d *Decoder) piece(p ansi.Piece) []Line {
	switch p.Kind {
	case ansi.Plain:
		return d.write(p.Raw)
	case ansi.Control:
		if p.Final == 'm' && p.Intermediates == "" {
			// A private marker makes this somebody else's command that happens to end
			// in m — XTerm's modifyOtherKeys is CSI > Ps m — and reading its numbers as
			// select-graphic-rendition would style the text that follows by accident.
			if params := ansi.Parse(p.Parameters); params.Marker() == 0 {
				d.sgr(params)
			}
		}
	case ansi.String:
		if p.Final == ']' {
			d.osc(p.Body)
		}
	case ansi.Other, ansi.Malformed:
	}
	return nil
}

// Output hyperlinks may affect text; terminal metadata commands must not survive into repainted content.
func (d *Decoder) osc(body string) {
	command, rest, found := strings.Cut(body, ";")
	if !found || command != "8" {
		return
	}
	_, target, found := strings.Cut(rest, ";")
	if !found {
		return
	}
	// body may point into the temporary partial-sequence buffer or a much larger
	// caller chunk. A live hyperlink owns only its address.
	d.link = strings.Clone(grid.LinkTarget(target))
}

func (d *Decoder) write(s string) []Line {
	var lines []Line
	for {
		before, after, found := strings.Cut(s, "\n")
		d.append(before)
		if !found {
			return lines
		}
		lines = append(lines, d.takeLine())
		s = after
	}
}

// Chunk boundaries must not create additional style spans.
func (d *Decoder) append(s string) {
	s = Printable(s)
	if s == "" {
		return
	}
	style := d.state.resolve(d.Base)
	if n := len(d.runs); n > 0 && d.runs[n-1].style == style && d.runs[n-1].link == d.link {
		d.runs[n-1].text = append(d.runs[n-1].text, s...)
		d.dirty = true
		return
	}
	d.runs = append(d.runs, decodedRun{
		text:  append([]byte(nil), s...),
		style: style,
		link:  d.link,
	})
	d.dirty = true
}

// takeLine transfers the current immutable line to the caller and starts a new
// decoder-owned line. The returned line shares no mutable run storage.
func (d *Decoder) takeLine() Line {
	line := d.openLine()
	d.runs = nil
	d.open = nil
	d.dirty = false
	return line
}

// SGR parameters are independent; unsupported attributes must not discard supported ones.
func (d *Decoder) sgr(ps ansi.Params) {
	if ps.Len() == 0 {
		// A bare "ESC [ m" is a reset, which is what an empty parameter defaulting to
		// zero already means.
		d.state = rendition{}
		return
	}
	for i := 0; i < ps.Len(); i++ {
		group := ps.Group(i)
		if group.Len() == 0 {
			continue
		}
		switch code := group.At(0); code {
		case ansi.SGRUnderline:
			// The one attribute with a shape of its own: "4:0" is off and every other
			// subparameter is a style of underline a cell cannot tell apart.
			if group.Len() > 1 && group.At(1) == ansi.SGRNoUnderline {
				d.state.attributes.set(grid.Underline, false)
				continue
			}
			d.state.attributes.set(grid.Underline, true)
		case ansi.SGRForeground, ansi.SGRBackground, ansi.SGRUnderlineColor:
			colour, used, ok := colourAt(ps, i, group)
			i += used
			if !ok {
				continue
			}
			switch code {
			case ansi.SGRForeground:
				d.state.foreground = colour
			case ansi.SGRBackground:
				d.state.background = colour
			}
			// 58 is the colour of the underline, which a cell has nowhere to keep. It
			// is read this far so that its parameters cannot be mistaken for the next
			// request in the list.
		default:
			d.simple(code)
		}
	}
}

// Both extended-color parameter encodings share one reader; only their parameter locations differ.
func colourAt(ps ansi.Params, i int, group ansi.Parameter) (grid.Color, int, bool) {
	if group.Len() > 1 {
		args := make([]int, 0, group.Len()-1)
		for j := 1; j < group.Len(); j++ {
			args = append(args, group.At(j))
		}
		c, _, ok := colourOf(args)
		return c, 0, ok
	}
	args := make([]int, 0, 4)
	for k := i + 1; k < ps.Len() && len(args) < cap(args); k++ {
		args = append(args, ps.At(k))
	}
	c, used, ok := colourOf(args)
	return c, used, ok
}

// Consume every color parameter even when malformed, or channels can be misread as SGR attributes.
func colourOf(args []int) (grid.Color, int, bool) {
	if len(args) == 0 {
		return grid.Color{}, 0, false
	}
	switch args[0] {
	case ansi.SGRColorIndexed:
		if len(args) < 2 {
			return grid.Color{}, len(args), false
		}
		index := args[1]
		if index < 0 || index > 255 {
			return grid.Color{}, 2, false
		}
		return rgb(grid.PaletteRGB(uint8(index))), 2, true
	case ansi.SGRColorRGB:
		channels, used := args[1:], 1
		if len(channels) == 4 {
			// The colon form reserves a color-space field before RGB. Skip it so
			// it cannot be misread as the red channel.
			channels, used = channels[1:], 2
		}
		if len(channels) < 3 {
			return grid.Color{}, len(args), false
		}
		used += 3
		r, okR := channel(channels[0])
		g, okG := channel(channels[1])
		b, okB := channel(channels[2])
		if !okR || !okG || !okB {
			return grid.Color{}, used, false
		}
		return grid.RGBColor(r, g, b), used, true
	default:
		return grid.Color{}, 1, false
	}
}

func (d *Decoder) simple(code int) {
	switch {
	case code == ansi.SGRReset:
		d.state = rendition{}
	case code >= ansi.SGRForegroundFirst && code <= ansi.SGRForegroundLast:
		d.state.foreground = ansiColor(code - ansi.SGRForegroundFirst)
	case code == ansi.SGRForegroundDefault:
		d.state.foreground = grid.Color{}
	case code >= ansi.SGRBackgroundFirst && code <= ansi.SGRBackgroundLast:
		d.state.background = ansiColor(code - ansi.SGRBackgroundFirst)
	case code == ansi.SGRBackgroundDefault:
		d.state.background = grid.Color{}
	case code >= ansi.SGRBrightForegroundFirst && code <= ansi.SGRBrightForegroundLast:
		d.state.foreground = ansiColor(code - ansi.SGRBrightForegroundFirst + ansi.ColorsPerIntensity)
	case code >= ansi.SGRBrightBackgroundFirst && code <= ansi.SGRBrightBackgroundLast:
		d.state.background = ansiColor(code - ansi.SGRBrightBackgroundFirst + ansi.ColorsPerIntensity)
	default:
		if attr, on, ok := attribute(code); ok {
			d.state.attributes.set(attr, on)
		}
	}
}

// Use the specified underline meaning for SGR 21; treating it as not-bold would erase unrelated emphasis.
func attribute(code int) (grid.Attr, bool, bool) {
	switch code {
	case ansi.SGRBold:
		return grid.Bold, true, true
	case ansi.SGRFaint:
		return grid.Dim, true, true
	case ansi.SGRItalic:
		return grid.Italic, true, true
	case ansi.SGRReverse:
		return grid.Reverse, true, true
	case ansi.SGRStrike:
		return grid.Strike, true, true
	case ansi.SGRDoubleUnderline:
		return grid.Underline, true, true
	case ansi.SGRNormalIntensity:
		return grid.Bold | grid.Dim, false, true
	case ansi.SGRNotItalic:
		return grid.Italic, false, true
	case ansi.SGRNotUnderlined:
		return grid.Underline, false, true
	case ansi.SGRNotReversed:
		return grid.Reverse, false, true
	case ansi.SGRNotStruckOut:
		return grid.Strike, false, true
	default:
		return 0, false, false
	}
}

// Reject out-of-byte-range channels before narrowing; conversion would turn
// a malformed value into a different, valid color.
func channel(v int) (uint8, bool) {
	switch {
	case v < 0, v > 255:
		return 0, false
	default:
		return uint8(v), true
	}
}

func ansiColor(index int) grid.Color {
	if index < 0 || index >= len(ansiColors) {
		return grid.Color{}
	}
	return ansiColors[index]
}

// Resolve the named palette once through grid's color authority.
var ansiColors = func() [16]grid.Color {
	var out [16]grid.Color
	for i := range uint8(len(out)) {
		out[i] = rgb(grid.PaletteRGB(i))
	}
	return out
}()

func rgb(c grid.RGB) grid.Color { return grid.RGBColor(c.R, c.G, c.B) }
