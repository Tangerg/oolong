// Package input turns the bytes a terminal sends into events.
//
// A terminal reports input as a stream that mixes plain text with escape
// sequences, and it splits that stream wherever the read happens to land: half a
// sequence in one read and half in the next is normal, not an error. [Parser] is
// therefore incremental — it is fed whatever arrived and returns whatever is now
// unambiguous.
//
// Nothing here touches a terminal. The parser is a function of its bytes, which
// is what lets every sequence this package claims to understand be stated as a
// test.
package input

import (
	"image"
	"strconv"
	"strings"
	"time"
)

// Event is one thing the terminal reported. The set is closed by the unexported
// method: a consumer's switch over events is exhaustive by construction.
type Event interface {
	terminalEvent()
}

// Handler is anything that answers an event, reporting whether it consumed it.
//
// It lives here for the same reason [grid.Drawer] lives with the view: the word
// belongs to the layer that owns the thing being handled. Everything further up
// that answers input says so by embedding this, so "consumed" means one thing
// across the whole repository rather than one thing per layer that happens to
// line up.
//
// An unconsumed event carries on to whatever else might want it, which is how a
// key can mean one thing inside a text field and another outside it without
// either side knowing about the other.
type Handler interface {
	Handle(ev Event) bool
}

// Mods is the set of modifier keys held during an event.
type Mods uint8

// The modifiers a terminal can report. Super is last because it is the only one
// that needs the Kitty protocol to arrive at all.
const (
	Shift Mods = 1 << iota
	Alt
	Ctrl
	// Super is the platform's own modifier — Command on macOS, the Windows key
	// elsewhere. Only terminals speaking the Kitty keyboard protocol report it.
	Super
)

// Has reports whether every modifier in want is held.
func (m Mods) Has(want Mods) bool { return m&want == want }

// String names the modifiers in the order a keybinding is conventionally written.
func (m Mods) String() string {
	var parts []string
	for _, named := range []struct {
		mod  Mods
		name string
	}{{Ctrl, "ctrl"}, {Alt, "alt"}, {Shift, "shift"}, {Super, "super"}} {
		if m.Has(named.mod) {
			parts = append(parts, named.name)
		}
	}
	return strings.Join(parts, "+")
}

// Code identifies which key was pressed. [Character] means the key produced text,
// carried in [Key.Rune].
type Code int

// The keys a terminal can report: a character press, then the named keys in the
// order a keyboard is usually described — what finishes a line, what cancels,
// what edits, then movement, then the function row.
const (
	// Character is the zero value, so a Key literal with only a rune in it is a
	// character press — which is what most of them are.
	Character Code = iota
	Enter
	Esc
	Backspace
	Tab
	Backtab
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
)

