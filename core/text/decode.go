package text

import (
	"strings"

	"github.com/Tangerg/oolong/core/ansi"
	"github.com/Tangerg/oolong/core/grid"
)

// Decoder turns terminal output back into styled text: the escape sequences a
// program wrote to colour its output become [Span]s, and everything else is
// dropped.
//
// This is the one direction that was missing. An interface that runs commands is
// handed their output, and their output is coloured; a cell refuses control
// characters at the boundary, on purpose, so without this every caller has either
// to strip the colour and lose it or to write this again.
//
// # Reading a stream
//
// Output arrives in whatever pieces a read produced, and neither a line nor a
// sequence respects those boundaries. [Decoder.Feed] answers with the lines a
// newline has finished, holds the rest, and carries the style in force from one
// piece to the next — so a colour opened in one chunk still applies in the next,
// and a sequence split down the middle is not read as text. [Decoder.Open] is the
// line still being written, which is what a live interface draws while the rest of
// it is still coming.
//
// A decoder belongs to one goroutine, like everything else here. It is deliberately
// not an [io.Writer]: something wired to a command's standard output is written to
// from whatever goroutine is waiting on that command, and this library has exactly
// one that may touch what is on screen. Read the pipe there, post the chunk, decode
// it here.
//
// # What is read and what is not
//
// Colour and the six attributes a cell can carry, which is all a cell has — and the
// hyperlink a terminal was told about, which is the one thing in the stream that
// says where a piece of text points. Every other sequence is consumed and dropped,
// and dropped is the point: it neither reaches a cell, where it would be obeyed on
// the next repaint, nor shows up as its own text.
//
// A carriage return is dropped rather than obeyed. Obeying it — and the cursor
// movement and erasure beside it, which is what a progress bar rewriting its line
// is made of — is a terminal emulator, which is another product and not this one.
// What that costs is visible and bounded: output that redrew a line in place reads
// as the several versions of it, one after another.
//
// The sixteen colours a terminal names rather than numbers are resolved through
// [grid.PaletteRGB], because a [grid.Color] is either a number or the terminal's own
// and there is nothing in between to hold "the user's idea of red". The values are
// xterm's, which is what a terminal that was never themed shows.
//
// A Decoder must not be copied after its first use.
type Decoder struct {
	// Base is what text arrives in before any sequence says otherwise, and what a
	// reset goes back to. It is how output is drawn in the interface's own body
	// style while still being recoloured by whatever wrote it.
	//
	// The zero value is the terminal's own appearance, which is right for output
	// shown on its own and wrong inside a themed pane.
	Base grid.Style

	// state is what the sequences seen so far asked for, over the base rather than
	// merged into it. Keeping the two apart is what lets "default foreground" mean
	// the base's foreground rather than the terminal's.
	state grid.Style
	// link is the address the output last opened, and "" between them.
	link string
	// held is a sequence that has begun and not ended, waiting for the rest of it.
	held strings.Builder
	// runs own the bytes of the line no newline has ended yet. Feed grows them with
	// amortised cost; Open materialises their immutable public form only when asked.
	runs  []decodedRun
	open  Line
	dirty bool
}

// decodedRun is the mutable, decoder-owned form of a [Span]. Keeping it private
// lets many small reads extend one run without rebuilding the immutable string a
// caller sees.
type decodedRun struct {
	text  []byte
	style grid.Style
	link  string
}

// maxHeld bounds what an unfinished sequence may hold.
//
// A sequence that has not ended within this many bytes is not one, and the bytes
// are dropped. Without a bound, output that opened a string command and never
// closed it would grow memory for as long as it kept arriving — and the cost of
// getting the bound wrong is one dropped sequence, against a program that fills
// its own address space.
const maxHeld = 1 << 16

// Feed takes another piece of the output and returns the lines a newline finished.
//
// What is left over stays in the decoder: the line no newline has ended yet — see
// [Decoder.Open] — and a sequence that arrived only in part. Nothing is lost by
// stopping in the middle of either.
//
// Feed follows the ordinary streaming-decoder shape: hand over the next piece,
// take back what is now decidable, and let [Decoder.Flush] settle what only the
// end of the stream can.
func (d *Decoder) Feed(chunk string) []Line {
	if chunk == "" {
		return nil
	}

	// A partial sequence is normally tiny, but may arrive one byte at a time. A
	// Builder makes that a growing buffer rather than one new string per byte. Keep
	// an offset into the stable source: mutating held while a scanner result still
	// points into it would violate the Builder's ownership contract.
	source := chunk
	buffered := d.held.Len() > 0
	if buffered {
		d.held.WriteString(chunk)
		source = d.held.String()
	}

	var lines []Line
	for at := 0; at < len(source); {
		piece, n, ok := ansi.Next(source[at:])
		if !ok {
			tail := source[at:]
			if len(tail) > maxHeld {
				// It has stopped being a sequence and started being a leak. All of what
				// is left is that one sequence — everything decodable before it has
				// already been read — so dropping it drops the sequence and nothing
				// else.
				d.held.Reset()
				return lines
			}
			if buffered && at == 0 {
				// The Builder already owns precisely this unfinished sequence. Leaving it
				// in place is what makes one-byte input amortised linear time.
				return lines
			}
			// tail may be a substring at the end of either a caller-owned chunk or a
			// buffer whose decoded prefix should be released. Retain exactly the tail.
			d.hold(tail)
			return lines
		}
		at += n
		lines = append(lines, d.piece(piece)...)
	}
	d.held.Reset()
	return lines
}

