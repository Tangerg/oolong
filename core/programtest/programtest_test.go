package programtest_test

import (
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
	host := programtest.New(t, 30, 4)
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
	host := programtest.New(t, 20, 3)
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

func TestEventsCanBeQueuedBeforeTheProgramStarts(t *testing.T) {
	host := programtest.New(t, 20, 3)
	for range 1_000 {
		if !host.Send(input.Key{Code: input.Down}) {
			t.Fatal("open host refused an event")
		}
	}
}
