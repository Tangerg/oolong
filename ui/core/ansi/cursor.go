package ansi

import (
	"strconv"
	"strings"
)

const (
	QueryCursorStyle = DCSIntroducer + "$q" + cursorStyleSelector + ST

	cursorStyleSelector  = string(cursorStyleIntermediate) + string(cursorStyleFinal)
	statusStringAccepted = "1$r"

	cursorStyleIntermediate = ' '
	cursorStyleFinal        = 'q'
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
	return CSI + strconv.Itoa(int(s)) + cursorStyleSelector
}

// CursorStyle reads p as DECSCUSR. False means p is another sequence or selects a
// style this package does not know.
func (p Piece) CursorStyle() (CursorStyle, bool) {
	if p.Kind != Control || p.Final != cursorStyleFinal || p.Intermediates != string(cursorStyleIntermediate) {
		return 0, false
	}
	if p.Parameters == "" {
		return CursorReset, true
	}
	return cursorStyleSelection(p.Parameters)
}

// cursorStyleSelection reads the one-digit selector that DECSCUSR and its DECRQSS
// answer both carry.
func cursorStyleSelection(value string) (CursorStyle, bool) {
	if len(value) != 1 || value[0] < '0' || value[0] > '0'+byte(CursorSteadyBar) {
		return 0, false
	}
	return CursorStyle(value[0] - '0'), true
}

// ParseCursorStyleReply requires a successful DECRQSS response for this setting
// to establish a restorable style. Other replies belong to their own consumers.
func ParseCursorStyleReply(body string) (CursorStyle, bool) {
	value, ok := strings.CutPrefix(body, statusStringAccepted)
	if !ok {
		return 0, false
	}
	value, ok = strings.CutSuffix(value, cursorStyleSelector)
	if !ok {
		return 0, false
	}
	return cursorStyleSelection(value)
}
