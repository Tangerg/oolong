package text

import (
	"strings"

	"github.com/Tangerg/oolong/core/ansi"
	"github.com/Tangerg/oolong/core/grid"
)

// Decoder retains lines, styles and incomplete escapes across output chunks.
// It belongs to one interface goroutine and must not be copied after first use;
// background producers post bytes rather than writing into it directly. Supported
// styles and links become [Span]s; other escapes and carriage returns are discarded
// so output cannot issue commands when repainted. Named colors use [grid.PaletteRGB].
type Decoder struct {
	noCopy noCopy

	// Base supplies initial and reset style; zero uses terminal appearance.
	Base grid.Style

	// Keep sequence overrides separate so default foreground resolves through Base.
	state grid.Style
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

// Open returns a borrowed read-only view of the unfinished line. Use [Line.Clone]
// to retain or modify it. New visible input invalidates the cached view.
func (d *Decoder) Open() Line {
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

// Flush ends the stream: the open line, if there is one, and nothing else.
//
// A sequence that never finished is dropped rather than shown, for the reason a
// cell drops a control character — half of a sequence is not text, and printing it
// would print the introducer.
func (d *Decoder) Flush() []Line {
	d.scan.Reset()
	if len(d.runs) == 0 {
		return nil
	}
	return []Line{d.takeLine()}
}

// Reset returns the decoder to where it started, keeping [Decoder.Base]. It is what
// a component reuses one for a second command rather than allocating another.
func (d *Decoder) Reset() {
	d.state = grid.Style{}
	d.link = ""
	d.scan.Reset()
	d.runs = nil
	d.open = nil
	d.dirty = false
}

// Decode is one string of output, whole, in one call.
//
// The base style is what it arrives in before any sequence says otherwise — see
// [Decoder.Base]. This is the form for output that has already finished; anything
// still arriving wants a [Decoder], which is the same reading with the state kept
// between pieces.
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
		// Understood well enough to know where it ended, and nothing this can show.
	}
	return nil
}

