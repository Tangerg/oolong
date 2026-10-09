package program_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
)

type exclusiveHandoverHost struct {
	*host
	active bool
}

func (h *exclusiveHandoverHost) Hand(prepare, run func() error) error {
	if run == nil {
		return nil
	}
	if h.active {
		return term.ErrHandoverActive
	}
	h.active = true
	defer func() { h.active = false }()
	if prepare != nil {
		if err := prepare(); err != nil {
			return err
		}
	}
	return run()
}

func TestRejectedHandoverCannotPrepareAnotherOwnersDisplay(t *testing.T) {
	h := &exclusiveHandoverHost{host: newHost(t)}
	var root *component
	err := program.Run(t.Context(), program.Config{Host: h, Inline: func(r *program.InlineRuntime) program.Component {
		root = &component{text: "interface", runtime: r.Runtime}
		r.Dispatcher().Post(func() {
			err := r.Session().Hand(func() error {
				before := root.drawn.Load()
				called := false
				err := r.Session().Hand(func() error { called = true; return nil })
				if called || !errors.Is(err, term.ErrHandoverActive) {
					t.Errorf("nested handover was not refused: child=%v error=%v", called, err)
				}
				if root.drawn.Load() != before {
					t.Error("a refused handover drew the interface over the active child")
				}
				return nil
			})
			if err != nil {
				t.Error(err)
			}
			r.Quit()
		})
		return root
	}})
	if err != nil {
		t.Fatal(err)
	}
}
