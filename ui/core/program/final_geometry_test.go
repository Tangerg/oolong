package program_test

import (
	"context"
	"errors"
	"image"
	"strconv"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestFinalPresentationReadsTheCanvasAfterTaskAdmissionStops(t *testing.T) {
	for _, ending := range []string{"quit", "cancel", "input EOF"} {
		t.Run(ending, func(t *testing.T) {
			host := newHost(t)
			want := image.Pt(host.w, host.h)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var retained *program.Runtime
			checks, paints := 0, 0
			checkSize := func(stage string) {
				t.Helper()
				width, height := retained.Size()
				if got := image.Pt(width, height); got != want {
					t.Errorf("%s size=%v, want %v", stage, got, want)
				}
			}
			inline, err := program.NewInline(program.InlineConfig{
				Host: host,
				Build: func(runtime *program.InlineRuntime) (scene.Element, error) {
					retained = runtime.Runtime
					if err := runtime.SetFramePreparation(func(width, height int) (bool, error) {
						checks++
						checkSize("preparation")
						if image.Pt(width, height) != want {
							t.Errorf("preparation arguments=%dx%d", width, height)
						}
						return true, nil
					}); err != nil {
						return scene.Element{}, err
					}
					switch ending {
					case "quit":
						if err := runtime.Dispatcher().Post(runtime.Quit); err != nil {
							return scene.Element{}, err
						}
					case "cancel":
						cancel()
					case "input EOF":
						close(host.events)
					}
					return scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
						paints++
						checkSize("paint")
						width, height := runtime.Size()
						frame.Text(0, 0, strconv.Itoa(width)+"x"+strconv.Itoa(height), grid.Style{})
						if paints == 2 {
							postErr := runtime.Dispatcher().Post(func() { t.Error("final paint reopened task admission") })
							if !errors.Is(postErr, program.ErrStopped) {
								t.Errorf("final painting task admission=%v", postErr)
							}
						}
					}}), nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := inline.Run(ctx); err != nil {
				t.Fatal(err)
			}
			if checks != 2 || paints != 2 {
				t.Fatalf("preparations=%d paints=%d, want opening and final presentation", checks, paints)
			}
			if width, height := retained.Size(); width != 0 || height != 0 {
				t.Fatalf("ended runtime retained canvas size %dx%d", width, height)
			}
		})
	}
}
