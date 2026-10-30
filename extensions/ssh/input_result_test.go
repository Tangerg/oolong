package ssh_test

import (
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"

	charmssh "charm.land/ssh"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
)

type inputResultSession struct {
	*fakeSession
	result error
}

func (s inputResultSession) Read(p []byte) (int, error) {
	return copy(p, "a\x1b"), s.result
}

type inputResultComponent struct{ keys []input.Key }

func (*inputResultComponent) Draw(grid.View) {}

func (c *inputResultComponent) Handle(event input.Event) bool {
	if key, ok := event.(input.Key); ok {
		c.keys = append(c.keys, key)
		return true
	}
	return false
}

func TestRunNormalizesOnlyBareInputEOF(t *testing.T) {
	cause := errors.New("session read failed")
	wrapped := fmt.Errorf("session: %w", io.EOF)
	joined := errors.Join(io.EOF, cause)
	for _, test := range []struct {
		name   string
		result error
		want   error
	}{
		{name: "bare", result: io.EOF},
		{name: "wrapped", result: wrapped, want: wrapped},
		{name: "joined", result: joined, want: joined},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := inputResultSession{
				fakeSession: &fakeSession{
					ctx: newFakeContext(t.Context()), ptyOK: true,
					window: charmssh.Window{Width: 80, Height: 24},
				},
				result: test.result,
			}
			component := &inputResultComponent{}
			err := runSSHFixture(session, program.ScreenConfig{Build: func(*program.Runtime) (scene.Element, error) { return fixtureElement(component), nil }})
			if !errors.Is(err, test.want) {
				t.Errorf("Run = %v, want %v", err, test.want)
			}
			if len(component.keys) != 2 || !component.keys[0].IsRune('a', 0) || component.keys[1].Code != input.Esc {
				t.Fatalf("accepted input was lost before the terminal result: %v", component.keys)
			}
		})
	}
}
