package uitest

import (
	"errors"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func pointerRoot(t *testing.T) *scene.Root {
	t.Helper()
	root, err := scene.NewRoot(scene.RootConfig{Element: scene.NewElement(scene.ElementConfig{})})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(root.Close)
	candidate, err := root.Prepare(grid.NewSurface(1, 1).View())
	if err != nil {
		t.Fatal(err)
	}
	candidate.Commit(0)
	return root
}

func TestPointerSharesThePhysicalInputAndRepaintFIFO(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		host := New(t, Config{Width: 1, Height: 1})
		root := pointerRoot(t)
		pointers, release, err := host.BindPointerInput(root.CapturePresentedInput)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		if !host.Press(input.Enter) || !host.Pointer(input.Mouse{Action: input.MouseDown}) {
			t.Fatal("valid ordered input was refused")
		}
		done := make(chan error, 1)
		go func() {
			_, err := host.Repaint(t.Context())
			done <- err
		}()
		synctest.Wait()
		select {
		case <-pointers:
			t.Fatal("pointer intent passed an earlier physical event")
		case <-host.Repaints():
			t.Fatal("repaint passed earlier input")
		default:
		}
		if key, ok := (<-host.Events()).(input.Key); !ok || key.Code != input.Enter {
			t.Fatal("lost physical event")
		}
		captured := <-pointers
		if mouse, ok := captured.Input().(input.Mouse); !ok || mouse.Action != input.MouseDown || mouse.At.IsZero() {
			t.Fatalf("lost pointer intent: %+v", captured.Input())
		}
		reply := <-host.Repaints()
		reply(1, 1, nil)
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}

func TestPointerReleaseRevokesOnlyItsSession(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		host := New(t, Config{Width: 1, Height: 1})
		root := pointerRoot(t)
		pointers, release, err := host.BindPointerInput(root.CapturePresentedInput)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		if _, _, duplicateErr := host.BindPointerInput(root.CapturePresentedInput); duplicateErr == nil {
			t.Fatal("overlapping sessions shared one capture binding")
		}
		if !host.Press(input.Enter) || !host.Pointer(input.Mouse{Action: input.MouseDown}) || !host.Press(input.Tab) {
			t.Fatal("valid input was refused")
		}
		synctest.Wait()
		release()
		if host.Pointer(input.Mouse{Action: input.MouseDown}) {
			t.Fatal("released session accepted pointer intent")
		}
		next, releaseNext, err := host.BindPointerInput(root.CapturePresentedInput)
		if err != nil {
			t.Fatal(err)
		}
		defer releaseNext()
		release()
		if !host.Pointer(input.Mouse{Action: input.MouseUp}) {
			t.Fatal("old release revoked the next binding")
		}
		for _, want := range []input.Code{input.Enter, input.Tab} {
			if key, ok := (<-host.Events()).(input.Key); !ok || key.Code != want {
				t.Fatalf("release changed physical input order: %+v, want %v", key, want)
			}
		}
		if mouse, ok := (<-next).Input().(input.Mouse); !ok || mouse.Action != input.MouseUp {
			t.Fatal("revoked pointer intent entered the next binding")
		}
		select {
		case <-pointers:
			t.Fatal("released session delivered retained pointer intent")
		default:
		}
		if err := host.Close(); err != nil {
			t.Fatal(err)
		}
		if host.Pointer(input.Mouse{Action: input.MouseDown}) {
			t.Fatal("closed host retained pointer admission")
		}
		if _, _, err := host.BindPointerInput(root.CapturePresentedInput); !errors.Is(err, program.ErrStopped) {
			t.Fatalf("closed host binding = %v", err)
		}
	})
}
