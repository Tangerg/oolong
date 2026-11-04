// Package latex renders mathematics as immutable terminal [Formula] rows for drawing,
// selection and search. Untrusted unsupported input stays readable with an error.
// TeX document features, inclusion, macro definitions and command execution are outside
// its contract. Public values expose terminal text rather than parser objects.
package latex

import (
	"strings"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

// Delimiter keeps separate ends and a repeatable middle to group tall formulas.
type Delimiter struct {
	Single, Top, Middle, Bottom string
}

// Glyphs stays separate from Look because terminal repertoire and application colour
// vary independently. Zero fields use Unicode; Plain also spells named symbols
// in ASCII.
type Glyphs struct {
	Horizontal string
	Radical    string
	Ellipsis   string
	Left       Delimiter
	Right      Delimiter
	Plain      bool
}

func Unicode() Glyphs {
	return Glyphs{
		Horizontal: "─",
		Radical:    "√",
		Ellipsis:   "…",
		Left: Delimiter{
			Single: "(", Top: "⎛", Middle: "⎜", Bottom: "⎝",
		},
		Right: Delimiter{
			Single: ")", Top: "⎞", Middle: "⎟", Bottom: "⎠",
		},
	}
}

// ASCII output spells named symbols because replacing only the structural glyphs
// would still leave a formula dependent on Unicode support.
func ASCII() Glyphs {
	return Glyphs{
		Horizontal: "-",
		Radical:    "sqrt",
		Ellipsis:   "...",
		Left: Delimiter{
			Single: "(", Top: "/", Middle: "|", Bottom: "\\",
		},
		Right: Delimiter{
			Single: ")", Top: "\\", Middle: "|", Bottom: "/",
		},
		Plain: true,
	}
}

// GlyphsFor uses the transport-resolved locale because the transport owns the terminal environment.
// An unspecified locale keeps Unicode; an explicit non-UTF encoding selects ASCII.
func GlyphsFor(locale string) Glyphs {
	if text.PrefersUnicode(locale) {
		return Unicode()
	}
	return ASCII()
}

func (g Glyphs) normalized() Glyphs {
	base := Unicode()
	if g.Plain {
		base = ASCII()
	}
	if g.Horizontal == "" {
		g.Horizontal = base.Horizontal
	}
	if g.Radical == "" {
		g.Radical = base.Radical
	}
	if g.Ellipsis == "" {
		g.Ellipsis = base.Ellipsis
	}
	g.Left = g.Left.normalized(base.Left)
	g.Right = g.Right.normalized(base.Right)
	return g
}

func (d Delimiter) normalized(base Delimiter) Delimiter {
	if d.Single == "" {
		d.Single = base.Single
	}
	if d.Top == "" {
		d.Top = base.Top
	}
	if d.Middle == "" {
		d.Middle = base.Middle
	}
	if d.Bottom == "" {
		d.Bottom = base.Bottom
	}
	return d
}

// Look remains readable without a palette: its zero value uses terminal-default
// styles, Unicode structure and start alignment.
type Look struct {
	Text, Rule, Error grid.Style
	Glyphs            Glyphs
	Align             layout.Align
}

func (l Look) normalized() Look {
	l.Glyphs = l.Glyphs.normalized()
	return l
}

// Formula retains unsupported source with Err because an unfinished streamed
// expression may become valid when its next chunk arrives. Prepared rows are
// immutable so drawing and selection share the same mathematical layout.
type Formula struct {
	source string
	look   Look
	lines  []text.Line
	width  int
	err    error
}

// Render accepts an expression body rather than a TeX document. Bounded parser
// depth keeps untrusted nesting off the goroutine stack. Unsupported constructs
// remain visible through Formula.Err without writing process output.
func Render(source string, look Look) *Formula {
	look = look.normalized()
	f := &Formula{source: strings.Clone(source), look: look}
	if source == "" {
		return f
	}

	// Keep exact source for callers while parsing and fallback share physical line boundaries.
	source = strings.ReplaceAll(strings.ReplaceAll(source, "\r\n", "\n"), "\r", "\n")
	laidOut, err := render(source, look)
	if err == nil {
		f.lines, err = laidOut.lines()
	}
	if err != nil {
		f.err = err
		f.lines = sourceLines(source, look.Error)
	}
	for _, line := range f.lines {
		f.width = max(f.width, line.Width())
	}
	return f
}

func (f *Formula) Source() string {
	if f == nil {
		return ""
	}
	return f.source
}

func (f *Formula) Err() error {
	if f == nil {
		return nil
	}
	return f.err
}

// Lines detaches span storage so callers can compose rows without mutating the formula.
func (f *Formula) Lines() []text.Line {
	if f == nil {
		return nil
	}
	return text.CloneLines(f.lines)
}

func (f *Formula) Width() int {
	if f == nil {
		return 0
	}
	return f.width
}

// HeightForWidth preserves prepared rows because changing mathematical line breaks
// changes the expression; narrow views therefore clip instead of wrapping.
func (f *Formula) HeightForWidth(int) int {
	if f == nil {
		return 0
	}
	return len(f.lines)
}

// Draw consumes prepared rows so it cannot advance parsing or semantic state.
func (f *Formula) Draw(v grid.View) {
	if f == nil || v.Empty() {
		return
	}
	w, _ := v.Size()
	visible := v.Visible()
	first := min(max(visible.Min.Y, 0), len(f.lines))
	last := min(max(visible.Max.Y, first), len(f.lines))
	for y := first; y < last; y++ {
		line := f.shown(f.lines[y], w)
		line.Draw(v, f.look.Align.Offset(w, line.Width()), y)
	}
}

// Rows preserves the drawn two-dimensional form for selection. Formula.Source
// remains available separately when callers need the original TeX.
func (f *Formula) Rows(width int) []text.Row {
	if f == nil {
		return nil
	}
	width = max(width, 1)
	rows := make([]text.Row, len(f.lines))
	for i, line := range f.lines {
		shown := f.shown(line, width)
		rows[i] = text.Row{
			Text: shown.String(), Offset: f.look.Align.Offset(width, shown.Width()),
		}
	}
	return rows
}

func (f *Formula) shown(line text.Line, width int) text.Line {
	if line.Width() <= width {
		return line
	}
	return line.Truncate(max(width, 0), f.look.Glyphs.Ellipsis)
}

func sourceLines(source string, style grid.Style) []text.Line {
	parts := strings.Split(source, "\n")
	lines := make([]text.Line, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, text.Of(strings.Clone(text.Printable(part)), style))
	}
	return lines
}

var (
	_ grid.Drawable = (*Formula)(nil)
	_ error         = (*formulaError)(nil)
)

type formulaError struct{ message string }

func (e *formulaError) Error() string { return "latex: " + e.message }
