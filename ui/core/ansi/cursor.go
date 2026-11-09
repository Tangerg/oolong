package ansi

import (
	"strconv"
	"strings"
)

const (
	ShowCursor       = "\x1b[?25h"
	HideCursor       = "\x1b[?25l"
	QueryCursorStyle = "\x1bP$q" + cursorStyleSelector + "\x1b\\"

	cursorStyleSelector  = " q"
	statusStringAccepted = "1$r"
)

// CursorStyle is the DECSCUSR wire selector. Reset has terminal-specific
// semantics; it does not identify either the user's preference or the entry state.
type CursorStyle uint8

const (
	CursorReset CursorStyle = iota
	CursorBlinkingBlock
	CursorSteadyBlock
	CursorBlinkingUnderline
	CursorSteadyUnderline
	CursorBlinkingBar
	CursorSteadyBar
)

// Sequence panics for an unknown selector, so unchecked values cannot become
// terminal control parameters.
func (s CursorStyle) Sequence() string {
	if s > CursorSteadyBar {
		panic("ansi: invalid cursor style")
	}
	return "\x1b[" + strconv.Itoa(int(s)) + cursorStyleSelector
}

// ParseCursorStyleReply requires a successful DECRQSS response for this setting
// to establish a restorable style. Other replies belong to their own consumers.
func ParseCursorStyleReply(body string) (CursorStyle, bool) {
	value, ok := strings.CutPrefix(body, statusStringAccepted)
	if !ok {
		return 0, false
	}
	value, ok = strings.CutSuffix(value, cursorStyleSelector)
	if !ok || len(value) != 1 || value[0] < '0' || value[0] > '0'+byte(CursorSteadyBar) {
		return 0, false
	}
	return CursorStyle(value[0] - '0'), true
}
