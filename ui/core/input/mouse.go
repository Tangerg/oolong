package input

import "image"

// Both report encodings carry the same three fields with the same button layout;
// they differ only in framing and in how a release is told apart from a press.
const mouseReportFields = 3

const (
	mouseLeftButton = iota
	mouseMiddleButton
	mouseRightButton
	mouseNoButton
)

const (
	mouseShiftBit = 1 << (iota + 2)
	mouseAltBit
	mouseCtrlBit
	mouseMotionBit
	mouseWheelBit
)

const (
	mouseButtonMask = mouseNoButton
	mouseReportMask = mouseButtonMask | mouseShiftBit | mouseAltBit | mouseCtrlBit | mouseMotionBit | mouseWheelBit
	mouseWheelUp    = 0
	mouseWheelDown  = 1
)

const (
	sgrMousePress   = 'M'
	sgrMouseRelease = 'm'
)

// A terminal that tracks motion but not SGR coordinates falls back to the X10
// encoding: each field is one raw byte offset by a space. The report is framed
// by its length, and a coordinate past what a byte can hold arrives out of range.
const (
	x10MouseIntro  = "\x1b[M"
	x10MouseOffset = ' '
)

// mouseReport is a pointer report with its encoding removed. Coordinates count
// from one, as the terminal does.
type mouseReport struct {
	bits     int
	x, y     int
	released bool
}

// decodeX10Mouse takes the fields whole or not at all: they are raw bytes that
// can look like characters, and any left behind would be typed into the focus.
func decodeX10Mouse(b []byte) (n int, ev Event, done bool) {
	n = len(x10MouseIntro) + mouseReportFields
	if len(b) < n {
		return 0, nil, false
	}
	fields := b[len(x10MouseIntro):n]
	bits := int(fields[0]) - x10MouseOffset
	report := mouseReport{
		bits: bits,
		x:    int(fields[1]) - x10MouseOffset,
		y:    int(fields[2]) - x10MouseOffset,
		// The encoding has no release form. A release reports no button, so it
		// cannot say which one was let go.
		released: bits&(mouseButtonMask|mouseMotionBit|mouseWheelBit) == mouseNoButton,
	}
	return n, report.event(), true
}

func (r mouseReport) event() Event {
	if r.bits&^mouseReportMask != 0 || r.x < 1 || r.y < 1 {
		return nil
	}
	// The terminal counts from one; everything above this package counts from zero.
	ev := Mouse{Pos: image.Pt(r.x-1, r.y-1), Mods: r.mods()}
	if r.bits&mouseWheelBit != 0 {
		action, ok := r.wheel()
		if !ok {
			return nil
		}
		ev.Action = action
		return ev
	}
	ev.Button = r.button()
	switch {
	case r.bits&mouseMotionBit != 0 && ev.Button == ButtonNone:
		ev.Action = MouseMove
	case r.bits&mouseMotionBit != 0:
		ev.Action = MouseDrag
	case r.released:
		ev.Action = MouseUp
	default:
		ev.Action = MouseDown
	}
	return ev
}

// wheel reads the direction from the button field, which a wheel report reuses.
// The horizontal wheel is refused because nothing here reads it.
func (r mouseReport) wheel() (MouseAction, bool) {
	switch r.bits & mouseButtonMask {
	case mouseWheelUp:
		return WheelUp, true
	case mouseWheelDown:
		return WheelDown, true
	default:
		return 0, false
	}
}

func (r mouseReport) button() Button {
	switch r.bits & mouseButtonMask {
	case mouseLeftButton:
		return ButtonLeft
	case mouseMiddleButton:
		return ButtonMiddle
	case mouseRightButton:
		return ButtonRight
	default:
		return ButtonNone
	}
}

func (r mouseReport) mods() Mods {
	var mods Mods
	if r.bits&mouseShiftBit != 0 {
		mods |= Shift
	}
	if r.bits&mouseAltBit != 0 {
		mods |= Alt
	}
	if r.bits&mouseCtrlBit != 0 {
		mods |= Ctrl
	}
	return mods
}