// hold replaces the undecided sequence with a detached copy. s is allowed to
// refer to held's current allocation; Reset releases ownership without changing
// those immutable bytes before WriteString reads them.
func (d *Decoder) hold(s string) {
	d.held.Reset()
	d.held.Grow(len(s))
	d.held.WriteString(s)
}

// Open is the line still being written: everything decoded since the last newline.
//
// It is what an interface draws while the rest is still arriving. The slice is the
// decoder's own and is replaced as more arrives, so anything keeping it past the
// next call keeps a copy.
func (d *Decoder) Open() Line { return d.materialise() }

// Flush ends the stream: the open line, if there is one, and nothing else.
//
// A sequence that never finished is dropped rather than shown, for the reason a
// cell drops a control character — half of a sequence is not text, and printing it
// would print the introducer.
func (d *Decoder) Flush() []Line {
	d.held.Reset()
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
	d.held.Reset()
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

// piece deals with one scanned piece of the stream.
func (d *Decoder) piece(p ansi.Piece) []Line {
	switch p.Kind {
	case ansi.Plain:
		return d.write(p.Raw)
	case ansi.Control:
		if p.Final == 'm' {
			d.sgr(ansi.Parse(p.Body))
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
	d.link = strings.Clone(printable(target))
}

// write adds text to the open line, breaking a line at every newline.
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
	s = printable(s)
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

// materialise returns the immutable public view of the open runs. Repeated Open
// calls without new input return the same view; Feed invalidates it only when it
// adds visible text.
func (d *Decoder) materialise() Line {
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

// takeLine transfers the current immutable line to the caller and starts a new
// decoder-owned line. The returned line shares no mutable run storage.
func (d *Decoder) takeLine() Line {
	line := d.materialise()
	d.runs = nil
	d.open = nil
	d.dirty = false
	return line
}

// printable drops the control characters that survived the scan.
//
// A tab is kept, because it is laid out rather than obeyed — see [TabStop] — and
// everything else in the C0 block is a movement instruction for a terminal this
// package is not. They are dropped here rather than at the cell so that measuring
// and drawing see the same text.
func printable(s string) string {
	if !strings.ContainsFunc(s, dropRune) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !dropRune(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// dropRune is [dropped] asked about one rune, which is the form a scan over raw
// output wants. The two must agree: text dropped here and text measured there are
// the same text.
func dropRune(r rune) bool { return r != '\t' && (r < 0x20 || r == 0x7f) }

// sgr applies one select-graphic-rendition sequence to the style in force.
//
// A parameter it does not recognise is skipped rather than treated as an error: the
// sequence is a list of independent requests, and one for something a cell cannot
// show — blinking, an overline, a colour under the underline — says nothing about
// the others.
func (d *Decoder) sgr(ps ansi.Params) {
	if ps.Empty() {
		// A bare "ESC [ m" is a reset, which is what an empty parameter defaulting to
		// zero already means.
		d.state = grid.Style{}
		return
	}
	for i := 0; i < ps.Count(); i++ {
		group := ps.Group(i)
		if len(group) == 0 {
			continue
		}
		switch code := group[0]; code {
		case 4:
			// The one attribute with a shape of its own: "4:0" is off and every other
			// subparameter is a style of underline a cell cannot tell apart.
			if len(group) > 1 && group[1] == 0 {
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

// colourAt reads an extended colour and reports how many further parameter groups it
// consumed.
//
// Both spellings are read. The colours may be subparameters of the 38 — which is
// what the standard says and what one terminal in three writes — or the parameters
// that follow it, which is what everything else writes. They are the same numbers
// in the same order either way, so they are read once and the only difference is
// where they were found.
func colourAt(ps ansi.Params, i int, group []int) (grid.Color, int, bool) {
	if len(group) > 1 {
		c, _, ok := colourOf(group[1:])
		return c, 0, ok
	}
	args := make([]int, 0, 4)
	for k := i + 1; k < ps.Count() && len(args) < cap(args); k++ {
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

// simple applies the parameters that are one number and nothing else.
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
