package kit

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

// Message is one finished thing somebody said, ready for a transcript or the
// terminal's own scrollback.
//
// It is the stable half of a streaming interface. A recent message may remain in a
// selectable transcript while interaction is worth its cost; an older one may be
// committed to the terminal, where it survives the program. The same passive Block
// works in both places and is measured before it is drawn because publication needs
// a row count:
//
//	m := &kit.Message{Theme: th, Speaker: "you", Body: line}
//	output.Print(m)
//
// It owns no interaction lifecycle or routing state. Its transcript or printer
// decides how long it remains live. Message is a pointer only because wrapping is
// memoised between Measure and Draw; that private cache is presentation state.
type Message struct {
	Theme Theme
	// Speaker is who said it. Empty draws no label and no gutter.
	Speaker string
	// Body is what they said. Newlines are line breaks; everything else wraps.
	Body string
	// Trailing is how many blank rows follow, so consecutive messages do not run
	// together. Zero uses one, which is what separates without doubling the spacing.
	Trailing int
	// Own marks the message as the user's own rather than an answer, which draws the
	// speaker in the accent colour instead of the muted one.
	Own bool

	paragraph  Paragraph
	cachedBody string
	cachedText grid.Style
	fresh      bool
}

var (
	_ headless.Block    = (*Message)(nil)
	_ headless.Copyable = (*Message)(nil)
)

// Measure is how many rows the message needs at this width.
func (m *Message) Measure(width int) int {
	if m == nil {
		return 0
	}
	return layout.Sum(m.head(), len(m.body().rows(m.wrapWidth(width))), m.trailing())
}

// Draw paints the speaker, the body and the blank rows after it.
func (m *Message) Draw(v grid.View) {
	if m == nil || v.Empty() {
		return
	}
	width, _ := v.Size()
	if width <= 0 {
		return
	}
	y := 0
	if m.head() > 0 {
		style := m.Theme.Muted
		if m.Own {
			style = m.Theme.Accent
		}
		v.Text(0, 0, m.Speaker, style)
		y = 1
	}
	body := m.body()
	_, height := v.Size()
	indent := m.indent()
	body.Draw(v.Sub(grid.Rect(indent, y, m.wrapWidth(width), layout.Remaining(height, y))))
}

// Rows returns the message without its visual gutter, with body offsets aligned to
// where Draw places them. Speaker and trailing rows remain real text rows because a
// selection dragged across the message includes the same vertical structure it saw.
func (m *Message) Rows(width int) []text.Row {
	if m == nil {
		return nil
	}
	out := make([]text.Row, 0, m.Measure(width))
	if m.head() > 0 {
		out = append(out, text.Row{Text: m.Speaker})
	}
	for _, row := range m.body().Rows(m.wrapWidth(width)) {
		row.Offset = layout.Sum(row.Offset, m.indent())
		out = append(out, row)
	}
	for range m.trailing() {
		out = append(out, text.Row{})
	}
	return out
}

// gutter is how far the body is indented under its speaker, so the eye can find
// where one message stops and the next starts without a rule between them.
const gutter = 2

func (m *Message) head() int {
	if m.Speaker == "" {
		return 0
	}
	return 1
}

func (m *Message) trailing() int {
	if m.Trailing > 0 {
		return m.Trailing
	}
	return 1
}

func (m *Message) indent() int {
	if m.Speaker == "" {
		return 0
	}
	return gutter
}

func (m *Message) wrapWidth(width int) int { return max(layout.Remaining(width, m.indent()), 1) }

func (m *Message) body() *Paragraph {
	if !m.fresh || m.cachedBody != m.Body || m.cachedText != m.Theme.Text {
		m.paragraph.SetText(linesOf(m.Body, m.Theme.Text))
		m.cachedBody = m.Body
		m.cachedText = m.Theme.Text
		m.fresh = true
	}
	return &m.paragraph
}
