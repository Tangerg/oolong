// Package input retains ambiguous bytes across read boundaries so splitting a
// terminal sequence or UTF-8 character cannot change its decoded events.
// Transports own I/O and timing; Parser owns protocol interpretation.
package input

import (
	"image"
	"strconv"
	"strings"
	"time"
)

// Event is one terminal report. A host supplies the concrete value types in this
// package. Pointers and wrappers cannot serve as immutable arrival snapshots.
type Event interface {
	terminalEvent()
}

// MaxDecodedEventBytes bounds the queue charge of any single event emitted by
// the terminal decoder. A host can use this bound to admit complete paste chunks.
const MaxDecodedEventBytes = 4 << 20

const eventOverhead = 1

// ByteSize charges one queue unit plus variable payload bytes for a normalized
// terminal event. Zero rejects a non-event or a mutable/noncanonical representation.
func ByteSize(event Event) int {
	switch e := event.(type) {
	case Key:
		return eventOverhead + len(e.Text)
	case Paste:
		return eventOverhead + len(e.Text)
	case OSC:
		return eventOverhead + len(e.Params)
	case OSCRejection:
		return eventOverhead + len(e.Prefix)
	case DCS:
		return eventOverhead + len(e.Body)
	case APC:
		return eventOverhead + len(e.Body)
	case DeviceAttributes:
		return eventOverhead + len(e.claims)
	case Mouse, KeyboardFlags, DeviceVersion, Resize, FocusIn, FocusOut:
		return eventOverhead
	default:
		return 0
	}
}

// Stamp preserves transport arrival time across queueing so key sequences,
// clicks, and wheel gestures use the same clock. Existing nonzero timestamps
// retain their producer's clock. It mutates the caller-owned event slice.
func Stamp(events []Event, at time.Time) []Event {
	for i, event := range events {
		switch timed := event.(type) {
		case Key:
			if timed.At.IsZero() {
				timed.At = at
				events[i] = timed
			}
		case Mouse:
			if timed.At.IsZero() {
				timed.At = at
				events[i] = timed
			}
		}
	}
	return events
}

type Mods uint8

// Modifier bits follow the Kitty wire encoding before its one-based offset.
const (
	Shift Mods = 1 << iota
	Alt
	Ctrl
	// Super is the platform's own modifier — Command on macOS, the Windows key
	// elsewhere. Only terminals speaking the Kitty keyboard protocol report it.
	Super
	Hyper
	Meta
)

func (m Mods) Has(want Mods) bool { return m&want == want }

var modifierNames = [...]struct {
	mod  Mods
	name string
}{{Ctrl, "ctrl"}, {Alt, "alt"}, {Shift, "shift"}, {Super, "super"}, {Hyper, "hyper"}, {Meta, "meta"}}

func (m Mods) String() string {
	var parts []string
	for _, named := range modifierNames {
		if m.Has(named.mod) {
			parts = append(parts, named.name)
		}
	}
	return strings.Join(parts, "+")
}

// Code identifies which key was pressed. [Character] means the key produced text,
// carried in [Key.Rune].
type Code int

const (
	// Character is the zero value, so a Key literal with only a rune in it is a
	// character press — which is what most of them are.
	Character Code = iota
	Enter
	Esc
	Backspace
	Tab
	Up
	Down
	Left
	Right
	Home
	End
	PageUp
	PageDown
	Delete
	Insert
	F1
	F2
	F3
	F4
	F5
	F6
	F7
	F8
	F9
	F10
	F11
	F12
	F13
	F14
	F15
	F16
	F17
	F18
	F19
	F20
	F21
	F22
	F23
	F24
	F25
	F26
	F27
	F28
	F29
	F30
	F31
	F32
	F33
	F34
	F35
	Keypad0
	Keypad1
	Keypad2
	Keypad3
	Keypad4
	Keypad5
	Keypad6
	Keypad7
	Keypad8
	Keypad9
	KeypadDecimal
	KeypadDivide
	KeypadMultiply
	KeypadSubtract
	KeypadAdd
	KeypadEnter
	KeypadEqual
	KeypadSeparator
	KeypadLeft
	KeypadRight
	KeypadUp
	KeypadDown
	KeypadPageUp
	KeypadPageDown
	KeypadHome
	KeypadEnd
	KeypadInsert
	KeypadDelete
	KeypadBegin
)

func (c Code) String() string {
	if name, ok := codeNames[c]; ok {
		return name
	}
	if key, ok := keypadKeys[c]; ok {
		return key.name
	}
	if c >= F1 && c <= F35 {
		return "f" + strconv.Itoa(int(c-F1)+1)
	}
	return "unknown"
}

