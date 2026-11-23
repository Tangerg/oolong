package grid

import (
	"errors"
	"fmt"
	"image"

	"github.com/Tangerg/oolong/ui/core/ansi"
	"github.com/Tangerg/oolong/ui/internal/known"
)

type CursorShape uint8

const (
	// CursorReset requests the terminal-specific DECSCUSR reset. Blink is ignored.
	// This is a blinking block on xterm, not the user's configured or entry shape.
	CursorReset CursorShape = iota
	CursorBlock
	CursorUnderline
	CursorBar
)

var ErrInvalidCursorShape = errors.New("grid: invalid cursor shape")

func (s CursorShape) Validate() error {
	if s > CursorBar {
		return fmt.Errorf("%w: %d", ErrInvalidCursorShape, s)
	}
	return nil
}

// CursorStyle is how the terminal's own cursor is drawn. The zero value uses the
// protocol reset. Blink applies to Block, Underline and Bar.
type CursorStyle struct {
	Shape CursorShape
	Blink bool
}

// Sequence panics for an invalid Shape rather than encoding it as a protocol
// reset with unrelated terminal-specific meaning.
func (s CursorStyle) Sequence() string { return s.selector().Sequence() }

// selector is the one mapping from shape and blink to the DECSCUSR wire value.
func (s CursorStyle) selector() ansi.CursorStyle {
	blinking := func(blinking, steady ansi.CursorStyle) ansi.CursorStyle {
		if s.Blink {
			return blinking
		}
		return steady
	}
	switch s.Shape {
	case CursorReset:
		return ansi.CursorReset
	case CursorBlock:
		return blinking(ansi.CursorBlinkingBlock, ansi.CursorSteadyBlock)
	case CursorUnderline:
		return blinking(ansi.CursorBlinkingUnderline, ansi.CursorSteadyUnderline)
	case CursorBar:
		return blinking(ansi.CursorBlinkingBar, ansi.CursorSteadyBar)
	}
	panic(ErrInvalidCursorShape)
}

func (s CursorStyle) normalized() CursorStyle {
	if err := s.Shape.Validate(); err != nil {
		panic(err)
	}
	if s.Shape == CursorReset {
		s.Blink = false
	}
	return s
}

type Cursor struct {
	Visible bool
	Pos     image.Point
	Style   CursorStyle
}

// cursorBelief is what a canvas last told the terminal about its cursor. Every
// redundant command can restart blinking, so a known, unchanged property is not
// sent again. Screen and Inline position the cursor differently but share this.
type cursorBelief struct {
	visible known.Value[bool]
	style   known.Value[CursorStyle]
}

func (b *cursorBelief) forget() { *b = cursorBelief{} }

func (b *cursorBelief) showing() bool { return known.Is(b.visible, true) }

// differs reports whether next needs any visibility or style command.
func (b *cursorBelief) differs(next Cursor) bool {
	return !known.Is(b.visible, next.Visible) || next.Visible && !known.Is(b.style, next.Style)
}

func (b *cursorBelief) appendStyle(out []byte, style CursorStyle) []byte {
	if known.Is(b.style, style) {
		return out
	}
	b.style = known.Of(style)
	return append(out, style.Sequence()...)
}

// appendVisibility follows positioning when showing, so the cursor never
// appears at a stale position.
func (b *cursorBelief) appendVisibility(out []byte, visible bool) []byte {
	if known.Is(b.visible, visible) {
		return out
	}
	b.visible = known.Of(visible)
	if visible {
		return append(out, showCursor...)
	}
	return append(out, hideCursor...)
}
