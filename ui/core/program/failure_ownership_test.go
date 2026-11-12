package program_test

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
)

type failingFrameOutput struct{ failure atomic.Pointer[error] }

func (w *failingFrameOutput) Write(data []byte) (int, error) {
	if failure := w.failure.Load(); failure != nil {
		return 0, *failure
	}
	return len(data), nil
}

func TestBuilderFailureRetainsPreviouslyObservedWriterCause(t *testing.T) {
	writerFailure := errors.New("transport failed before construction")
	buildFailure := errors.New("builder failed afterward")
	destination := &failingFrameOutput{}
	destination.failure.Store(&writerFailure)
	writer := term.NewWriter(term.WriterConfig{Destination: destination})
	t.Cleanup(func() { _ = writer.Close() })
	if _, err := writer.Queue([]byte("prior output")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Drain(term.DrainGrace); err != nil {
		t.Fatal(err)
	}
	h := newHost(t)
	h.writer = writer
	err := runFixture(t.Context(), program.InlineConfig{Host: h, Build: func(rt *program.InlineRuntime) (scene.Element, error) {
		if err := publish(rt, nil); !errors.Is(err, writerFailure) {
			t.Errorf("publication = %v, want writer failure", err)
		}
		return scene.Element{}, buildFailure
	}})
	for _, cause := range []error{writerFailure, buildFailure} {
		if !errors.Is(err, cause) {
			t.Errorf("Run = %v, lost %v", err, cause)
		}
	}
}

func TestLaterWriterFailureCannotReplaceTheFatalFrameCause(t *testing.T) {
	frameFailure := errors.New("image erase failed")
	writerFailure := errors.New("transport failed afterward")
	destination := &failingFrameOutput{}
	writer := term.NewWriter(term.WriterConfig{Destination: destination})
	t.Cleanup(func() { _ = writer.Close() })
	h := newHost(t)
	h.writer = writer
	painter := &handoverPainter{cause: frameFailure}
	err := runFixture(t.Context(), program.InlineConfig{Host: h, Build: func(rt *program.InlineRuntime) (scene.Element, error) {
		generation := uint64(1)
		if err := rt.Dispatcher().Post(func() {
			generation++
			if err := rt.Session().Hand(func() error {
				t.Error("failed frame transferred the display")
				return nil
			}); !errors.Is(err, frameFailure) {
				t.Errorf("Hand = %v, want frame failure", err)
			}
			destination.failure.Store(&writerFailure)
			if _, err := writer.Queue([]byte("independent output")); err != nil {
				t.Error(err)
			}
			if err := writer.Drain(term.DrainGrace); err != nil {
				t.Error(err)
			}
			for range 2 {
				if err := publish(rt, nil); !errors.Is(err, writerFailure) {
					t.Errorf("publication = %v, want writer failure", err)
				}
			}
			rt.Quit()
		}); err != nil {
			t.Fatal(err)
		}
		return scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
			frame.Paint(grid.Area(0, 0, 2, 1), generation, painter)
		}}), nil
	}})
	for _, cause := range []error{frameFailure, writerFailure} {
		if !errors.Is(err, cause) {
			t.Errorf("Run = %v, lost %v", err, cause)
		}
	}
	if painter.paintCalls != 1 || painter.eraseCalls != 1 {
		t.Errorf("failed display repeated callbacks: paint=%d, erase=%d", painter.paintCalls, painter.eraseCalls)
	}
}
