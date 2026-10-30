package uitest_test

import (
	"context"
	"errors"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// Runtime scenarios use an unstyled custom scene leaf, independent of base/kit.
type fixtureView interface {
	Draw(view grid.View)
	Handle(event input.Event) bool
}

func fixtureElement(component fixtureView) scene.Element {
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
