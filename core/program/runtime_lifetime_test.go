package program_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/core/graphics"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
)

type imageLifetimeHost struct {
	*host
	transmitted, released int
}

type lifetimeOutput struct{ drawn *bool }

func (lifetimeOutput) HeightForWidth(int) int { return 1 }
func (o lifetimeOutput) Draw(grid.View)       { *o.drawn = true }

func TestEnvironmentKeepsTheFactsLearnedBeforeConstruction(t *testing.T) {
	h := newHost(t)
	want := grid.Ground{BG: grid.RGBColor(1, 2, 3)}
	h.ground = want
	var environment program.Environment
	var runtime *program.Runtime
	drawnAgainst := make(chan grid.Ground, 1)
	err := program.Run(context.Background(), program.Config{Host: h, Root: func(r *program.Runtime) program.Component {
		runtime = r
		environment = r.Environment()
		h.ground = grid.Ground{BG: grid.RGBColor(4, 5, 6)}
		if environment.Ground() != want || r.Environment().Ground() != want {
			t.Error("a host change replaced the startup environment during construction")
		}
		r.Dispatcher().Post(r.Quit)
		return &drawer{runtime: r, seen: drawnAgainst}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if environment.Ground() != want || runtime.Environment().Ground() != want {
		t.Fatal("a retained environment queries the host instead of retaining learned facts")
	}
	select {
	case got := <-drawnAgainst:
		if got != want {
			t.Fatal("canvas ground diverged from the startup environment")
		}
	default:
		t.Fatal("no opening frame drawn")
	}
}

func TestInlineConstructionPublishesWithTheStartupEnvironment(t *testing.T) {
	for _, method := range []string{"Print", "Append"} {
		t.Run(method, func(t *testing.T) {
			h := newHost(t)
			want := grid.Ground{FG: grid.RGBColor(210, 220, 230), BG: grid.RGBColor(20, 30, 40)}
			h.ground = want
			called := false
			err := program.Run(t.Context(), program.Config{Host: h, Inline: func(r *program.InlineRuntime) program.Component {
				h.ground = grid.Ground{}
				draw := func(view grid.View) {
					called = true
					if got := view.Ground(); got != want || got != r.Environment().Ground() {
						t.Errorf("construction publication ground = %+v, want startup %+v", got, want)
					}
					view.Text(0, 0, "startup", grid.Style{})
					view.Fade(view.Bounds(), 0.5)
				}
				var err error
				if method == "Print" {
					err = r.Print(measurer{measure: func(int) int { return 1 }, draw: draw})
				} else {
					err = r.Append(func(view grid.View) bool { draw(view); return false })
				}
				if err != nil {
					t.Fatal(err)
				}
				r.Quit()
				return &component{text: "live", runtime: r.Runtime}
			}})
			if err != nil || !called {
				t.Fatalf("startup publication = %v, called=%v", err, called)
			}
		})
	}
}

func (h *imageLifetimeHost) Transmit([]byte) (graphics.Image, error) {
	h.transmitted++
	return graphics.Image{ID: 1}, nil
}

func (h *imageLifetimeHost) ReleaseImage(graphics.Image) error { h.released++; return nil }

func TestEndedRuntimeRevokesRetainedCapabilities(t *testing.T) {
	for _, buildFails := range []bool{false, true} {
		name := "Run returned"
		if buildFails {
			name = "builder failed"
		}
		t.Run(name, func(t *testing.T) {
			h := &imageLifetimeHost{host: newHost(t)}
			var runtime *program.InlineRuntime
			var clipboard program.Clipboard
			var images program.Images
			var session program.Session
			err := program.Run(context.Background(), program.Config{Host: h, Inline: func(r *program.InlineRuntime) program.Component {
				runtime, clipboard, images, session = r, r.Clipboard(), r.Images(), r.Session()
				if buildFails {
					return nil
				}
				r.Quit()
				return &component{text: "finished", runtime: r.Runtime}
			}})
			if (err != nil) != buildFails {
				t.Fatal(err)
			}
			called := false
			if err := session.Hand(func() error { called = true; return nil }); called || !errors.Is(err, errors.ErrUnsupported) {
				t.Errorf("ended session acquired display: child=%v error=%v", called, err)
			}
			session.SetTitle("stale")
			session.SetProgress(term.Progress{State: term.ProgressNormal, Percent: 10})
			session.Bell()
			session.Notify("stale")
			if clipboard.Copy("stale") || clipboard.Paste() || runtime.Clipboard().Copy("late") {
				t.Error("an ended runtime changed its externally owned host clipboard")
			}
			if _, err := images.Transmit(nil); !errors.Is(err, errors.ErrUnsupported) {
				t.Errorf("ended image transmission = %v", err)
			}
			if err := images.Release(graphics.Image{ID: 1}); !errors.Is(err, errors.ErrUnsupported) {
				t.Errorf("ended image release = %v", err)
			}
			drawn := false
			if err := runtime.Print(lifetimeOutput{drawn: &drawn}); drawn || !errors.Is(err, program.ErrStopped) {
				t.Errorf("an ended inline runtime accepted publication: drawn=%v error=%v", drawn, err)
			}
			if title, rang, notifications := h.said(); title != "" || rang != 0 || len(notifications) != 0 ||
				h.progress != (term.Progress{}) || h.transmitted != 0 || h.released != 0 || h.timesHanded() != 0 {
				t.Fatal("a capability outlived the program that authorized its host operations")
			}
		})
	}
}
