package program_test

import (
	"errors"
	"image"
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestKnownWriterFailureRefusesNewInlineContent(t *testing.T) {
	cause := errors.New("destination failed")
	writer := &protocolWriter{changes: make(chan struct{}), err: cause}
	host := &protocolHost{events: make(chan input.Event), writer: writer}
	err := runFixture(t.Context(), program.InlineConfig{Host: host, Build: func(r *program.InlineRuntime) (scene.Element, error) {
		refusal := publish(r, measurer{measure: func(int) int { t.Error("refused content measured"); return 1 }, draw: func(grid.View) { t.Error("refused content painted") }})
		if !errors.Is(refusal, cause) {
			t.Errorf("refusal = %v", refusal)
		}
		r.Quit()
		return fixtureElement(&component{text: "live", runtime: r.Runtime}), nil
	}})
	if !errors.Is(err, cause) || writer.queued.Load() != 0 {
		t.Fatalf("Run=%v queued=%d", err, writer.queued.Load())
	}
}

func TestInlineCaptureRefusesAllRowsWhenALaterRowIsUnsupported(t *testing.T) {
	host := newHost(t)
	err := runFixture(t.Context(), program.InlineConfig{Host: host, Build: func(r *program.InlineRuntime) (scene.Element, error) {
		refusal := publish(r, measurer{measure: func(int) int { return 2 }, draw: func(view grid.View) {
			view.Text(0, 0, "partial publication", grid.Style{})
			view.Paint(grid.Area(0, 1, 1, 1), 1, admissionPainter{})
		}})
		if !errors.Is(refusal, errors.ErrUnsupported) {
			t.Errorf("capture refusal=%v", refusal)
		}
		r.Quit()
		return fixtureElement(&component{text: "live", runtime: r.Runtime}), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(host.frames.String(), "partial publication") {
		t.Fatal("refused content leaked into output")
	}
}

type admissionPainter struct{}

func (admissionPainter) Paint(io.Writer, image.Point) error { return nil }
func (admissionPainter) Erase(io.Writer) error              { return nil }

func TestInlinePublicationRequiresAnInlineProgram(t *testing.T) {
	err := runFixture(t.Context(), program.ScreenConfig{Host: newHost(t), Build: func(r *program.Runtime) (scene.Element, error) {
		inline := &program.InlineRuntime{Runtime: r}
		if err := publish(inline, nil); !errors.Is(err, errors.ErrUnsupported) {
			t.Errorf("non-inline publication=%v", err)
		}
		r.Quit()
		return fixtureElement(&component{text: "screen", runtime: r}), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
}
