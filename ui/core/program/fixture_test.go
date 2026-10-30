package program_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// Runtime scenarios use an unstyled custom scene leaf, independent of base/kit.
func fixtureElement(component interface {
	Draw(view grid.View)
	Handle(event input.Event) bool
},
) scene.Element {
	if component == nil {
		return scene.Element{}
	}
	return scene.NewElement(scene.ElementConfig{
		Paint: func(frame scene.Frame) { component.Draw(frame.View) },
		Handle: func(event scene.Event) scene.EventResult {
			return scene.EventResult{Consumed: component.Handle(event.Input)}
		},
	})
}

func runFixture(ctx context.Context, config any) error {
	switch cfg := config.(type) {
	case program.ScreenConfig:
		screen, err := program.NewScreen(cfg)
		if err != nil {
			return err
		}
		return screen.Run(ctx)
	case program.InlineConfig:
		inline, err := program.NewInline(cfg)
		if err != nil {
			return err
		}
		return inline.Run(ctx)
	default:
		return errors.New("test: unsupported session fixture")
	}
}

func publish(runtime *program.InlineRuntime, body grid.Drawable) error {
	_, err := runtime.Publish(body)
	return err
}

func after(t *testing.T, runtime *program.Runtime, delay time.Duration, fn func()) func() {
	t.Helper()
	scope, err := runtime.NewScope(program.ScopeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	timer, err := runtime.After(scope, delay, func() { defer scope.Close(); fn() })
	if err != nil {
		scope.Close()
		t.Fatal(err)
	}
	return func() { timer.Stop(); scope.Close() }
}

func every(t *testing.T, runtime *program.Runtime, interval time.Duration, fn func()) func() {
	t.Helper()
	scope, err := runtime.NewScope(program.ScopeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	timer, err := runtime.Every(scope, interval, fn)
	if err != nil {
		scope.Close()
		t.Fatal(err)
	}
	return func() { timer.Stop(); scope.Close() }
}
