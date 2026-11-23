package ansi

import "strconv"

const (
	CSI = escapeText + string(rune(ControlIntroducer))
	// ST is the seven-bit string terminator. Its C1 form is never emitted because
	// that byte is also a UTF-8 continuation byte.
	ST = escapeText + string(rune(stringTerminatorFinal))
)

// Function is the final byte that selects an ECMA-48 or DEC control function.
// Encoders share these mnemonics instead of spelling final bytes independently.
type Function byte

const (
	CUU     Function = 'A'
	CUD     Function = 'B'
	CUF     Function = 'C'
	CUB     Function = 'D'
	CHA     Function = 'G'
	CUP     Function = 'H'
	ED      Function = 'J'
	EL      Function = 'K'
	SU      Function = 'S'
	SD      Function = 'T'
	DA      Function = 'c'
	VPA     Function = 'd'
	HVP     Function = 'f'
	SM      Function = 'h'
	RM      Function = 'l'
	SGR     Function = 'm'
	DECSTBM Function = 'r'
)

// AppendCSI appends one control sequence with decimal parameters. Omitted
// parameters take the function's protocol default.
func AppendCSI(dst []byte, function Function, params ...int) []byte {
	dst = append(dst, CSI...)
	for i, param := range params {
		if i > 0 {
			dst = append(dst, ';')
		}
		dst = strconv.AppendInt(dst, int64(param), 10)
	}
	return append(dst, byte(function))
}

// Mode is a DEC private mode. Its number is spelled once so that the halves
// that set and reset it cannot drift apart.
type Mode uint16

const (
	// ModeCursorVisible is DECTCEM.
	ModeCursorVisible Mode = 25
	// ModeAnyEventMouse reports motion without a button, so hover is visible.
	ModeAnyEventMouse Mode = 1003
	ModeFocusReports  Mode = 1004
	// ModeSGRMouse is the only mouse coordinate encoding that works past column 223.
	ModeSGRMouse           Mode = 1006
	ModeAltScreen          Mode = 1049
	ModeBracketedPaste     Mode = 2004
	ModeSynchronizedOutput Mode = 2026
)

// Set is DECSET for m.
func (m Mode) Set() string { return m.sequence(SM) }

// Reset is DECRST for m.
func (m Mode) Reset() string { return m.sequence(RM) }

func (m Mode) sequence(function Function) string {
	return CSI + string(rune(DECPrivate)) + strconv.Itoa(int(m)) + string(function)
}

// DECPrivate marks a parameter section as DEC's rather than ECMA-48's: a private
// mode, and a terminal's answer about one.
const DECPrivate = '?'
