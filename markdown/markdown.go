// Package markdown turns markdown into terminal rows — including markdown that has
// not finished arriving.
//
// It is a module of its own, and the reason is a dependency. Rendering markdown
// needs a parser, and a parser is a tree of somebody else's code; the two modules
// this is built on promise a dependency list that a terminal library can be adopted
// for. So the parser lives here, behind a boundary, and nothing above or beside this
// module hears about it.
//
// # What it is for
//
// The commonest thing a streaming interface does is show an answer as it arrives.
// That is not what a markdown renderer normally does: every one of them takes a
// finished document and gives back a finished rendering, and a program showing a
// model's answer has neither. [Stream] is the difference — it is handed whatever has
// arrived, hands back the blocks that are certainly finished, and re-renders the one
// still being written on every keystroke of it.
//
// Finished blocks are finished for good, which is what makes this cheap: a paragraph
// that has been published is never parsed again, however long the answer becomes.
//
// # What it produces
//
// [Block]s of styled lines, not a string and not cells. A line has no width yet —
// see [github.com/Tangerg/oolong/core/text.Line] — so wrapping happens where the
// width is known, which is where it is drawn. [Doc] is the drawable form for a
// caller who wants one; a caller with its own idea of layout takes the blocks.
//
// # What it does not do
//
// It does not highlight code. A highlighter is several megabytes of lexers and a
// matter of taste, which is the same argument that keeps one appearance out of the
// behaviour a widget has — so [Look.Highlight] is where one plugs in, and a document
// with none draws code in one style. Chroma is ten lines away and is nobody's
// dependency until somebody wants it.
package markdown

