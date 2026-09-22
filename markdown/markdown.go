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
// [Block]s, not a string and not cells. A block owns the styled source and the
// layout rule that gives it physical rows at a width. Keeping those together is
// what lets prose wrap, rules stretch and tables reflow without turning any of
// them into cells before the final region is known. [Doc] composes the blocks; a
// caller with its own layout can measure and draw them directly.
//
// # What it does not do
//
// It does not highlight code or typeset mathematics. Those concerns bring their own
// parsers, dependencies and policies. [Look.SetRenderer] is the one seam where
// recognized semantic blocks receive such a renderer; without one their source stays
// readable. The Markdown parser and its AST never cross that seam.
package markdown

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

// Doc is a rendered document, ready to be measured and drawn.
//
// It draws into a grid view and is a
// [grid.Drawable], which is what lets it go into a
// slot, container or viewport belonging to a package this one has never heard of.
// Copies detach block and layout storage before either can be changed. Embedded
// content remains shared and must obey Renderer's stable-result contract.
type Doc struct {
	// blocks are private because every mutation must invalidate placements. Exposing this
	// slice made it possible to change the document while its cached wrap still
	// described the old one.
	blocks []Block
	// blocksOwner identifies the value allowed to append into spare block capacity.
	// A copied Doc detaches before appending, so two values cannot overwrite each
	// other's logical document through a shared backing array.
	blocksOwner *Doc

	// places is an immutable layout snapshot shared by measurement and drawing.
	places  []placedBlock
	height  int
	atWidth int
	fresh   bool
}

// SetBlocks replaces the document. Blocks are immutable values; Doc copies the
// slice so the caller may reuse it after this returns.
func (d *Doc) SetBlocks(blocks []Block) {
	d.blocks = slices.Clone(blocks)
	d.blocksOwner = d
	d.invalidate()
}

// Append adds blocks to the end, which is what a stream does as they are finished.
func (d *Doc) Append(blocks ...Block) {
	if len(blocks) == 0 {
		return
	}
	d.ownBlocks()
	d.blocks = append(d.blocks, blocks...)
	d.invalidate()
}

// ownBlocks detaches a copied document before it grows the shared slice.
func (d *Doc) ownBlocks() {
	if d.blocksOwner == d {
		return
	}
	d.blocks = slices.Clone(d.blocks)
	d.blocksOwner = d
}

// invalidate releases the immutable presentation snapshot.
func (d *Doc) invalidate() {
	d.places = nil
	d.height = 0
	d.fresh = false
}

// Len reports how many rendered blocks the document owns.
func (d *Doc) Len() int {
	if d == nil {
		return 0
	}
	return len(d.blocks)
}

// Blocks returns the immutable rendered blocks in document order. The returned
// slice is independent; the blocks themselves are values safe to share.
func (d *Doc) Blocks() []Block {
	if d == nil {
		return nil
	}
	return slices.Clone(d.blocks)
}

// HeightForWidth reports the composed physical height at width.
func (d *Doc) HeightForWidth(width int) int { d.arrange(width); return d.height }

// Draw delegates to each block using the same geometry as measurement and Rows.
func (d *Doc) Draw(v grid.View) {
	if v.Empty() {
		return
	}
	width, _ := v.Size()
	d.arrange(width)
	visible := v.Visible()
	for _, placed := range d.places {
		if placed.top >= visible.Max.Y || placed.top+placed.layout.height <= visible.Min.Y {
			continue
		}
		placed.layout.draw(v.Sub(grid.Rect(0, placed.top, width, placed.layout.height)))
	}
}

// Rows projects visible text using the same block placements as Draw. A child
// without Rows contributes blank physical rows, preserving selection coordinates.
func (d *Doc) Rows(width int) []text.Row {
	d.arrange(width)
	rows := make([]text.Row, d.height)
	for _, placed := range d.places {
		copy(rows[placed.top:], placed.layout.project())
	}
	return rows
}

type placedBlock struct {
	top    int
	layout blockLayout
}

