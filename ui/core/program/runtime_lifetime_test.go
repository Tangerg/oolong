package program_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
)

type imageLifetimeHost struct {
	*host
	transmitted, released int
}

func TestEnvironmentKeepsTheFactsLearnedBeforeConstruction(t *testing.T) {
	h := newHost(t)
	want := grid.Ground{BG: grid.RGBColor(1, 2, 3)}
	h.ground = want
	var environment program.Environment
	var runtime *program.Runtime
	drawnAgainst := make(chan grid.Ground, 1)
	err := runFixture(context.Background(), program.ScreenConfig{Host: h, Build: func(r *program.Runtime) (scene.Element, error) {
		runtime = r
		environment = r.Environment()
		h.ground = grid.Ground{BG: grid.RGBColor(4, 5, 6)}
		if environment.Ground() != want || r.Environment().Ground() != want {
			t.Error("a host change replaced the startup environment during construction")
		}
		if err := r.Dispatcher().Post(r.Quit); err != nil {
			t.Error(err)
		}
		return fixtureElement(&drawer{runtime: r, seen: drawnAgainst}), nil
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
	for _, method := range []string{"Publish"} {
		t.Run(method, func(t *testing.T) {
			h := newHost(t)
			want := grid.Ground{FG: grid.RGBColor(210, 220, 230), BG: grid.RGBColor(20, 30, 40)}
			h.ground = want
			called := false
			err := runFixture(t.Context(), program.InlineConfig{Host: h, Build: func(r *program.InlineRuntime) (scene.Element, error) {
				h.ground = grid.Ground{}
				draw := func(view grid.View) {
					called = true
					if got := view.Ground(); got != want || got != r.Environment().Ground() {
						t.Errorf("construction publication ground = %+v, want startup %+v", got, want)
					}
					view.Text(0, 0, "startup", grid.Style{})
					view.Fade(view.Bounds(), 0.5)
				}
				err := publish(r, measurer{measure: func(int) int { return 1 }, draw: draw})
				if err != nil {
					t.Fatal(err)
				}
				r.Quit()
				return fixtureElement(&component{text: "live", runtime: r.Runtime}), nil
			}})
			if err != nil || !called {
				t.Fatalf("startup publication = %v, called=%v", err, called)
			}
		})
	}
}

type startupColorHost struct {
	*host
	depth grid.Depth
}

func (h *startupColorHost) Color() grid.Depth { return h.depth }

func TestInlineStartupPublicationUsesTheSelectedColorDepth(t *testing.T) {
	for _, test := range []struct {
		name  string
		color grid.Depth
		want  string
	}{
		{name: "host", color: grid.Auto, want: ";91"},
		{name: "override", color: grid.TrueColor, want: ";38;2;255;0;0"},
		{name: "disabled", color: grid.NoColor},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := &startupColorHost{host: newHost(t), depth: grid.Depth16}
			err := runFixture(t.Context(), program.InlineConfig{
				Host: h, Color: test.color,
				Build: func(r *program.InlineRuntime) (scene.Element, error) {
					h.depth = grid.NoColor
					if err := publish(r, measurer{
						measure: func(int) int { return 1 },
						draw: func(view grid.View) {
							view.Text(0, 0, "startup", grid.Style{FG: grid.RGBColor(255, 0, 0)})
						},
					}); err != nil {
						return scene.Element{}, err
					}
					r.Quit()
					return fixtureElement(&component{text: "live", runtime: r.Runtime}), nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			output := h.frames.String()
			if !strings.Contains(output, "startup") || !strings.Contains(output, test.want) {
				t.Fatalf("startup output=%q, want color %q", output, test.want)
			}
			if test.color == grid.NoColor && (strings.Contains(output, ";38;") || strings.Contains(output, ";91")) {
				t.Fatalf("disabled startup color reached the writer: %q", output)
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
			err := runFixture(context.Background(), program.InlineConfig{Host: h, Build: func(r *program.InlineRuntime) (scene.Element, error) {
				runtime, clipboard, images, session = r, r.Clipboard(), r.Images(), r.Session()
				if buildFails {
					return fixtureElement(nil), nil
				}
				r.Quit()
				return fixtureElement(&component{text: "finished", runtime: r.Runtime}), nil
			}})
			if (err != nil) != buildFails {
				t.Fatal(err)
			}
			called := false
			if err := session.Hand(func() error { called = true; return nil }); called || !errors.Is(err, errors.ErrUnsupported) {
				t.Errorf("ended session acquired display: child=%v error=%v", called, err)
			}
			if err := session.SetTitle("stale"); err == nil {
				t.Error("refused operation reported success")
			}
			if err := session.SetProgress(term.Progress{State: term.ProgressNormal, Percent: 10}); err == nil {
				t.Error("refused operation reported success")
			}
			if err := session.Bell(); err == nil {
				t.Error("refused operation reported success")
			}
			if err := session.Notify("stale"); err == nil {
				t.Error("refused operation reported success")
			}
			if clipboard.Copy("stale") || clipboard.Paste() || runtime.Clipboard().Copy("late") {
				t.Error("an ended runtime changed its externally owned host clipboard")
			}
			if _, err := images.Transmit(nil); !errors.Is(err, errors.ErrUnsupported) {
				t.Errorf("ended image transmission = %v", err)
			}
			if err := images.Release(graphics.Image{ID: 1}); !errors.Is(err, errors.ErrUnsupported) {
				t.Errorf("ended image release = %v", err)
			}
			if err := publish(runtime, measurer{
				measure: func(int) int { t.Error("ended runtime measured content"); return 1 },
				draw:    func(grid.View) { t.Error("ended runtime drew content") },
			}); !errors.Is(err, program.ErrStopped) {
				t.Errorf("an ended inline runtime accepted publication: %v", err)
			}
			if title, rang, notifications := h.said(); title != "" || rang != 0 || len(notifications) != 0 ||
				h.progress != (term.Progress{}) || h.transmitted != 0 || h.released != 0 || h.timesHanded() != 0 {
				t.Fatal("a capability outlived the program that authorized its host operations")
			}
		})
	}
}
