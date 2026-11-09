package program_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
)

func TestSessionRejectsUnknownProgressBeforeCallingCustomHost(t *testing.T) {
	h := &partialHost{minimalHost: &minimalHost{
		events: make(chan input.Event),
		writer: term.NewWriter(term.WriterConfig{Destination: &frames{}}),
		w:      20,
		h:      4,
	}}
	defer func() { _ = h.writer.Close() }()
	accepted := term.Progress{State: term.ProgressNormal, Percent: 42}
	err := runFixture(t.Context(), program.ScreenConfig{
		Host: h, Build: func(runtime *program.Runtime) (scene.Element, error) {
			for _, test := range []struct {
				progress term.Progress
				want     error
				calls    int
			}{
				{term.Progress{State: term.ProgressState(255)}, term.ErrInvalidProgressState, 0},
				{accepted, nil, 1},
				{term.Progress{State: term.ProgressWarning + 1}, term.ErrInvalidProgressState, 1},
			} {
				if setErr := runtime.Session().SetProgress(test.progress); !errors.Is(setErr, test.want) {
					t.Errorf("progress %+v: %v, want %v", test.progress, setErr, test.want)
				}
				if len(h.progress) != test.calls {
					t.Errorf("progress %+v reached the host: calls=%d, want %d", test.progress, len(h.progress), test.calls)
				}
			}
			runtime.Quit()
			return fixtureElement(&component{text: "ready", runtime: runtime}), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(h.progress, []term.Progress{accepted}) {
		t.Fatalf("accepted progress = %+v, want only %+v", h.progress, accepted)
	}
}