func (d *Doc) arrange(width int) {
	if d.fresh && d.atWidth == width {
		return
	}
	var places []placedBlock
	height := 0
	for _, block := range d.blocks {
		placed := block.layout(width)
		if placed.height == 0 {
			continue
		}
		if block.blankBefore && height > 0 {
			height++
		}
		places = append(places, placedBlock{top: height, layout: placed})
		height += placed.height
	}
	d.places, d.height, d.atWidth, d.fresh = places, height, width, true
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
	// Text is body text.
	Text grid.Style
	// Headings are the levels, from one: the first entry is a level-one heading, and a
	// level deeper than the list gets the last entry.
	//
	// One field rather than a style for headings and a list that overrides it. Two
	// ways to say one thing is a thing to be inconsistent about, and "every heading
	// alike" is already sayable — it is a list of one.
	Headings []grid.Style

	// Strong and Emphasis are bold and italic, or whatever a look prefers them to be.
	Strong   grid.Style
	Emphasis grid.Style
	// Struck is text somebody crossed out.
	Struck grid.Style
	// Code is a span of code in a sentence, and Block is a block of it.
	Code  grid.Style
	Block grid.Style
	// Link is the text of a link. The words carry the address themselves — see
	// [github.com/Tangerg/oolong/core/text.Span.Link] — so a terminal that shows
	// hyperlinks opens what was written.
	Link grid.Style
	// Target is the address written out after the words, for output going somewhere
	// that cannot show a hyperlink. A look with no style for it does not write one,
	// which is the shorter reading and the one worth having on a terminal.
	Target grid.Style
	// Quote is quoted text and Rail the bar beside it.
	Quote grid.Style
	Rail  grid.Style
	// Marker is a bullet or a number, and Rule a thematic break.
	Marker grid.Style
	Rule   grid.Style

	// Glyphs are the characters the furniture is drawn with, kept apart from the
	// styles for the reason the two are different questions: which grey a quotation
	// is drawn in is taste, and whether the terminal can draw the bar beside it is a
	// fact about the terminal. It is the same division the kit package makes between
	// a theme and a glyph set, and a caller with one of those builds this from it.
	Glyphs Glyphs

	// Extension renderers handle recognized semantic blocks whose implementation belongs in a
	// sibling module. Code highlighting and mathematical typesetting use this same
	// boundary. Missing renderers leave source readable in Block style.
	//
	// The functions consume and produce core values. They never receive parser
	// nodes, so the Markdown implementation and its dependency remain private.
	extensions map[Extension]Renderer
}

// Extension identifies a semantic block recognized by Markdown. It is not a parser
// plug-in API: syntax and parse correctness remain this module's responsibility;
// rendering the resulting domain content is the replaceable part.
type Extension uint8

const (
	_ Extension = iota
	// FencedCode is a backtick or tilde fence. The renderer's info is the language
	// written after the fence.
	FencedCode
	// DisplayMath is a $$ block or a fenced block whose language is math.
	DisplayMath
)

// Renderer prepares one semantic block. The result owns layout and may optionally
// implement Rows(width int) []text.Row. Its semantics must remain stable for the
// lifetime of every published Block: replace content rather than mutating it.
// Calls are synchronous preparation, never Draw/HeightForWidth/Rows callbacks.
// Expensive backends must be prepared separately and return accepted snapshots.
// A result and error may coexist. ErrUnhandled explicitly requests source text;
// nil content without an error violates this contract.
type Renderer func(info, source string) (grid.Drawable, error)

// ErrUnhandled explicitly declines an extension body; Markdown displays its source.
var ErrUnhandled = errors.New("markdown: extension not handled")

// ExtensionError locates an extension failure in the blocks returned by one
// Render, Feed, Open or Flush call. Block is a zero-based index in that result.
type ExtensionError struct {
	Block     int
	Extension Extension
	Info      string
	Err       error
}

func (e *ExtensionError) Error() string {
	return fmt.Sprintf("markdown: block %d extension %d (%s): %v", e.Block, e.Extension, e.Info, e.Err)
}

// Unwrap preserves the backend's diagnostic identity.
func (e *ExtensionError) Unwrap() error { return e.Err }

// SetRenderer sets the sole renderer for extension. A nil renderer removes it.
//
// The zero Look is ready. The private registry is copied before mutation, so changing
// a copied Look does not silently change the value it was copied from.
func (l *Look) SetRenderer(extension Extension, renderer Renderer) {
	l.extensions = maps.Clone(l.extensions)
	if renderer == nil {
		delete(l.extensions, extension)
		if len(l.extensions) == 0 {
			l.extensions = nil
		}
		return
	}
	if l.extensions == nil {
		l.extensions = make(map[Extension]Renderer)
	}
	l.extensions[extension] = renderer
}

func (l *Look) renderer(extension Extension) Renderer { return l.extensions[extension] }

// Glyphs are the characters a document's furniture is drawn with.
//
// The zero value draws none of it, which is legible and plain: a list is still
// indented, a quotation is still inset, and a rule is still a row of its own. That
// is the right answer for a terminal that cannot draw the characters, and the
// caller is the one who knows whether it can.
type Glyphs struct {
	// Bullet marks an item of an unordered list.
	Bullet string
	// Bar is what a quotation is barred with, on every row of it.
	Bar string
	// Divider is what a thematic break and a table's heading rule are drawn with.
	Divider string
	// Checked and Unchecked mark a task list's items.
	Checked, Unchecked string
}

// heading is the style for a heading of a level, counting from one. A level deeper
// than the look has entries for is drawn as the deepest one it has.
func (l *Look) heading(level int) grid.Style {
	if len(l.Headings) == 0 {
		return l.Text
	}
	return l.Headings[min(max(level, 1), len(l.Headings))-1]
}

// A Doc is a Measurer, which is what lets it go in a slot without an adapter. The
// assertion is here so that a change to either side is a build failure rather than a
// surprise at a call site in somebody else's program.
var _ grid.Drawable = (*Doc)(nil)
