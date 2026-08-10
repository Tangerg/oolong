// Package latex lays LaTeX mathematics out as terminal rows.
//
// It is deliberately a mathematics renderer, not a TeX engine. Documents,
// packages, file inclusion, macro definitions, page layout and command execution
// are outside its contract. A formula is untrusted content: unsupported or
// incomplete input becomes readable source with an error attached rather than a
// panic or an empty region.
//
// # One output model
//
// [Render] returns a concrete, immutable [Formula]. The same value measures and
// draws as a [github.com/Tangerg/oolong/core/grid.Drawable], and exposes the rows it
// draws for transcript selection and search. There is no image-only rendering path:
// terminal text remains visible on every host and remains meaningful when copied.
//
// # Integration
//
// [Of] adapts the same renderer to the two-string function shape used by semantic
// content registries. The first argument is reserved for format information and the
// second is the expression source:
//
//	render := latex.Of(latex.Look{Text: style, Glyphs: latex.Unicode()})
//	lines := render("", source)
//
// [Of] returns lines and does not expose the [Formula] value. Call [Render] inside
// a custom adapter to observe [Formula.Err], [Formula.Source] or [Formula.Width]
// before returning [Formula.Lines]:
//
//	render := func(_ string, source string) []text.Line {
//		formula := latex.Render(source, look)
//		if err := formula.Err(); err != nil {
//			report(source, err)
//		}
//		return formula.Lines()
//	}
//
// The producer and consumer remain peers. Both know only core text and grid values;
// neither imports the other.
package latex

import (
	"errors"
	"strings"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Delimiter is one side of a grouped expression. Single is used for a one-row
// group; Top, Middle and Bottom stretch it beside taller content.
type Delimiter struct {
	Single, Top, Middle, Bottom string
}

// Glyphs are the characters used to construct a formula's two-dimensional
// furniture. They are separate from [Look] because character repertoire is a fact
// about the driven terminal while colour is application appearance.
//
// The zero value is completed from [Unicode]. Set Plain when custom glyphs must also
// spell named symbols such as alpha and leq in ASCII.
type Glyphs struct {
	Horizontal string
	Radical    string
	Ellipsis   string
	Left       Delimiter
	Right      Delimiter
	Plain      bool
}

// Unicode is the formula set for a UTF-8 terminal.
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

// ASCII is the formula set for output that cannot safely carry Unicode. Named
// symbols are written as names; structural lines and delimiters use ASCII.
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

// GlyphsFor picks the set a terminal in locale can draw. An empty locale keeps the
// modern Unicode default; a locale that explicitly names another encoding uses the
// ASCII set.
//
// The resolved locale is an argument because choosing which environment belongs to
// a terminal is a transport concern. Formula appearance only interprets the fact it
// was given.
func GlyphsFor(locale string) Glyphs {
	if locale == "" || text.UTF8Locale(locale) {
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

// Look is a formula's appearance. Text covers symbols and operands, Rule covers
// fractions and overlines, and Error covers source shown when the expression is not
// supported. Align places a formula narrower than its region.
//
// The zero value uses terminal-default styles, Unicode furniture and start
// alignment.
type Look struct {
	Text, Rule, Error grid.Style
	Glyphs            Glyphs
	Align             layout.Align
}

func (l Look) normalized() Look {
	l.Glyphs = l.Glyphs.normalized()
	return l
}

// Formula is one immutable rendered expression.
//
// Unsupported input is still a Formula: [Formula.Err] reports why it could not be
// typeset and its drawing is the original source in Look.Error. This is the useful
// failure mode for streamed model output, where an unfinished command must remain
// visible and a later chunk may make it valid.
type Formula struct {
	source string
	look   Look
	lines  []text.Line
	width  int
	err    error
}

// Render lays out source. Source is the body of one math expression; surrounding
// $, $$, \( and \[ delimiters are not part of it. To keep untrusted input off the
// goroutine stack, brace-group and consecutive-script nesting beyond 256 levels is
// reported through [Formula.Err] and shown as source.
func Render(source string, look Look) *Formula {
	look = look.normalized()
	f := &Formula{source: strings.Clone(source), look: look}
	if source == "" {
		return f
	}

	laidOut, err := render(source, look)
	if err != nil {
		f.err = err
		f.lines = sourceLines(f.source, look.Error)
	} else {
		f.lines = laidOut.lines()
	}
	for _, line := range f.lines {
		f.width = max(f.width, line.Width())
	}
	return f
}

// Of returns the semantic-renderer function shape. Every call goes through [Render];
// this is an adapter to a consumer-owned function type, not a second rendering API.
//
// Use Of when styled lines are the complete result. The function intentionally omits
// the resulting [Formula] and its error, source, width and drawing methods. Call
// [Render] in a custom adapter when those values matter, then return [Formula.Lines].
func Of(look Look) func(info, source string) []text.Line {
	look = look.normalized()
	return func(_ string, source string) []text.Line {
		return Render(source, look).Lines()
	}
}

// Source returns the expression body supplied to [Render].
func (f *Formula) Source() string {
	if f == nil {
		return ""
	}
	return f.source
}

// Err reports why the source was shown rather than typeset. It is nil for an empty
// or successfully rendered formula.
func (f *Formula) Err() error {
	if f == nil {
		return nil
	}
	return f.err
}

// Lines returns a deep copy of the laid-out terminal lines. It is the styled-text
// boundary used by Markdown and custom passive-content composers.
func (f *Formula) Lines() []text.Line {
	if f == nil {
		return nil
	}
	return text.CloneLines(f.lines)
}

// Width reports the natural width of the formula in terminal columns.
func (f *Formula) Width() int {
	if f == nil {
		return 0
	}
	return f.width
}

// Measure reports the formula's fixed row count. A formula does not wrap: changing
// mathematical line breaks changes the expression, so a narrow view clips it.
func (f *Formula) Measure(int) int {
	if f == nil {
		return 0
	}
	return len(f.lines)
}

// Draw writes the formula, clipping wide rows and preserving its two-dimensional
// layout. Drawing never reparses or changes semantic state.
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

// Rows returns the meaningful text and rendered offset of each physical formula
// row at width. Selection therefore copies the same readable two-dimensional form
// that was drawn; [Formula.Source] remains available when an application wants the
// original LaTeX instead.
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
		lines = append(lines, text.Of(strings.Clone(part), style))
	}
	return lines
}

var (
	_ grid.Drawable = (*Formula)(nil)
	_ error         = (*parseError)(nil)
)

// parseError keeps the external parser and its panic conventions out of the public
// API while preserving a useful explanation through Formula.Err.
type parseError struct{ message string }

func (e *parseError) Error() string { return "latex: " + e.message }

func parseFailure(v any) error {
	var err error
	switch value := v.(type) {
	case error:
		err = value
	case string:
		err = errors.New(value)
	default:
		err = errors.New("parser failed")
	}
	return &parseError{message: err.Error()}
}
