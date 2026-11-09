package input

import (
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"
)

var ErrInvalidEvent = errors.New("input: invalid event")

// Validate admits immutable terminal values from byte decoders and synthetic
// producers under one semantic contract. Query payload meaning and terminal size
// policy remain with their respective owners. Unknown nonnegative keyboard
// feature bits remain valid evidence from future terminals.
func Validate(event Event) error {
	if ByteSize(event) == 0 {
		return fmt.Errorf("%w: noncanonical value %T", ErrInvalidEvent, event)
	}
	switch e := event.(type) {
	case Key:
		return e.validate()
	case Mouse:
		return e.validate()
	case Paste:
		return validateText(e.Text)
	case OSC:
		return validateOSC(e.Command, e.Params)
	case OSCRejection:
		return validateOSC(e.Command, e.Prefix)
	case DCS:
		return validateText(e.Body)
	case APC:
		return validateText(e.Body)
	case KeyboardFlags:
		if e.Features < 0 {
			return fmt.Errorf("%w: negative keyboard features", ErrInvalidEvent)
		}
	case DeviceVersion:
		return e.validate()
	case DeviceAttributes:
		if e.Class < 0 {
			return fmt.Errorf("%w: negative device class", ErrInvalidEvent)
		}
	}
	return nil
}

func (k Key) validate() error {
	if k.Transition > Release {
		return fmt.Errorf("%w: key transition %d", ErrInvalidEvent, k.Transition)
	}
	if k.Mods&^shortcutMods != 0 {
		return fmt.Errorf("%w: key modifiers %d", ErrInvalidEvent, k.Mods)
	}
	if err := k.validateIdentity(); err != nil {
		return err
	}
	if k.ShiftedRune != 0 && !k.Mods.Has(Shift) {
		return fmt.Errorf("%w: shifted identity without Shift", ErrInvalidEvent)
	}
	if k.Code == Character && k.Rune == 0 && (k.ShiftedRune != 0 || k.BaseRune != 0) {
		return fmt.Errorf("%w: alternate identity without a primary key", ErrInvalidEvent)
	}
	if err := validateText(k.Text); err != nil {
		return err
	}
	for _, r := range k.Text {
		if !associatedTextRune(r) {
			return fmt.Errorf("%w: key text contains a control character", ErrInvalidEvent)
		}
	}
	return nil
}

func (k Key) validateIdentity() error {
	if k.Code < Character || k.Code > KeypadBegin {
		return fmt.Errorf("%w: key code %d", ErrInvalidEvent, k.Code)
	}
	for _, r := range [...]rune{k.Rune, k.ShiftedRune, k.BaseRune} {
		if !utf8.ValidRune(r) {
			return fmt.Errorf("%w: key identity is not a Unicode scalar", ErrInvalidEvent)
		}
	}
	if k.Code != Character && k.Rune != 0 {
		return fmt.Errorf("%w: functional key also names a character", ErrInvalidEvent)
	}
	return nil
}

func (d DeviceVersion) validate() error {
	if d.Kind < 0 || d.Version < 0 || d.Patch < 0 {
		return fmt.Errorf("%w: negative device version field", ErrInvalidEvent)
	}
	return nil
}

func (m Mouse) validate() error {
	if m.Action > MouseCancel {
		return fmt.Errorf("%w: mouse action %d", ErrInvalidEvent, m.Action)
	}
	if m.Button > ButtonRight {
		return fmt.Errorf("%w: mouse button %d", ErrInvalidEvent, m.Button)
	}
	if m.Mods&^shortcutMods != 0 {
		return fmt.Errorf("%w: mouse modifiers %d", ErrInvalidEvent, m.Mods)
	}
	if m.Button != ButtonNone && (m.Action == MouseMove || m.Action == WheelUp || m.Action == WheelDown) {
		return fmt.Errorf("%w: non-button mouse action names a button", ErrInvalidEvent)
	}
	return nil
}

func validateText(text string) error {
	if !utf8.ValidString(text) {
		return fmt.Errorf("%w: invalid UTF-8 payload", ErrInvalidEvent)
	}
	return nil
}

func validateOSC(command int, body string) error {
	if command < 0 {
		return fmt.Errorf("%w: negative OSC command", ErrInvalidEvent)
	}
	return validateText(body)
}

func associatedTextRune(r rune) bool { return !unicode.IsControl(r) }
