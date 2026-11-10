// Package markdown keeps document syntax independent of code and mathematics
// renderers. Immutable source snapshots give provisional and final documents the
// same semantics without importing peer parser dependencies.
package markdown

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"sync/atomic"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Doc owns a replaceable sequence of immutable blocks. A copy initially reads
// the same sequence; subsequent replacements and appends affect only the receiver.
// A projection finishes against the sequence captured when it started, even if
// an embedded drawable replaces the document during a layout callback.
type Doc struct{ view *Snapshot }

// Snapshot retains one immutable block sequence and a bounded single-width layout
// memo. A delayed preparation can populate only its own version, never a later
// Doc replacement. The zero value is empty; a Snapshot must not be copied after use.
// Concurrent projections may replace the width memo but cannot alter an already
// returned layout. Embedded drawables must also support concurrent reads when used
// this way, and retain the immutable-content contract of Block.
type Snapshot struct {
	blocks []Block
	layout atomic.Pointer[documentLayout]
	// One append may use the spare tail. Other copies deriving from this exact
	// sequence must detach; readers only access the immutable prefix.
	extended atomic.Bool
}

// SetBlocks copies the slice to prevent caller edits from replacing retained blocks.
func (d *Doc) SetBlocks(blocks []Block) {
	d.view = &Snapshot{blocks: slices.Clone(blocks)}
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
	d.view = &Snapshot{blocks: append(prefix, blocks...)}
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
	places []placedBlock
	// Measuring an embedded drawable must not allocate its complete text extent.
	// Explicit preparation warms this separate memo before a UI owner needs rows.
	rows          atomic.Pointer[rowProjection]
	width, height int
}

type rowProjection struct{ values []text.Row }

// HeightForWidth prepares missing geometry without materializing text rows.
// Combined row-space overflow panics before publishing the new geometry.
func (d *Snapshot) HeightForWidth(width int) int { return d.arrange(width).height }

// Draw uses the view's logical width; clipping does not change the layout width.
// Combined row-space overflow panics before publishing the new geometry.
func (d *Snapshot) Draw(v grid.View) {
	width, _ := v.Size()
	d.arrange(width).Draw(v)
}

// A prepared layout pins its complete Markdown subtree when the document caches
// another width. A smaller view clips those placements instead of reflowing.
func (d *documentLayout) HeightForWidth(int) int { return d.height }

func (d *documentLayout) Draw(v grid.View) {
	if v.Empty() {
		return
	}
	visible := v.Visible()
	first := sort.Search(len(d.places), func(index int) bool {
		placed := d.places[index]
		return placed.top+placed.layout.height > visible.Min.Y
	})
	for _, placed := range d.places[first:] {
		if placed.top >= visible.Max.Y {
			break
		}
		placed.layout.draw(v.Sub(grid.Area(0, placed.top, d.width, placed.layout.height)))
	}
}

// Rows returns an owned slice. Memoized text remains immutable after caller edits.
// Combined row-space overflow panics before publishing the new geometry.
func (d *Snapshot) Rows(width int) []text.Row {
	rows, _ := d.arrange(width).prepareRows(context.Background())
	return slices.Clone(rows.values)
}

func (d *Snapshot) arrange(width int) *documentLayout {
	if d == nil {
		return &documentLayout{}
	}
	if previous := d.layout.Load(); previous != nil && previous.width == width {
		return previous
	}
	result, _ := d.prepareLayout(context.Background(), width)
	return result
}

// PrepareWidth completes both wrapping and text projection before a UI owner
// accepts this result. Cancellation is checked between blocks and projections;
// an embedded callback must return before its cancellation can be observed.
// A nonpositive width prepares empty content. Concurrent preparation of another
// width may replace this memo, so readiness must still be read with PreparedLayout.
// Combined row-space overflow panics before publishing the new geometry.
func (d *Snapshot) PrepareWidth(ctx context.Context, width int) error {
	layout, err := d.prepareLayout(ctx, width)
	if err != nil {
		return err
	}
	_, err = layout.prepareRows(ctx)
	return err
}

// PreparedLayout never starts work. False means that geometry or text rows for
// this width are not ready. Its result retains the complete immutable layout tree
// after another width replaces a source snapshot's memo; embedded content remains
// subject to Block's passive-content contract.
func (d *Snapshot) PreparedLayout(width int) (grid.Drawable, bool) {
	if d == nil {
		return &documentLayout{}, true
	}
	result := d.layout.Load()
	if result == nil || result.width != width || result.rows.Load() == nil {
		return nil, false
	}
	return result, true
}

func (d *Snapshot) prepareLayout(ctx context.Context, width int) (*documentLayout, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	if d == nil {
		return &documentLayout{}, nil
	}
	if previous := d.layout.Load(); previous != nil && previous.width == width {
		return previous, nil
	}
	result := &documentLayout{width: width}
	for _, block := range d.blocks {
		if err := context.Cause(ctx); err != nil {
			return nil, err
		}
		placed, err := block.prepareLayout(ctx, width)
		if err != nil {
			return nil, err
		}
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
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	d.layout.Store(result)
	return result, nil
}

func (d *documentLayout) prepareRows(ctx context.Context) (*rowProjection, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	if previous := d.rows.Load(); previous != nil {
		return previous, nil
	}
	result := &rowProjection{values: make([]text.Row, d.height)}
	for _, placed := range d.places {
		if err := context.Cause(ctx); err != nil {
			return nil, err
		}
		rows, err := placed.layout.project(ctx)
		if err != nil {
			return nil, err
		}
		copy(result.values[placed.top:], rows)
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	if d.rows.CompareAndSwap(nil, result) {
		return result, nil
	}
	return d.rows.Load(), nil
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
	// redundant text when the words already carry a usable link. Address-like labels
	// must match the canonical web host or mail recipients. Differing or ambiguous
	// address labels carry no link; their destination remains visible.
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

func (e Extension) String() string {
	switch e {
	case FencedCode:
		return "fenced code"
	case DisplayMath:
		return "display math"
	default:
		return fmt.Sprintf("extension %d", uint8(e))
	}
}

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
	return fmt.Sprintf("markdown: block %d %v (%q): %q", e.Block, e.Extension, e.Info, e.Err)
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
		panic(fmt.Sprintf("markdown: invalid %v", extension))
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
