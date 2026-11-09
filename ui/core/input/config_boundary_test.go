package input_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
)

func TestDecoderConstructorsRejectInvalidConfiguration(t *testing.T) {
	for _, test := range []struct {
		name  string
		build func()
		want  error
	}{
		{"parser dialect", func() { input.NewParser(input.ParserConfig{Keyboard: input.KeyboardDialect(255)}) }, input.ErrInvalidKeyboardDialect},
		{"stream dialect", func() { input.NewStream(input.StreamConfig{Keyboard: input.KeyboardDialect(255)}) }, input.ErrInvalidKeyboardDialect},
		{"negative grace", func() { input.NewStream(input.StreamConfig{Grace: -time.Second}) }, input.ErrInvalidEscapeGrace},
	} {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				err, _ := recover().(error)
				if !errors.Is(err, test.want) {
					t.Fatalf("configuration panic = %v, want %v", err, test.want)
				}
			}()
			test.build()
		})
	}
}

func TestStreamConfigurationCanBeValidatedBeforeConstruction(t *testing.T) {
	for _, test := range []struct {
		name   string
		config input.StreamConfig
		want   error
	}{
		{"default", input.StreamConfig{}, nil},
		{"kitty", input.StreamConfig{Keyboard: input.KittyKeyboard}, nil},
		{"custom grace", input.StreamConfig{Grace: time.Millisecond}, nil},
		{"unknown dialect", input.StreamConfig{Keyboard: input.KittyKeyboard + 1}, input.ErrInvalidKeyboardDialect},
		{"negative grace", input.StreamConfig{Grace: -time.Nanosecond}, input.ErrInvalidEscapeGrace},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.config.Validate(); !errors.Is(err, test.want) {
				t.Fatalf("configuration error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestValidDecoderConfigurationsPreserveModifiedKeys(t *testing.T) {
	at := time.Unix(1, 0)
	wire := []byte("\x1b[1;5A")
	for _, dialect := range []input.KeyboardDialect{input.XTermKeyboard, input.KittyKeyboard} {
		parser := input.NewParser(input.ParserConfig{Keyboard: dialect})
		stream := input.NewStream(input.StreamConfig{Keyboard: dialect})
		for _, events := range [][]input.Event{parser.Feed(wire), stream.Feed(wire, at)} {
			if len(events) != 1 || !events[0].(input.Key).Is(input.Up, input.Ctrl) {
				t.Fatalf("dialect %d decoded Ctrl+Up as %+v", dialect, events)
			}
		}
	}
}
