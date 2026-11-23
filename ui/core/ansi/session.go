package ansi

import "strconv"

// Session requests ask the terminal about itself or keep state it restores later.
// They draw nothing, so an output model recognizes them by these exact values.
const (
	// QueryPrimaryAttributes is DA1. Every terminal answers it, so a probe sends it
	// last and its reply settles the questions asked before it.
	QueryPrimaryAttributes = CSI + string(DA)
	// QuerySecondaryAttributes is DA2, a numeric identity for terminals that do not
	// report a name.
	QuerySecondaryAttributes = CSI + ">0" + string(DA)
	// QueryTerminalVersion is XTVERSION, the terminal's own name and version.
	QueryTerminalVersion = CSI + ">0q"
	// QueryKeyboard asks which kitty keyboard enhancements are in effect.
	QueryKeyboard = CSI + "?u"
	// PopKeyboard restores the enhancements in effect before the last PushKeyboard.
	PopKeyboard = CSI + "<u"
	// PushTitle and PopTitle save and restore the window title, so a session does
	// not leave its own title on the shell.
	PushTitle = CSI + "22;0t"
	PopTitle  = CSI + "23;0t"
	// DesignateG0 introduces the character set that ordinary text bytes select.
	// DesignateASCII chooses US-ASCII, which leaves every text byte meaning itself;
	// any other set would change what the following text says.
	DesignateG0    = escapeText + "("
	DesignateASCII = DesignateG0 + "B"
)

// PushKeyboard turns on kitty keyboard enhancement flags, saving the previous set
// for PopKeyboard.
func PushKeyboard(flags int) string { return CSI + ">" + strconv.Itoa(flags) + "u" }
