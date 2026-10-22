package program_test

import (
	"errors"
	"image"
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
)

func TestKnownWriterFailureRefusesNewInlineContent(t *testing.T) {
	for _, method := range []string{"Print", "Append"} {
		t.Run(method, func(t *testing.T) {
			cause := errors.New("destination failed")
			writer := &protocolWriter{changes: make(chan struct{}), err: cause}
			host := &protocolHost{events: make(chan input.Event), writer: writer}
			err := program.Run(t.Context(), program.Config{Host: host, Inline: func(r *program.InlineRuntime) program.Component {
				var refusal error
				if method == "Append" {
					refusal = r.Append(func(grid.View) bool { t.Error("refused append was drawn"); return false })
				} else {
					refusal = r.Print(measurer{
						measure: func(int) int { t.Error("refused output was measured"); return 1 },
						draw:    func(grid.View) { t.Error("refused output was drawn") },
					})
				}
				if !errors.Is(refusal, cause) {
					t.Errorf("publication refusal = %v", refusal)
				}
				r.Quit()
				return &component{text: "live", runtime: r.Runtime}
			}})
			if !errors.Is(err, cause) || writer.queued.Load() != 0 {
				t.Fatalf("failed publication lost its outcome: Run=%v queued=%d", err, writer.queued.Load())
			}
		})
	}
}

func TestInlineAppendRefusesAllRowsWhenALaterRowIsUnsupported(t *testing.T) {
	host := newHost(t)
	calls := 0
	err := program.Run(t.Context(), program.Config{Host: host, Inline: func(r *program.InlineRuntime) program.Component {
		refusal := r.Append(func(view grid.View) bool {
			calls++
			if calls == 1 {
				view.Text(0, 0, "partial publication", grid.Style{})
				return true
			}
			view.Paint(grid.Area(0, 0, 1, 1), 1, admissionPainter{})
			return false
		})
		if !errors.Is(refusal, errors.ErrUnsupported) {
			t.Errorf("append refusal = %v", refusal)
		}
		r.Quit()
		return &component{text: "live", runtime: r.Runtime}
	}})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || strings.Contains(host.frames.String(), "partial publication") {
		t.Fatalf("refused append leaked an earlier row: calls=%d output=%q", calls, host.frames.String())
	}
}

type admissionPainter struct{}

func (admissionPainter) Paint(io.Writer, image.Point) error { return nil }
func (admissionPainter) Erase(io.Writer) error              { return nil }

func TestInlinePublicationRequiresAnInlineProgram(t *testing.T) {
	err := program.Run(t.Context(), program.Config{Host: newHost(t), Root: func(r *program.Runtime) program.Component {
		inline := &program.InlineRuntime{Runtime: r}
		if err := inline.Print(nil); !errors.Is(err, errors.ErrUnsupported) {
			t.Errorf("non-inline Print = %v", err)
		}
		if err := inline.Append(nil); !errors.Is(err, errors.ErrUnsupported) {
			t.Errorf("non-inline Append = %v", err)
		}
		r.Quit()
		return &component{text: "screen", runtime: r}
	}})
	if err != nil {
		t.Fatal(err)
	}
}