var codeNames = map[Code]string{
	Character: "char",
	Enter:     "enter",
	Esc:       "esc",
	Backspace: "backspace",
	Tab:       "tab",
	Up:        "up",
	Down:      "down",
	Left:      "left",
	Right:     "right",
	Home:      "home",
	End:       "end",
	PageUp:    "pageup",
	PageDown:  "pagedown",
	Delete:    "delete",
	Insert:    "insert",
}

type Transition uint8

// Terminals without event-type reporting produce Press for every key event.
const (
	// Press is the zero value: an ordinary terminal only ever reports presses,
	// and a Key literal that says nothing about its transition means one.
	Press Transition = iota
	Repeat
	Release
)

// Key carries a character as [Character] with the rune in Rune. Ctrl held with a
// letter also arrives as a character — the letter, lowercased, with [Ctrl] in
// Mods — because that is what the terminal actually sends and inventing a
// separate representation for it would mean two ways to ask the same question.
type Key struct {
	Code Code
	Rune rune
	// ShiftedRune is the character for the shifted key in the active layout.
	// BaseRune is the same physical key in the standard PC-101 layout.
	// Either is zero when the terminal omitted that alternate identity.
	ShiftedRune rune
	BaseRune    rune
	Mods        Mods
	Transition  Transition
	// Text is what the key produced, when the terminal was able to say. It can
	// hold more than one code point, and is empty on terminals that do not report
	// it. [Key.FallbackRune] supplies the character fallback.
	Text string
	// At is when the transport first observed this key's bytes, so assembly and
	// queueing cannot move it across a frame or key-sequence boundary. Zero is untimed.
	At time.Time
}

func (Key) terminalEvent() {}

// Is requires exact modifiers so a Ctrl binding cannot claim Ctrl+Shift.
func (k Key) Is(code Code, mods Mods) bool {
	return k.Code == code && k.Mods == mods
}

func (k Key) IsRune(r rune, mods Mods) bool {
	return k.Code == Character && k.Rune == r && k.Mods == mods
}

// Down reports whether the key is going down — pressed or auto-repeating.
// Most handlers want this rather than Press alone, or holding a key stops working
// on terminals that report repeats.
func (k Key) Down() bool { return k.Transition != Release }

func (k Key) String() string { return k.Chord().String() }

type MouseAction uint8

// Wheel directions are actions because scrolling does not hold a button.
const (
	MouseDown MouseAction = iota
	MouseUp
	MouseDrag
	MouseMove
	WheelUp
	WheelDown
	// MouseLeave is synthesized by a routing boundary when the pointer stops
	// targeting a child. It invalidates hover without ending an accepted gesture.
	MouseLeave
	// MouseCancel is synthesized when a routing owner ends a gesture without a
	// release, such as a new press superseding the accepted one. It cannot click.
	MouseCancel
)

type Button uint8

const (
	// ButtonNone means no button for movement or a wheel, and an unspecified
	// released button on MouseUp.
	ButtonNone Button = iota
	ButtonLeft
	ButtonMiddle
	ButtonRight
)

// Mouse is a mouse event, positioned in cells with the origin at the top left.
type Mouse struct {
	Pos    image.Point
	Action MouseAction
	Button Button
	Mods   Mods
	// At is when the transport first observed this report's bytes. Completing a
	// split report cannot authorize its old coordinates against a newer frame.
	// Zero means arrival was not timed.
	At time.Time
}

func (Mouse) terminalEvent() {}

// Releases reports whether this is a release of button. ButtonNone on the event
// means the released button is unspecified and ends the current captured press.
func (m Mouse) Releases(button Button) bool {
	return m.Action == MouseUp && m.matchesButton(button)
}

// Drags reports whether this is a drag of button. ButtonNone leaves the button
// unspecified and continues the current captured press.
func (m Mouse) Drags(button Button) bool {
	return m.Action == MouseDrag && m.matchesButton(button)
}

func (m Mouse) matchesButton(button Button) bool {
	return m.Button == ButtonNone || m.Button == button
}

// Paste separates payload from keystrokes so embedded shortcuts cannot run
// while the caller inserts text.
type Paste struct{ Text string }

func (Paste) terminalEvent() {}

type OSC struct {
	Command int
	// Params is everything after the command number and its semicolon, left as
	// the terminal wrote it apart from invalid UTF-8 being replaced.
	//
	// What it means depends on the command, which this package deliberately does
	// not work out. Reading a background colour belongs to whatever owns colours;
	// this package owns bytes.
	Params string
}

func (OSC) terminalEvent() {}

// OSCRejection proves that an oversized OSC reached a complete terminator.
// Prefix retains only its first semicolon-terminated parameter, including that
// separator, when the complete header fits the decoder's parameter budget.
// Otherwise it is empty. No payload is retained. Query owners must still match
// this evidence to their own outstanding reply before settling a failure.
type OSCRejection struct {
	Command int
	Prefix  string
}

func (OSCRejection) terminalEvent() {}

