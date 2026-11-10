package input_test

import (
	"errors"
	"image"
	"testing"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/input"
)

func TestEventValidationRejectsInvalidValues(t *testing.T) {
	for _, test := range []struct {
		name  string
		event input.Event
	}{
		{"nil", nil},
		{"mutable", &input.Key{Code: input.Enter}},
		{"negative code", input.Key{Code: -1}},
		{"unknown code", input.Key{Code: input.KeypadBegin + 1}},
		{"unknown transition", input.Key{Code: input.Enter, Transition: 255}},
		{"unknown key modifier", input.Key{Rune: 'a', Mods: 1 << 6}},
		{"invalid rune", input.Key{Rune: 0xd800}},
		{"invalid alternate", input.Key{Rune: 'a', BaseRune: utf8.MaxRune + 1}},
		{"control identity", input.Key{Rune: 0x1f}},
		{"control alternate", input.Key{Rune: 'a', ShiftedRune: '\u0085', Mods: input.Shift}},
		{"two identities", input.Key{Code: input.Enter, Rune: 'a'}},
		{"unshifted alternate", input.Key{Rune: 'a', ShiftedRune: 'A'}},
		{"missing primary", input.Key{BaseRune: 'a'}},
		{"invalid key text", input.Key{Text: "\xff"}},
		{"control key text", input.Key{Text: "\t"}},
		{"invalid paste", input.Paste{Text: "\xff"}},
		{"unknown mouse action", input.Mouse{Action: 255}},
		{"unknown mouse button", input.Mouse{Button: 255}},
		{"unknown mouse modifier", input.Mouse{Mods: 1 << 7}},
		{"move with button", input.Mouse{Action: input.MouseMove, Button: input.ButtonLeft}},
		{"wheel with button", input.Mouse{Action: input.WheelUp, Button: input.ButtonLeft}},
		{"negative command", input.OSC{Command: -1}},
		{"invalid OSC text", input.OSC{Params: "\xff"}},
		{"invalid rejected OSC text", input.OSCRejection{Prefix: "\xff"}},
		{"invalid DCS text", input.DCS{Body: "\xff"}},
		{"invalid APC text", input.APC{Body: "\xff"}},
		{"negative keyboard flags", input.KeyboardFlags{Features: -1}},
		{"negative device version", input.DeviceVersion{Patch: -1}},
		{"negative device class", input.Attributes(-1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := input.Validate(test.event); !errors.Is(err, input.ErrInvalidEvent) {
				t.Fatalf("Validate(%#v) = %v", test.event, err)
			}
		})
	}
	if (input.Key{Transition: 255}).Down() {
		t.Fatal("unknown transition became a press")
	}
}

func TestEventValidationPreservesProtocolAndSyntheticContracts(t *testing.T) {
	for _, event := range []input.Event{
		input.Key{},
		input.Key{Transition: input.Release},
		input.Key{Text: "合成文本", Transition: input.Repeat},
		input.Key{Rune: 'a', ShiftedRune: 'A', BaseRune: 'q', Mods: input.Shift | input.Ctrl | input.Alt | input.Super | input.Hyper | input.Meta},
		input.Key{Rune: 'a', Text: "a", Transition: input.Release},
		input.Key{Rune: '\u00a0'},
		input.Paste{Text: "\t\n\x1b[A"},
		input.Mouse{},
		input.Mouse{Action: input.MouseDrag, Pos: image.Pt(-1, -2)},
		input.Mouse{Action: input.MouseUp},
		input.Mouse{Action: input.MouseCancel, Button: input.ButtonLeft},
		input.Mouse{Action: input.MouseLeave, Button: input.ButtonRight},
		input.KeyboardFlags{Features: 1 << 20},
		input.DeviceVersion{},
		input.Attributes(62, 1, 4),
		input.OSC{Command: 52, Params: "c;"},
		input.DCS{Body: "1$r2 q"},
		input.APC{Body: "Gi=1;OK"},
		input.Resize{},
		input.FocusIn{},
		input.FocusOut{},
	} {
		if err := input.Validate(event); err != nil {
			t.Errorf("valid event %#v refused: %v", event, err)
		}
	}
}

func FuzzParserEmitsValidEvents(f *testing.F) {
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, wire []byte) {
		for _, dialect := range []input.KeyboardDialect{input.XTermKeyboard, input.KittyKeyboard} {
			parser := input.NewParser(input.ParserConfig{Keyboard: dialect})
			for _, event := range append(parser.Feed(wire), parser.Flush()...) {
				if err := input.Validate(event); err != nil {
					t.Fatalf("dialect %v emitted invalid %#v from %q: %v", dialect, event, wire, err)
				}
			}
		}
	})
}
