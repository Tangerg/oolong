package program_test

import (
	"context"
	"errors"
	"image"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
)

func TestFramePreparationKeepsTheOwnerResponsiveUntilTaskDelivery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		host := newHost(t)
		host.w, host.h = 20, 4
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		release := make(chan struct{})
		ownerReady := make(chan program.Dispatcher, 1)
		finished := make(chan error, 1)
		var runtime *program.Runtime
		var prepared bool
		var caption string
		var checkedSize image.Point
		screen, launchErr := program.NewScreen(program.ScreenConfig{
			Host: host, FrameInterval: time.Millisecond,
			Build: func(owner *program.Runtime) (scene.Element, error) {
				runtime = owner
				if err := owner.SetFramePreparation(func(width, height int) (bool, error) {
					checkedSize = image.Pt(width, height)
					return prepared, nil
				}); err != nil {
					return scene.Element{}, err
				}
				scope, scopeErr := owner.NewScope(program.ScopeConfig{})
				if scopeErr != nil {
					return scene.Element{}, scopeErr
				}
				_, startErr := owner.Start(scope, program.Work[string]{
					Run: func(ctx context.Context) (string, error) {
						select {
						case <-release:
							return "prepared", nil
						case <-ctx.Done():
							return "", context.Cause(ctx)
						}
					},
					Apply: func(value string, err error) {
						defer scope.Close()
						if err != nil {
							t.Error(err)
							owner.Quit()
							return
						}
						caption, prepared = value, true
						owner.Refresh()
					},
				})
				if startErr != nil {
					return scene.Element{}, startErr
				}
				ownerReady <- owner.Dispatcher()
				return scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
					frame.Text(0, 0, caption, grid.Style{})
				}}), nil
			},
		})
		if launchErr != nil {
			t.Fatal(launchErr)
		}
		go func() { finished <- screen.Run(ctx) }()
		var dispatch program.Dispatcher
		select {
		case dispatch = <-ownerReady:
		case err := <-finished:
			t.Fatalf("session ended before building: %v", err)
		}
		synctest.Wait()
		if err := dispatch.Invoke(t.Context(), func() error {
			if width, height := runtime.Size(); width != 20 || height != 4 || checkedSize != image.Pt(width, height) {
				t.Errorf("initial size=%dx%d preparation=%v", width, height, checkedSize)
			}
			runtime.Refresh()
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		host.events <- input.Resize{Width: 30, Height: 5}
		synctest.Wait()
		if err := dispatch.Invoke(t.Context(), func() error {
			if width, height := runtime.Size(); width != 30 || height != 5 || checkedSize != image.Pt(width, height) {
				t.Errorf("resized size=%dx%d preparation=%v", width, height, checkedSize)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if output := host.frames.String(); output != "" {
			t.Fatalf("unprepared frame reached the host: %q", output)
		}
		close(release)
		synctest.Wait()
		if output := host.frames.String(); !strings.Contains(output, "prepared") {
			t.Fatalf("delivered preparation did not paint: %q", output)
		}
		if err := dispatch.Invoke(t.Context(), func() error {
			caption = "ungated!"
			return runtime.SetFramePreparation(func(int, int) (bool, error) { return false, nil })
		}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
		synctest.Wait()
		if output := host.frames.String(); strings.Contains(output, "ungated!") {
			t.Fatalf("replacement check allowed an unprepared frame: %q", output)
		}
		if err := dispatch.Invoke(t.Context(), func() error { return runtime.SetFramePreparation(nil) }); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if output := host.frames.String(); !strings.Contains(output, "ungated!") {
			t.Fatalf("removing preparation left its frame parked: %q", output)
		}
		cancel()
		if err := <-finished; err != nil {
			t.Fatal(err)
		}
		if width, height := runtime.Size(); width != 0 || height != 0 {
			t.Fatalf("stopped runtime retained size %dx%d", width, height)
		}
		if err := runtime.SetFramePreparation(nil); !errors.Is(err, program.ErrStopped) {
			t.Fatalf("stopped preparation = %v", err)
		}
	})
}

func TestFramePreparationFailureReturnsFromRunWithoutPainting(t *testing.T) {
	for _, ready := range []bool{false, true} {
		t.Run(strconv.FormatBool(ready), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				host := newHost(t)
				checks, paints := 0, 0
				screen, launchErr := program.NewScreen(program.ScreenConfig{
					Host: host,
					Build: func(runtime *program.Runtime) (scene.Element, error) {
						err := runtime.SetFramePreparation(func(int, int) (bool, error) {
							checks++
							return ready, term.ErrQueueFull
						})
						return scene.NewElement(scene.ElementConfig{Paint: func(scene.Frame) { paints++ }}), err
					},
				})
				if launchErr != nil {
					t.Fatal(launchErr)
				}
				ctx, cancel := context.WithTimeout(t.Context(), time.Second)
				defer cancel()
				if err := screen.Run(ctx); !errors.Is(err, term.ErrQueueFull) || checks != 1 || paints != 0 || host.frames.String() != "" {
					t.Fatalf("preparation checks=%d paints=%d output=%q error=%v", checks, paints, host.frames.String(), err)
				}
			})
		})
	}
}