// osc reads the one operating system command that says something about the text:
// the hyperlink, which is "8", its parameters, and the address.
//
// An empty address closes the link, which is how the protocol says "the words after
// this point go nowhere". Everything else a command could say — the window title,
// the working directory — is about the terminal rather than about the text, and a
// program's output has no business saying it through here.
func (d *Decoder) osc(body string) {
	command, rest, found := strings.Cut(body, ";")
	if !found || command != "8" {
		return
	}
	// Then the link's own parameters, which nothing here reads, and the address.
	_, target, found := strings.Cut(rest, ";")
	if !found {
		return
	}
	// body may point into the temporary partial-sequence buffer or a much larger
	// caller chunk. A live hyperlink owns only its address.
	d.link = strings.Clone(Printable(target))
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

// append puts a run of text on the end of the open line, joining it to what is
// there when the style has not changed — so a line that arrived in twenty pieces is
// as few spans as it has styles.
func (d *Decoder) append(s string) {
	s = Printable(s)
	if s == "" {
		return
	}
	style := d.Base.Merge(d.state)
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
	line := d.Open()
	d.runs = nil
	d.open = nil
	d.dirty = false
	return line
}

// sgr applies one select-graphic-rendition sequence to the style in force.
//
// A parameter it does not recognise is skipped rather than treated as an error: the
// sequence is a list of independent requests, and one for something a cell cannot
// show — blinking, an overline, a colour under the underline — says nothing about
// the others.
func (d *Decoder) sgr(ps ansi.Params) {
	if ps.Len() == 0 {
		// A bare "ESC [ m" is a reset, which is what an empty parameter defaulting to
		// zero already means.
		d.state = grid.Style{}
		return
	}
	for i := 0; i < ps.Len(); i++ {
		group := ps.Group(i)
		if group.Len() == 0 {
			continue
		}
		switch code := group.At(0); code {
		case 4:
			// The one attribute with a shape of its own: "4:0" is off and every other
			// subparameter is a style of underline a cell cannot tell apart.
			if group.Len() > 1 && group.At(1) == 0 {
				d.state.Attr &^= grid.Underline
				continue
			}
			d.state.Attr |= grid.Underline
		case 38, 48, 58:
			colour, used, ok := colourAt(ps, i, group)
			i += used
			if !ok {
				continue
			}
			switch code {
			case 38:
				d.state.FG = colour
			case 48:
				d.state.BG = colour
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

// colourOf reads the numbers after a 38, a 48 or a 58: a selector, then either one
// palette index or three channels. It reports how many of them it took, whether or
// not they named a colour, so that a malformed one cannot leave its channels to be
// read as attributes.
func colourOf(args []int) (grid.Color, int, bool) {
	if len(args) == 0 {
		return grid.Color{}, 0, false
	}
	switch args[0] {
	case 5:
		if len(args) < 2 {
			return grid.Color{}, len(args), false
		}
		index := args[1]
		if index < 0 || index > 255 {
			return grid.Color{}, 2, false
		}
		return rgb(grid.PaletteRGB(uint8(index))), 2, true
	case 2:
		channels, used := args[1:], 1
		if len(channels) == 4 {
			// The subparameter spelling carries a colour-space identifier before the
			// channels, which is always empty and always ignored.
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
	case code == 0:
		d.state = grid.Style{}
	case code >= 30 && code <= 37:
		d.state.FG = ansiColor(code - 30)
	case code == 39:
		d.state.FG = grid.Color{}
	case code >= 40 && code <= 47:
		d.state.BG = ansiColor(code - 40)
	case code == 49:
		d.state.BG = grid.Color{}
	case code >= 90 && code <= 97:
		d.state.FG = ansiColor(code - 90 + 8)
	case code >= 100 && code <= 107:
		d.state.BG = ansiColor(code - 100 + 8)
	default:
		if attr, on, ok := attribute(code); ok {
			if on {
				d.state.Attr |= attr
			} else {
				d.state.Attr &^= attr
			}
		}
	}
}

// attribute maps a parameter onto one of the six attributes a cell carries, and
// says whether it turns it on or off.
//
// 21 is the doubly-underlined one, which a cell draws as an underline. Some
// terminals read it as "not bold" instead; underlining text that was meant to stop
// being bold is the smaller of the two mistakes, and it is what the standard says.
func attribute(code int) (grid.Attr, bool, bool) {
	switch code {
	case 1:
		return grid.Bold, true, true
	case 2:
		return grid.Dim, true, true
	case 3:
		return grid.Italic, true, true
	case 7:
		return grid.Reverse, true, true
	case 9:
		return grid.Strike, true, true
	case 21:
		return grid.Underline, true, true
	case 22:
		return grid.Bold | grid.Dim, false, true
	case 23:
		return grid.Italic, false, true
	case 24:
		return grid.Underline, false, true
	case 27:
		return grid.Reverse, false, true
	case 29:
		return grid.Strike, false, true
	default:
		return 0, false, false
	}
}

// channel reads one of the three numbers a truecolor parameter carries, refusing
// anything that is not a byte rather than narrowing it to one — a red of 999 is a
// sequence to distrust, not a red of 231.
func channel(v int) (uint8, bool) {
	switch {
	case v < 0, v > 255:
		return 0, false
	default:
		return uint8(v), true
	}
}

// ansiColor is one of the sixteen a terminal names, or nothing for an index that
// is not one of them.
func ansiColor(index int) grid.Color {
	if index < 0 || index >= len(ansiColors) {
		return grid.Color{}
	}
	return ansiColors[index]
}

// ansiColors is what those sixteen resolve to, worked out once. See
// [grid.PaletteRGB] for why the values are xterm's and not a promise.
var ansiColors = func() [16]grid.Color {
	var out [16]grid.Color
	for i := range uint8(len(out)) {
		out[i] = rgb(grid.PaletteRGB(i))
	}
	return out
}()

func rgb(c grid.RGB) grid.Color { return grid.RGBColor(c.R, c.G, c.B) }
