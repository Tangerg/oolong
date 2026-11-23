package ansi

import "strconv"

type StringFamily byte

const (
	OSC StringFamily = ']'
	DCS StringFamily = 'P'
	APC StringFamily = '_'
	PM  StringFamily = '^'
	SOS StringFamily = 'X'
)

// Each introducer is Escape followed by its family byte above.
const (
	OSCIntroducer = escapeText + string(OSC)
	DCSIntroducer = escapeText + string(DCS)
	APCIntroducer = escapeText + string(APC)
)

// OSC commands the framework sends or reads. Encoders, decoders and output models
// share these numbers, as they share SGR parameters.
const (
	// OSCTitle sets the icon and window titles together, so destinations that tell
	// them apart cannot keep inconsistent names.
	OSCTitle     = 0
	OSCDirectory = 7
	// OSCHyperlink opens and closes a hyperlink.
	OSCHyperlink = 8
	// OSCNotify posts a notification. ConEmu also reads task progress from it.
	OSCNotify     = 9
	OSCForeground = 10
	OSCBackground = 11
	OSCClipboard  = 52
	// OSCInlineFile is iTerm2's inline image transfer.
	OSCInlineFile = 1337
)

// OSCPrefix starts an OSC sequence for command, up to its first parameter. The
// caller adds the parameters and a terminator.
func OSCPrefix(command int) string { return OSCIntroducer + strconv.Itoa(command) + ";" }

func (f StringFamily) Valid() bool {
	switch f {
	case OSC, DCS, APC, PM, SOS:
		return true
	default:
		return false
	}
}

type StringResult uint8

const (
	StringContent StringResult = iota
	StringTerminated
	StringCanceled
	StringIncomplete
)

const (
	cancel     = 0x18
	substitute = 0x1a
)

// Step owns the boundary shared by retained and discarded string bodies. A
// canceled Escape consumes nothing: it belongs to the next sequence. The C1 ST
// byte is excluded because it can occur inside UTF-8 text.
func (f StringFamily) Step(current, next byte, hasNext bool) (int, StringResult) {
	switch {
	case current == Bell && f == OSC:
		return 1, StringTerminated
	case current == cancel || current == substitute:
		return 1, StringCanceled
	case current != Escape:
		return 1, StringContent
	case !hasNext:
		return 0, StringIncomplete
	case next == stringTerminatorFinal:
		return 2, StringTerminated
	default:
		return 0, StringCanceled
	}
}
