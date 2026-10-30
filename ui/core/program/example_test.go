package program_test

import (
	"context"
	"fmt"
	"time"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// NewScreen validates its configuration without acquiring a terminal. Build runs
// once, after Run has a host, and all changes then use the same UI owner.
func ExampleNewScreen() {
	_, err := program.NewScreen(program.ScreenConfig{Build: func(runtime *program.Runtime) (scene.Element, error) {
		now := time.Now()
		scope, err := runtime.NewScope(program.ScopeConfig{})
		if err != nil {
			return scene.Element{}, err
		}
		if _, err := runtime.Every(scope, time.Second, func() { now = time.Now() }); err != nil {
			return scene.Element{}, err
		}
		return scene.NewElement(scene.ElementConfig{
			Paint: func(frame scene.Frame) { frame.Text(0, 0, now.Format(time.TimeOnly), grid.Style{}) },
			Handle: func(event scene.Event) scene.EventResult {
				if key, ok := event.Input.(input.Key); ok && key.Down() && key.Rune == 'q' {
					runtime.Quit()
					return scene.EventResult{Consumed: true}
				}
				return scene.EventResult{}
			},
		}), nil
	}})
	fmt.Println(err)
	// Output: <nil>
}

func ExampleNewInline() {
	_, err := program.NewInline(program.InlineConfig{Build: func(_ *program.InlineRuntime) (scene.Element, error) {
		return scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { frame.Text(0, 0, "Working", grid.Style{}) }}), nil
	}})
	fmt.Println(err)
	// Output: <nil>
}

func ExampleRuntime_Start() {
	type result struct{ count int }
	load := func(runtime *program.Runtime, scope *program.Scope, apply func(result, error)) (*program.Task, error) {
		return runtime.Start(scope, program.Work[result]{
			Run: func(ctx context.Context) (result, error) {
				if err := ctx.Err(); err != nil {
					return result{}, context.Cause(ctx)
				}
				return result{count: 42}, nil
			},
			Apply: apply,
		})
	}
	_ = load
	fmt.Println("worker execution and UI delivery have separate lifetimes")
	// Output: worker execution and UI delivery have separate lifetimes
}