// DCS leaves its body uninterpreted because reply syntax belongs to the query
// owner. Only recognized reply prefixes are framed, preserving Alt+Shift+P.
type DCS struct{ Body string }

func (DCS) terminalEvent() {}

// APC carries an application-program command reply. The parser recognizes Kitty
// graphics acknowledgements beginning with Gi= and retains their complete body.
type APC struct{ Body string }

func (APC) terminalEvent() {}

// KeyboardFlags is a terminal's answer about which keyboard protocol enhancements
// are turned on.
//
// Asking is not the same as being answered, and being answered is not the same as
// having asked. A terminal may accept the request for unambiguous key codes and give
// nothing for key releases — the protocol is live, the teardown still owes a pop, and
// no release ever arrives. Nothing in the events themselves distinguishes that from a
// user who simply has not lifted a key, so the only way to know is to read back what
// took.
type KeyboardFlags struct{ Features KeyboardFeatures }

func (KeyboardFlags) terminalEvent() {}

// KeyboardFeatures is a set of progressive enhancements requested from or reported
// by a terminal speaking the Kitty keyboard protocol.
//
// It is shared by terminal configuration and the negotiated result so there cannot
// be two vocabularies for one protocol. Zero is the historical keyboard encoding.
type KeyboardFeatures int

const (
	// KeyboardDisambiguate makes every key arrive as an unambiguous code rather than as
	// whatever byte it historically produced. It is what makes Shift+Enter and
	// Ctrl+Enter tellable apart from Enter.
	KeyboardDisambiguate KeyboardFeatures = 1 << iota
	// KeyboardReportEvents adds key releases and repeats. Without it a key going down is
	// all there is, and anything held cannot be known to have been let go.
	KeyboardReportEvents
	KeyboardReportAlternates
	KeyboardReportAllAsEscapes
	// KeyboardReportText requests associated text. Session setup also enables
	// KeyboardReportAllAsEscapes, its prerequisite in the Kitty protocol.
	KeyboardReportText

	// KeyboardAll is every enhancement this version understands. Unknown bits a
	// future terminal reports remain in KeyboardFlags, but are never emitted by a
	// request built by this version.
	KeyboardAll = KeyboardDisambiguate | KeyboardReportEvents | KeyboardReportAlternates |
		KeyboardReportAllAsEscapes | KeyboardReportText
)

func (k KeyboardFeatures) Has(features KeyboardFeatures) bool { return k&features == features }

// DeviceVersion retains raw numbers because their interpretation depends on
// the terminal; the protocol does not define a shared version encoding.
type DeviceVersion struct {
	Kind           int
	Version, Patch int
}

func (DeviceVersion) terminalEvent() {}

// DeviceAttributes can delimit an earlier capability query: a session may
// settle unanswered probes when this later response arrives.
type DeviceAttributes struct {
	// Class is the terminal class the answer led with: 62 for a VT220, 64 for a
	// VT420. Little depends on it, and terminals that emulate one of those are
	// not otherwise alike.
	Class int
	// Claims stay in a string so Event values remain comparable. A slice here
	// would make interface equality panic at runtime.
	claims string
}

func Attributes(class int, features ...int) DeviceAttributes {
	var b strings.Builder
	for _, n := range features {
		if n <= 0 {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(';')
		}
		b.WriteString(strconv.Itoa(n))
	}
	return DeviceAttributes{Class: class, claims: b.String()}
}

func (DeviceAttributes) terminalEvent() {}

// Features are the numbered extensions the terminal claimed. Sixel graphics is 4.
// There is no authority over the list and a terminal may claim what it does not do,
// so this is evidence rather than proof.
func (d DeviceAttributes) Features() []int {
	if d.claims == "" {
		return nil
	}
	fields := strings.Split(d.claims, ";")
	out := make([]int, 0, len(fields))
	for _, field := range fields {
		if n, err := strconv.Atoi(field); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// Has reads claims directly so checking one capability need not allocate a
// slice of every advertised extension.
func (d DeviceAttributes) Has(n int) bool {
	if n <= 0 {
		return false
	}
	for field := range strings.SplitSeq(d.claims, ";") {
		if claimed, err := strconv.Atoi(field); err == nil && claimed == n {
			return true
		}
	}
	return false
}

// Resize reports the terminal's new size in cells.
type Resize struct{ Width, Height int }

func (Resize) terminalEvent() {}

type FocusIn struct{}

func (FocusIn) terminalEvent() {}

type FocusOut struct{}

func (FocusOut) terminalEvent() {}

func (m Mods) Rune(r rune) Chord { return Chord{Code: Character, Rune: r, Mods: m} }

func (m Mods) With(code Code) Chord { return Chord{Code: code, Mods: m} }

func (k Key) Chord() Chord { return Chord{Code: k.Code, Rune: k.Rune, Mods: k.Mods} }
