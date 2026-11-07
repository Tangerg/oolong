package program_test

import (
	"errors"
	"io"
	"math"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
)

func TestInvalidColorConfigurationFailsBeforeOpeningTheSession(t *testing.T) {
	cfg := program.Config{Color: grid.Depth(math.MaxUint8)}
	if _, err := program.NewScreen(program.ScreenConfig{Config: cfg, Build: func(*program.Runtime) (scene.Element, error) {
		t.Fatal("invalid configuration invoked the builder")
		return scene.Element{}, nil
	}}); !errors.Is(err, grid.ErrInvalidDepth) {
		t.Fatalf("screen configuration = %v", err)
	}
	if _, err := program.NewInline(program.InlineConfig{Config: cfg, Build: func(*program.InlineRuntime) (scene.Element, error) {
		t.Fatal("invalid configuration invoked the builder")
		return scene.Element{}, nil
	}}); !errors.Is(err, grid.ErrInvalidDepth) {
		t.Fatalf("inline configuration = %v", err)
	}
}

func TestInvalidHostColorCannotEnterTheEnvironment(t *testing.T) {
	for _, override := range []grid.Depth{grid.Auto, grid.TrueColor} {
		writer := term.NewWriter(term.WriterConfig{Destination: io.Discard})
		t.Cleanup(func() { _ = writer.Close() })
		host := &partialHost{
			minimalHost: &minimalHost{events: make(chan input.Event), writer: writer, w: 2, h: 1},
			colorAnswer: grid.Depth(math.MaxUint8),
		}
		err := runFixture(t.Context(), program.ScreenConfig{Host: host, Color: override, Build: func(*program.Runtime) (scene.Element, error) {
			t.Fatal("invalid host color invoked the builder")
			return scene.Element{}, nil
		}})
		if !errors.Is(err, grid.ErrInvalidDepth) || writer.Queued() != 0 {
			t.Errorf("override=%d: frames=%d err=%v", override, writer.Queued(), err)
		}
	}
}
