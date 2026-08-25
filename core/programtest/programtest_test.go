package programtest_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/programtest"
)

type component struct {
	runtime *program.Runtime
	text    string
}

type styledComponent struct{ split bool }

func (c styledComponent) Draw(view grid.View) {
	red := grid.Style{FG: grid.RGBColor(255, 0, 0)}
	green := grid.Style{FG: grid.RGBColor(0, 255, 0)}
	view.Text(0, 0, "hello ", red)
	if c.split {
		view.Text(0, 1, "world", green)
		return
	}
	view.Text(6, 0, "world", green)
}

func (styledComponent) Handle(input.Event) bool { return false }

// editable answers the keys Type cannot spell as well as the ones it can, so a test
// can drive both halves of a keyboard through one component.
type editable struct {
	runtime *program.Runtime
	text    string
}

func (e *editable) Draw(view grid.View) { view.Text(0, 0, e.text, grid.Style{}) }

func (e *editable) Handle(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	switch {
	case key.Code == input.Backspace:
		e.text = e.text[:max(len(e.text)-1, 0)]
	case key.Rune == 'q':
		e.runtime.Quit()
	default:
		e.text += string(key.Rune)
	}
	return true
}

func (c *component) Draw(view grid.View) { view.Text(0, 0, c.text, grid.Style{}) }

func (c *component) Handle(event input.Event) bool {
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	if key.Rune == 'q' {
		c.runtime.Quit()
		return true
	}
	c.text += string(key.Rune)
	return true
}

func TestHostDrivesAProgramWithoutATerminal(t *testing.T) {
	host := programtest.New(t, programtest.Config{Width: 30, Height: 4})
	done := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: host,
			Root: func(runtime *program.Runtime) program.Component {
				return &component{runtime: runtime, text: "ready"}
			},
		})
	}()

	host.Shows(t, "ready")
	host.Type("!")
	host.Shows(t, "ready!")
	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("program ended with %v", err)
	}
}

func TestHostDoesNotInventOptionalCapabilities(t *testing.T) {
	host := programtest.New(t, programtest.Config{Width: 20, Height: 3})
	if _, ok := any(host).(program.GroundHost); ok {
		t.Fatal("test host implements GroundHost; capability absence cannot be tested")
	}
	if _, ok := any(host).(program.CopyHost); ok {
		t.Fatal("test host implements CopyHost; capability absence cannot be tested")
	}
	if _, ok := any(host).(program.NotifyHost); ok {
		t.Fatal("test host implements NotifyHost; capability absence cannot be tested")
	}
}

func TestTextAssertionsIgnoreAppearanceButNotGeometryBoundaries(t *testing.T) {
	for _, test := range []struct {
		name  string
		split bool
	}{
		{name: "appearance is transparent"},
		{name: "geometry remains a boundary", split: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			host := programtest.New(t, programtest.Config{Width: 20, Height: 3})
			go func() {
				_ = program.Run(t.Context(), program.Config{
					Host: host,
					Root: func(*program.Runtime) program.Component {
						return styledComponent{split: test.split}
					},
				})
			}()
			host.Shows(t, "hello")
			if test.split {
				host.Hides(t, "hello world")
				return
			}
			host.Shows(t, "hello world")
		})
	}
}

func TestEventsCanBeQueuedBeforeTheProgramStarts(t *testing.T) {
	host := programtest.New(t, programtest.Config{Width: 20, Height: 3})
	for range 1_000 {
		if !host.Send(input.Key{Code: input.Down}) {
			t.Fatal("open host refused an event")
		}
	}
}

func TestResizeUsesTheProgramSurfaceBound(t *testing.T) {
	host := programtest.New(t, programtest.Config{Width: 20, Height: 3})
	for _, size := range []struct{ width, height int }{
		{0, 3}, {-1, 3}, {program.MaxCells, 2},
	} {
		if host.Resize(size.width, size.height) {
			t.Errorf("Resize(%d, %d) accepted invalid geometry", size.width, size.height)
		}
	}
	if !host.Resize(40, 6) {
		t.Fatal("Resize rejected valid geometry")
	}
}

// TestPressDeliversKeysTypeCannotSpell. Type is for text, and there is no character
// that means Backspace or Down; a test that could only send runes could exercise a
// widget's typing and none of its navigation.
func TestPressDeliversKeysTypeCannotSpell(t *testing.T) {
	host := programtest.New(t, programtest.Config{Width: 20, Height: 3})
	done := make(chan error, 1)
	go func() {
		done <- program.Run(t.Context(), program.Config{
			Host: host,
			Root: func(runtime *program.Runtime) program.Component {
				return &editable{runtime: runtime, text: "ready"}
			},
		})
	}()

	host.Shows(t, "ready")
	if !host.Press(input.Backspace) {
		t.Fatal("open host refused a key press")
	}
	// Hides rather than Shows("read"): every prefix of the old text is still in the
	// old text, so only the disappearance of the last character says the key landed.
	host.Hides(t, "ready")
	host.Type("q")
	if err := <-done; err != nil {
		t.Fatalf("program ended with %v", err)
	}
}

// TestFramesKeepEveryWriteWhileFrameKeepsTheLast separates the two output questions.
// Frame answers "what does the screen hold now", which is one diff; Frames answers
// "what did the program send, in order", which is the only way to assert that
// something appeared and then went away rather than never having been drawn.
func TestFramesKeepEveryWriteWhileFrameKeepsTheLast(t *testing.T) {
	host := programtest.New(t, programtest.Config{Width: 20, Height: 3})
	go func() {
		_ = program.Run(t.Context(), program.Config{
			Host: host,
			Root: func(runtime *program.Runtime) program.Component {
				return &editable{runtime: runtime, text: "alpha"}
			},
		})
	}()

	host.Shows(t, "alpha")
	for range len("alpha") {
		host.Press(input.Backspace)
	}
	host.Type("beta")
	host.Shows(t, "beta")

	if frames := host.Frames(); !strings.Contains(frames, "alpha") {
		t.Errorf("Frames lost an earlier write: %q", frames)
	}
	if frame := host.Frame(); strings.Contains(frame, "alpha") {
		t.Errorf("Frame kept a superseded write: %q", frame)
	}
}