// String names the key the way a help line would print it.
func (c Code) String() string {
	if name, ok := codeNames[c]; ok {
		return name
	}
	if c >= F1 && c <= F12 {
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
	Backtab:   "shift+tab",
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

// Transition is what happened to a key.
type Transition uint8

// What happened to a key. Repeat and Release only ever arrive from a terminal
// speaking the Kitty keyboard protocol; everything else reports presses.
const (
	// Press is the zero value: an ordinary terminal only ever reports presses,
	// and a Key literal that says nothing about its transition means one.
	Press Transition = iota
	Repeat
	Release
)

// Key is a keyboard event.
//
// A character key arrives as [Character] with the rune in Rune. Ctrl held with a
// letter also arrives as a character — the letter, lowercased, with [Ctrl] in
// Mods — because that is what the terminal actually sends and inventing a
// separate representation for it would mean two ways to ask the same question.
type Key struct {
	Code Code
	Rune rune
	Mods Mods
	// Transition is Press unless the terminal speaks the Kitty keyboard protocol,
	// which is the only way repeats and releases are ever reported.
	Transition Transition
	// Text is what the key produced, when the terminal was able to say. It can
	// hold more than one code point, and is empty on terminals that do not report
	// it — Rune is the fallback and the common case.
	Text string
}

func (Key) terminalEvent() {}

// Is reports whether the key is code with exactly mods held.
//
// Exactly, not at least: a binding on Ctrl+C that also fired for Ctrl+Shift+C
// would swallow a keystroke its owner never claimed.
func (k Key) Is(code Code, mods Mods) bool {
	return k.Code == code && k.Mods == mods
}

// IsRune reports whether the key is the character r with exactly mods held.
func (k Key) IsRune(r rune, mods Mods) bool {
	return k.Code == Character && k.Rune == r && k.Mods == mods
}

// Down reports whether the key is going down — pressed or auto-repeating.
// Most handlers want this rather than Press alone, or holding a key stops working
// on terminals that report repeats.
func (k Key) Down() bool { return k.Transition != Release }

// String names the keystroke the way a help line or a keybinding file writes it.
func (k Key) String() string {
	var b strings.Builder
	if mods := k.Mods.String(); mods != "" {
		b.WriteString(mods)
		b.WriteByte('+')
	}
	if k.Code == Character {
		if k.Rune == ' ' {
			b.WriteString("space")
		} else {
			b.WriteRune(k.Rune)
		}
		return b.String()
	}
	b.WriteString(k.Code.String())
	return b.String()
}

// MouseAction is what the mouse did.
type MouseAction uint8

// What the mouse did. Drag is a move with a button held, and the two wheel
// directions are actions rather than buttons because no button is involved.
const (
	MouseDown MouseAction = iota
	MouseUp
	MouseDrag
	MouseMove
	WheelUp
	WheelDown
)

// Button identifies which mouse button an action belongs to.
type Button uint8

// The buttons a terminal reports. There is no fourth: the higher button numbers
// in the protocol are the wheel, which arrives as an action instead.
const (
	// ButtonNone is the zero value, which is right for a bare move and for a
	// wheel: neither belongs to a button.
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
	// At is when the report arrived, as whatever read it saw. It is zero when nothing
	// timed it, which is what a parser fed bytes directly produces.
	//
	// A mouse report means different things depending on when it came. Two presses
	// close together are a double-click and a terminal never says so; a run of wheel
	// reports without a gap is a trackpad and not the wheel. Both questions are about
	// arrival, and the only thing that knows the answer is the goroutine that did the
	// reading — so it is stamped there rather than left for every caller to supply a
	// clock for a fact the library already had.
	At time.Time
}

func (Mouse) terminalEvent() {}

// Paste is a block of text the terminal delivered as a paste rather than as
// keystrokes, so it can be inserted whole instead of being interpreted a
// character at a time.
type Paste struct{ Text string }

func (Paste) terminalEvent() {}

// OSC is an operating system command the terminal sent.
//
// This is how a terminal answers a question. A program writes a query, and the
// answer comes back on the input stream mixed in with whatever the user is
// typing — asking what colour the terminal draws on and reading its clipboard
// both work this way. A session that asks nothing never sees one.
type OSC struct {
	// Command is the number the sequence names itself by: 11 for the colour the
	// terminal draws on, 52 for its clipboard.
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

// DCS is a device control string the terminal sent.
//
// It is the other shape an answer comes in, and the one that carries a terminal's own
// name and version — the reply to the version query is ">|kitty(0.32.2)". There is no
// command number and no single grammar, so the body comes back as it was written and
// what it means is decided by whoever asked.
//
// Only the shapes a terminal actually replies in are decoded as one. See the package's
// own notes on why: the introducer is also Alt+Shift+P.
type DCS struct{ Body string }

func (DCS) terminalEvent() {}

// KeyboardFlags is a terminal's answer about which of the Kitty keyboard protocol's
// enhancements are turned on.
//
// Asking is not the same as being answered, and being answered is not the same as
// having asked. A terminal may accept the request for unambiguous key codes and give
// nothing for key releases — the protocol is live, the teardown still owes a pop, and
// no release ever arrives. Nothing in the events themselves distinguishes that from a
// user who simply has not lifted a key, so the only way to know is to read back what
// took.
type KeyboardFlags struct{ Flags int }

func (KeyboardFlags) terminalEvent() {}

// The Kitty keyboard protocol's progressive enhancements, as the bits a terminal
// reports them in.
const (
	// KittyDisambiguate makes every key arrive as an unambiguous code rather than as
	// whatever byte it historically produced. It is what makes Shift+Enter and
	// Ctrl+Enter tellable apart from Enter.
	KittyDisambiguate = 1 << iota
	// KittyReportEvents adds key releases and repeats. Without it a key going down is
	// all there is, and anything held cannot be known to have been let go.
	KittyReportEvents
	// KittyReportAlternates adds the key a different layout would have produced.
	KittyReportAlternates
	// KittyReportAllAsEscapes makes even plain letters arrive as sequences.
	KittyReportAllAsEscapes
	// KittyReportText adds the text a key produced, which the terminal knows and a
	// program guessing from a keycode does not.
	KittyReportText
)

// Has reports whether a flag is among those the terminal turned on.
func (k KeyboardFlags) Has(flag int) bool { return k.Flags&flag == flag }

// DeviceVersion is a terminal's answer to being asked which version of itself it is.
//
// It is the query for the terminals that answer nothing else. Alacritty exports no
// version in the environment and declines the version string on principle; this is
// what it does answer.
//
// The numbers are as the terminal sent them, because what they mean is the terminal's
// convention and not a standard: most pack a version as major, minor and patch into
// [DeviceVersion.Version], and reading it as anything is a bet on which terminal is
// being asked.
type DeviceVersion struct {
	// Kind is the terminal class number, which says very little.
	Kind int
	// Version and Patch are what it reported.
	Version, Patch int
}

func (DeviceVersion) terminalEvent() {}

// DeviceAttributes is a terminal's answer to being asked what it is.
//
// Every terminal answers this one, which makes it useful for more than what it
// says. A question a terminal might not understand can be followed by this one,
// and this answer arriving without the other is how a terminal says it did not
// understand — which is not something any terminal says out loud.
type DeviceAttributes struct {
	// Class is the terminal class the answer led with: 62 for a VT220, 64 for a
	// VT420. Little depends on it, and terminals that emulate one of those are
	// not otherwise alike.
	Class int
	// claims is the numbered extensions, semicolon-separated as the terminal wrote
	// them.
	//
	// A string rather than a slice so that this event is comparable, like every
	// other one. Comparing events through the interface is what a caller does and
	// what this package's own tests do, and a slice field turns that into a panic
	// at run time rather than an error at compile time — the fuzz target that
	// checks a split read decodes the same way found exactly that.
	claims string
}

// Attributes is an answer with the given class and extensions, for anything
// standing in for a terminal.
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

// Has reports whether the terminal claimed extension n.
//
// It reads the claims rather than building the slice, because this is asked once per
// capability and a session asks about two of them.
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

// FocusIn reports that the terminal window took focus.
type FocusIn struct{}

func (FocusIn) terminalEvent() {}

// FocusOut reports that the terminal window lost focus.
type FocusOut struct{}

func (FocusOut) terminalEvent() {}
