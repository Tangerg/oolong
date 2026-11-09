package term

import (
	"errors"
	"testing"
)

func TestUnknownProgressCannotChangeAcceptedState(t *testing.T) {
	for _, phase := range []string{"idle", "active", "paused", "pending clear", "resumed"} {
		t.Run(phase, func(t *testing.T) {
			p := newTaskProgress()
			var written []string
			queue := func(data []byte) (uint64, error) {
				written = append(written, string(data))
				return uint64(len(written)), nil
			}
			if phase != "idle" {
				if err := p.to(Progress{State: ProgressNormal, Percent: 42}, queue); err != nil {
					t.Fatal(err)
				}
			}
			switch phase {
			case "paused", "pending clear", "resumed":
				p.pause()
			}
			if phase == "pending clear" {
				if err := p.to(Progress{}, queue); err != nil {
					t.Fatal(err)
				}
			}
			if phase == "resumed" {
				if err := p.restore(queue); err != nil {
					t.Fatal(err)
				}
			}
			select {
			case <-p.wake:
			default:
			}
			current, published, paused, dirty := p.current, p.published, p.paused, p.dirty
			cleanup, writes := p.leave(), len(written)
			for _, state := range []ProgressState{ProgressWarning + 1, ProgressState(255)} {
				if err := p.to(Progress{State: state, Percent: 80}, queue); !errors.Is(err, ErrInvalidProgressState) {
					t.Errorf("unknown progress state %d: %v", state, err)
				}
				if p.current != current || p.published != published || p.paused != paused || p.dirty != dirty ||
					p.leave() != cleanup || len(written) != writes || len(p.wake) != 0 {
					t.Fatalf("invalid progress changed accepted state or output: current=%+v published=%t paused=%t dirty=%t cleanup=%q writes=%q wake=%d",
						p.current, p.published, p.paused, p.dirty, p.leave(), written[writes:], len(p.wake))
				}
			}
		})
	}
}
