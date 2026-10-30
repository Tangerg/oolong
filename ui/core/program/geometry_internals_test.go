package program

import (
	"image"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func geometryOwner(t *testing.T) (*program, *int, *int) {
	t.Helper()
	presses, cancellations := 0, 0
	root, err := scene.NewRoot(scene.RootConfig{Element: scene.NewElement(scene.ElementConfig{
		Paint: func(frame scene.Frame) { frame.Text(0, 0, "target", grid.Style{}) },
		Handle: func(event scene.Event) scene.EventResult {
			if mouse, ok := event.Input.(input.Mouse); ok {
				switch mouse.Action {
				case input.MouseDown:
					presses++
					return scene.EventResult{Consumed: true, Capture: true}
				case input.MouseCancel:
					cancellations++
				default:
					return scene.EventResult{}
				}
			}
			return scene.EventResult{}
		},
	})})
	if err != nil {
		t.Fatal(err)
	}
	p := &program{root: root, canvas: grid.NewScreen(12, 3), writer: &admissionWriter{changes: make(chan struct{})}, tasks: newTaskQueue()}
	t.Cleanup(p.stop)
	if _, err := p.compose(true); err != nil {
		t.Fatal(err)
	}
	return p, &presses, &cancellations
}

func TestPhysicalGeometryChangesRevokeCaptureAndQueuedCoordinates(t *testing.T) {
	for _, event := range []input.Event{input.Resize{Width: 15, Height: 4}, input.FocusIn{}} {
		p, presses, cancellations := geometryOwner(t)
		mouse := input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 0)}
		if !p.root.Handle(mouse) || *presses != 1 {
			t.Fatal("initial written target did not capture")
		}
		old := p.root.CaptureInput(mouse)
		if err := p.handle(event); err != nil {
			t.Fatal(err)
		}
		if *cancellations != 1 {
			t.Fatalf("capture cancellation = %d", *cancellations)
		}
		if p.root.HandleInput(old) || p.root.Handle(mouse) || *presses != 1 {
			t.Fatal("uncertain physical geometry accepted an old coordinate")
		}
		if _, err := p.compose(true); err != nil {
			t.Fatal(err)
		}
		if !p.root.Handle(mouse) || *presses != 2 {
			t.Fatal("replacement written geometry did not restore hit testing")
		}
	}
}

type geometryHandover struct{}

func (geometryHandover) Hand(prepare, run func() error) error {
	if err := prepare(); err != nil {
		return err
	}
	return run()
}

func TestDisplayHandoverInvalidatesCoordinatesBeforeExternalOutput(t *testing.T) {
	p, presses, _ := geometryOwner(t)
	p.host.handover = geometryHandover{}
	mouse := input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(1, 0)}
	old := p.root.CaptureInput(mouse)
	err := (Session{runtime: &Runtime{p: p}}).Hand(func() error {
		if p.root.HandleInput(old) || p.root.Handle(mouse) {
			t.Fatal("external output reused the UI's old hit map")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.root.HandleInput(old) || *presses != 0 {
		t.Fatal("handover return revived old coordinates")
	}
	if _, err := p.compose(true); err != nil {
		t.Fatal(err)
	}
	if !p.root.Handle(mouse) || *presses != 1 {
		t.Fatal("repaint did not restore hit testing")
	}
}
