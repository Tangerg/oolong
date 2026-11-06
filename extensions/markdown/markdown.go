// Package markdown keeps document syntax independent of code and mathematics
// renderers. Immutable source snapshots give provisional and final documents the
// same semantics without importing peer parser dependencies.
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

// SetBlocks copies the slice to prevent caller edits from replacing retained blocks.
func (d *Doc) SetBlocks(blocks []Block) {
	d.view = &documentView{blocks: slices.Clone(blocks)}
}

// Append lets copies advance independently; only one descendant can claim shared spare
// capacity without changing another copy's sequence.
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

func (d *Doc) Len() int {
	if d == nil || d.view == nil {
		return 0
	}
	return len(d.view.blocks)
}

// Blocks detaches its slice so callers can reorder their copy without changing the document.
func (d *Doc) Blocks() []Block {
	if d == nil || d.view == nil {
		return nil
	}
	return slices.Clone(d.view.blocks)
}

// HeightForWidth panics on unrepresentable row space to prevent composed geometry overflow.
func (d *Doc) HeightForWidth(width int) int { return d.view.HeightForWidth(width) }

// Draw panics on unrepresentable row space to prevent addressing unrelated rows.
func (d *Doc) Draw(v grid.View) { d.view.Draw(v) }

// Rows preserves blank placements for children without text rows so selection
// matches drawing. Unrepresentable row space panics.
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

// Look uses terminal-default appearance at its zero value so this parser module need not
// depend on the component package that owns application palettes.
type Look struct {
	Text grid.Style
	// One sequence owns heading appearance; deeper levels reuse its final style.
	// A single entry already expresses one style for every heading.
	Headings []grid.Style

	Strong   grid.Style
	Emphasis grid.Style
	Struck   grid.Style
	Code     grid.Style
	Block    grid.Style
	// Styled words retain their address so wrapping cannot change the hyperlink.
	// A document is untrusted, so only http, https and mailto destinations become
	// hyperlinks; any other keeps this style without one.
	Link grid.Style
	// An explicit address helps outputs without hyperlinks. A zero style omits that
	// redundant text when the words already carry a usable link. Words that show a
	// different address carry no link and are always followed by the destination.
	Target grid.Style
	Quote  grid.Style
	Rail   grid.Style
	Marker grid.Style
	Rule   grid.Style

	// Terminal repertoire varies independently of colour, so the application passes
	// glyphs separately from its palette.
	Glyphs Glyphs

	// Peer renderers receive core values instead of parser nodes so their dependencies
	// and policies remain independent of Markdown. Missing renderers leave source readable.
	extensions map[Extension]Renderer
}

// Extension permits replacement of semantic block rendering while Markdown
// retains ownership of recognition and parse correctness.
type Extension uint8

const (
	_ Extension = iota
	FencedCode
	DisplayMath
)

// Renderer results may outlive preparation, so a returned drawable must retain
// stable semantics until every block releases it. Preparation is synchronous and
// never runs during drawing or measurement; expensive backends supply prepared
// snapshots. Content and error may coexist. ErrUnhandled requests source text;
// nil content without an error violates the contract.
type Renderer func(info, source string) (grid.Drawable, error)

// ErrUnhandled distinguishes a deliberate decline from backend failure so source
// fallback does not conceal diagnostics.
var ErrUnhandled = errors.New("markdown: extension not handled")

// ExtensionError keeps a zero-based Block index in one Render result so a backend
// failure stays associated with the readable content returned alongside it. A nested
// failure identifies the containing list-item Block, which owns those children.
type ExtensionError struct {
	Block     int
	Extension Extension
	Info      string
	Err       error
}

func (e *ExtensionError) Error() string {
	return fmt.Sprintf("markdown: block %d extension %d (%q): %q", e.Block, e.Extension, e.Info, e.Err)
}

func (e *ExtensionError) Unwrap() error { return e.Err }

// SetRenderer copies the registry before mutation to keep copied Looks independent.
// A nil renderer removes that extension; the zero Look is ready for registration.
// It panics before mutation unless extension is FencedCode or DisplayMath, because
// only Markdown's parser can define a semantic hook that rendering will invoke.
func (l *Look) SetRenderer(extension Extension, renderer Renderer) {
	switch extension {
	case FencedCode, DisplayMath:
	default:
		panic(fmt.Sprintf("markdown: invalid extension %d", extension))
	}
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

// Glyphs keeps structural spacing at its zero value without assuming the terminal can
// draw decorative characters. The caller owns that capability decision.
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

var _ grid.Drawable = (*Doc)(nil)
