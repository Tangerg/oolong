// Package markdown isolates its parser and renders source snapshots into
// width-dependent [Block]s. Provisional and final snapshots share full document
// semantics. Code and mathematics use [Look.SetRenderer], keeping
// their parser dependencies and policies outside this module.
package markdown

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"sync/atomic"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Doc owns a replaceable sequence of immutable blocks. A copy initially reads
// the same sequence; subsequent replacements and appends affect only the receiver.
// A projection finishes against the sequence captured when it started, even if
// an embedded drawable replaces the document during a layout callback.
type Doc struct{ view *documentView }

// documentView binds the cache to the blocks it describes. A delayed layout may
// populate its own version; it cannot publish geometry for a replacement.
type documentView struct {
	blocks []Block
	layout atomic.Pointer[documentLayout]
	// One append may use the spare tail. Other copies deriving from this exact
	// sequence must detach; readers only access the immutable prefix.
	extended atomic.Bool
}

// SetBlocks replaces the document and owns the supplied slice.
func (d *Doc) SetBlocks(blocks []Block) {
	d.view = &documentView{blocks: slices.Clone(blocks)}
}

// Append adds immutable blocks while preserving the meaning of earlier copies.
func (d *Doc) Append(blocks ...Block) {
	if len(blocks) == 0 {
		return
	}
	var prefix []Block
	if previous := d.view; previous != nil {
		prefix = previous.blocks
		if !previous.extended.CompareAndSwap(false, true) {
			prefix = slices.Clip(prefix)
		}
	}
	d.view = &documentView{blocks: append(prefix, blocks...)}
}

// Len reports how many rendered blocks the document owns.
func (d *Doc) Len() int {
	if d == nil || d.view == nil {
		return 0
	}
	return len(d.view.blocks)
}

// Blocks returns an independent slice of the immutable blocks in document order.
func (d *Doc) Blocks() []Block {
	if d == nil || d.view == nil {
		return nil
	}
	return slices.Clone(d.view.blocks)
}

// HeightForWidth returns the composed height. It panics if the row space cannot fit in int.
func (d *Doc) HeightForWidth(width int) int { return d.view.HeightForWidth(width) }

// Draw paints the visible blocks from one layout. It panics for unrepresentable row space.
func (d *Doc) Draw(v grid.View) { d.view.Draw(v) }

// Rows projects meaningful text using the same placements as Draw. A child
// without Rows contributes blank rows. It panics for unrepresentable row space.
func (d *Doc) Rows(width int) []text.Row { return d.view.Rows(width) }

type placedBlock struct {
	top    int
	layout blockLayout
}

type documentLayout struct {
	places        []placedBlock
	width, height int
}

func (d *documentView) HeightForWidth(width int) int { return d.arrange(width).height }

func (d *documentView) Draw(v grid.View) {
	if v.Empty() {
		return
	}
	width, _ := v.Size()
	positions, visible := d.arrange(width), v.Visible()
	for _, placed := range positions.places {
		if placed.top >= visible.Max.Y || placed.top+placed.layout.height <= visible.Min.Y {
			continue
		}
		placed.layout.draw(v.Sub(grid.Area(0, placed.top, width, placed.layout.height)))
	}
}

func (d *documentView) Rows(width int) []text.Row {
	positions := d.arrange(width)
	rows := make([]text.Row, positions.height)
	for _, placed := range positions.places {
		copy(rows[placed.top:], placed.layout.project())
	}
	return rows
}

func (d *documentView) arrange(width int) *documentLayout {
	if d == nil {
		return &documentLayout{}
	}
	if previous := d.layout.Load(); previous != nil && previous.width == width {
		return previous
	}
	result := &documentLayout{width: width}
	for _, block := range d.blocks {
		placed := block.layout(width)
		if placed.height == 0 {
			continue
		}
		gap := 0
		if block.blankBefore && result.height > 0 {
			gap = 1
		}
		if placed.height > math.MaxInt-result.height-gap {
			panic("markdown: document row space exhausted")
		}
		top := result.height + gap
		result.places = append(result.places, placedBlock{top: top, layout: placed})
		result.height = top + placed.height
	}
	d.layout.Store(result)
	return result
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
	Text grid.Style
	// Headings are the levels, from one: the first entry is a level-one heading, and a
	// level deeper than the list gets the last entry.
	//
	// One field rather than a style for headings and a list that overrides it. Two
	// ways to say one thing is a thing to be inconsistent about, and "every heading
	// alike" is already sayable — it is a list of one.
	Headings []grid.Style

	Strong   grid.Style
	Emphasis grid.Style
	Struck   grid.Style
	Code     grid.Style
	Block    grid.Style
	// Link is the text of a link. The words carry the address themselves — see
	// [github.com/Tangerg/oolong/ui/core/text.Span.Link] — so a terminal that shows
	// hyperlinks opens what was written.
	Link grid.Style
	// Target is the address written out after the words, for output going somewhere
	// that cannot show a hyperlink. A look with no style for it does not write one,
	// which is the shorter reading and the one worth having on a terminal.
	Target grid.Style
	Quote  grid.Style
	Rail   grid.Style
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
// Render call or complete source preparation. Block is a zero-based index in that result.
type ExtensionError struct {
	Block     int
	Extension Extension
	Info      string
	Err       error
}

func (e *ExtensionError) Error() string {
	return fmt.Sprintf("markdown: block %d extension %d (%q): %q", e.Block, e.Extension, e.Info, e.Err)
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
	Bullet             string
	Bar                string
	Divider            string
	Checked, Unchecked string
}

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