import (
	"strings"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Block is one piece of a rendered document: lines that share an indent, and
// whatever is drawn in front of them.
//
// The two prefixes are two different things, which is why there are two. A marker
// goes on the first row only and is what a list item's bullet is; a rail goes on
// every row, including the rows a wrap produced, and is what a quotation's bar is.
// A renderer that had only one of them would either lose the bar down the side of a
// wrapped quotation or repeat the bullet down the side of a wrapped list item.
type Block struct {
	// Lines are what the block says. They have no width yet: wrapping happens where
	// the width is known.
	Lines []text.Line
	// Indent is which column the block's text starts at, which is how deep in a list
	// or a quotation it sits.
	Indent int
	// Marker is drawn on the first row, ending where the text starts: a bullet, a
	// number, a task's box.
	Marker text.Line
	// Rail is drawn on every row, ending where the text starts. It is what a
	// quotation's bar is, and it is on every row because a wrapped quotation with a
	// bar beside only its first line is a quotation that stops looking like one.
	Rail text.Line
	// Rule says the block is a line across the page: its one line is repeated to the
	// width it is drawn in rather than wrapped, because a rule is as wide as the room
	// it separates and only drawing knows what that is — and a rule wrapped instead of
	// stretched comes out as ten rows of dashes.
	Rule bool
	// Gap says a blank row belongs before this block. It is set between the blocks of
	// a document and not between the items of a list, which is the difference between
	// prose and a list that reads as one thing.
	Gap bool
}

// Doc is a rendered document, ready to be measured and drawn.
//
// It is a [github.com/Tangerg/oolong/core/grid.Drawer] and a
// [github.com/Tangerg/oolong/core/layout.Measurer] and nothing else, which is what
// lets it go into a slot, a container or a viewport belonging to a package this one
// has never heard of. Everything above the substrate speaks those two, so this
// module needs no widget of its own and imports none.
type Doc struct {
	// Blocks are what the document came to.
	Blocks []Block

	// rows memoises the wrap, which is asked for twice per frame — once to measure and
	// once to draw — and is the most expensive thing this does.
	rows    []row
	atWidth int
	fresh   bool
}

// row is one physical row: what it says, and where it starts.
type row struct {
	line   text.Line
	at     int
	prefix text.Line
}

// SetBlocks replaces the document.
func (d *Doc) SetBlocks(blocks []Block) {
	d.Blocks, d.fresh = blocks, false
}

// Append adds blocks to the end, which is what a stream does as they are finished.
func (d *Doc) Append(blocks ...Block) {
	d.Blocks = append(d.Blocks, blocks...)
	d.fresh = false
}

// Measure is how many rows the document needs at this width.
func (d *Doc) Measure(width int) int { return len(d.wrap(width)) }

// Draw writes the document, one wrapped row per row of v.
func (d *Doc) Draw(v grid.View) {
	width, height := v.Size()
	for y, r := range d.wrap(width) {
		if y >= height {
			return
		}
		if len(r.prefix) > 0 {
			r.prefix.Draw(v, r.at-r.prefix.Width(), y)
		}
		r.line.Draw(v, r.at, y)
	}
}

// wrap lays every block out at a width, once per width.
func (d *Doc) wrap(width int) []row {
	if d.fresh && d.atWidth == width {
		return d.rows
	}
	rows := d.rows[:0]
	for _, block := range d.Blocks {
		if block.Gap && len(rows) > 0 {
			rows = append(rows, row{})
		}
		at := block.Indent
		room := max(width-at, 1)
		if block.Rule {
			rows = append(rows, row{line: stretch(block.Lines, room), at: at, prefix: block.Rail})
			continue
		}
		first := true
		for _, line := range block.Lines {
			for _, wrapped := range line.Wrap(room) {
				prefix := block.Rail
				if first {
					// The marker replaces the rail on the row it is on, because they
					// occupy the same columns: a list inside a quotation is a bar, then a
					// bullet, and the bullet is the deeper of the two.
					if len(block.Marker) > 0 {
						prefix = block.Marker
					}
					first = false
				}
				rows = append(rows, row{line: wrapped.Line, at: at, prefix: prefix})
			}
		}
	}
	d.rows, d.atWidth, d.fresh = rows, width, true
	return rows
}

// stretch repeats a line until it fills the room there is, which is what a rule
// across the page is: one character, as many times as the width says.
func stretch(lines []text.Line, room int) text.Line {
	if len(lines) == 0 || len(lines[0]) == 0 {
		return nil
	}
	span := lines[0][0]
	width := text.Width(span.Text)
	if width <= 0 {
		return nil
	}
	span.Text = strings.Repeat(span.Text, room/width+1)
	return text.Line{span}.Truncate(room, "")
}

// Look is how a document is drawn: a style for every part of one, and the characters
// its furniture is made of.
//
// The zero Look draws everything in the terminal's own appearance, which is legible
// and says nothing. A caller with a palette builds one from it — which is deliberately
// not done here, because this module cannot see the palette without depending on the
// package that has one, and that is the dependency this whole boundary exists to
// avoid.
type Look struct {
	// Text is body text, and Heading is a heading of any level. Levels are told apart
	// by [Look.Headings] where a caller wants them to be.
	Text    grid.Style
	Heading grid.Style
	// Headings, when it has an entry for a level, overrides Heading for that level —
	// index zero is a level-one heading. A caller that wants every heading to look the
	// same leaves it nil.
	Headings []grid.Style

	// Strong and Emphasis are bold and italic, or whatever a look prefers them to be.
	Strong   grid.Style
	Emphasis grid.Style
	// Struck is text somebody crossed out.
	Struck grid.Style
	// Code is a span of code in a sentence, and Block is a block of it.
	Code  grid.Style
	Block grid.Style
	// Link is the text of a link, and Target the address after it.
	Link   grid.Style
	Target grid.Style
	// Quote is quoted text and Rail the bar beside it.
	Quote grid.Style
	Rail  grid.Style
	// Marker is a bullet or a number, and Rule a thematic break.
	Marker grid.Style
	Rule   grid.Style

	// Bullet is what an unordered list item is marked with, Rail what a quotation is
	// barred with, and Divider what a thematic break is drawn with. A look that leaves
	// them empty gets no furniture at all, which is the right answer for a terminal
	// that cannot draw the characters and a poor one everywhere else — see the glyph
	// set in the kit package for how a program decides which it is.
	Bullet  string
	Bar     string
	Divider string
	// Checked and Unchecked mark a task list's items.
	Checked, Unchecked string

	// Highlight turns a block of code into styled lines, and is where a syntax
	// highlighter plugs in. Nil draws the code in [Look.Block], which is what a
	// document that nobody has chosen a highlighter for should look like.
	//
	// The language is whatever was written after the fence, unexamined: it is the
	// author's word for what the code is, and mapping it onto a lexer is the
	// highlighter's business rather than this module's.
	Highlight func(language, source string) []text.Line
}

// heading is the style for a heading of a level, counting from one.
func (l Look) heading(level int) grid.Style {
	if level >= 1 && level <= len(l.Headings) {
		return l.Headings[level-1]
	}
	return l.Heading
}

// A Doc is a Measurer, which is what lets it go in a slot without an adapter. The
// assertion is here so that a change to either side is a build failure rather than a
// surprise at a call site in somebody else's program.
var (
	_ layout.Measurer = (*Doc)(nil)
	_ grid.Drawer     = (*Doc)(nil)
)
