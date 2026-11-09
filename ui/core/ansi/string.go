package ansi

type StringFamily byte

const (
	OSC StringFamily = ']'
	DCS StringFamily = 'P'
	APC StringFamily = '_'
	PM  StringFamily = '^'
	SOS StringFamily = 'X'
)

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
	case (current == cancel || current == substitute) && f == DCS:
		return 1, StringCanceled
	case current != Escape:
		return 1, StringContent
	case !hasNext:
		return 0, StringIncomplete
	case next == '\\':
		return 2, StringTerminated
	default:
		return 0, StringCanceled
	}
}
