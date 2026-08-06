package kit

import (
	"github.com/Tangerg/oolong/core/grid"
)

// Message is one finished thing somebody said, drawn into the terminal's own
// scrollback.
//
// It is the other half of a streaming interface. What is still happening is the
// block at the bottom; what is finished belongs to the terminal, where it can be
// scrolled back to, selected, and read after the program has exited. A message is
// what goes there, and it is measured before it is drawn because printing takes a
// row count:
//
//	m := kit.Message{Theme: th, Speaker: "you", Body: line}
//	rows := m.Measure(width)
//	output.PrintRows(rows, m.Draw)
//
// Nothing here is retained. Once printed, the rows are the terminal's, which is why
// this is a value and not a widget with state.
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
}

// Measure is how many rows the message needs at this width.
func (m Message) Measure(width int) int {
	return m.head() + len(m.body(width).rows(m.wrapWidth(width))) + m.trailing()
}

// Draw paints the speaker, the body and the blank rows after it.
func (m Message) Draw(v grid.View) {
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
	body := m.body(width)
	_, height := v.Size()
	body.Draw(v.Sub(grid.Rect(gutter, y, m.wrapWidth(width), max(height-y, 0))))
}

// gutter is how far the body is indented under its speaker, so the eye can find
// where one message stops and the next starts without a rule between them.
const gutter = 2

func (m Message) head() int {
	if m.Speaker == "" {
		return 0
	}
	return 1
}

func (m Message) trailing() int {
	if m.Trailing > 0 {
		return m.Trailing
	}
	return 1
}

func (m Message) wrapWidth(width int) int { return max(width-gutter, 1) }

func (m Message) body(int) *Paragraph {
	return &Paragraph{Lines: linesOf(m.Body, m.Theme.Text)}
}
