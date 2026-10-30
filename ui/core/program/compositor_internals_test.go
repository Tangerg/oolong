package program

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
)

type admissionWriter struct {
	refuse  bool
	frames  [][]byte
	changes chan struct{}
}

func (w *admissionWriter) Queue(frame []byte) (uint64, error) {
	if w.refuse {
		return 0, term.ErrQueueFull
	}
	w.frames = append(w.frames, frame)
	return uint64(len(w.frames)), nil
}
func (w *admissionWriter) Changes() <-chan struct{} { return w.changes }
func (w *admissionWriter) Written() uint64          { return uint64(len(w.frames)) }
func (*admissionWriter) Err() error                 { return nil }
func (*admissionWriter) Drain(time.Duration) error  { return nil }

type permanentLine string

func (permanentLine) HeightForWidth(int) int   { return 1 }
func (line permanentLine) Draw(view grid.View) { view.Text(0, 0, string(line), grid.Style{}) }

func TestRefusedFrameAdvancesNoneOfTheComposedOwners(t *testing.T) {
	for _, test := range []struct {
		name   string
		width  int
		refuse bool
		cause  error
	}{
		{name: "writer capacity", width: 30, refuse: true, cause: term.ErrQueueFull},
		{name: "unrepresentable atom", width: 1, cause: errors.ErrUnsupported},
	} {
		t.Run(test.name, func(t *testing.T) {
			inline := grid.NewInline(test.width, 5)
			publisher, err := publication.New(publication.Config{Width: 30})
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := publisher.TryPublish(permanentLine("permanent界"))
			if err != nil {
				t.Fatal(err)
			}
			root, err := scene.NewRoot(scene.RootConfig{Element: scene.NewElement(scene.ElementConfig{
				Paint: func(frame scene.Frame) { frame.Text(0, 0, "live", grid.Style{}) },
			})})
			if err != nil {
				t.Fatal(err)
			}
			writer := &admissionWriter{refuse: test.refuse, changes: make(chan struct{})}
			p := &program{root: root, canvas: inline, inline: inline, publisher: publisher, writer: writer, tasks: newTaskQueue()}
			t.Cleanup(p.stop)
			if _, err := p.compose(false); !errors.Is(err, test.cause) {
				t.Fatalf("admission = %v", err)
			}
			if root.Progress().LayoutRevision != 0 || receipt.Snapshot().Sequence != 0 || len(writer.frames) != 0 {
				t.Fatal("refused bytes advanced a scene, publication receipt or writer")
			}
			if items, _ := publisher.Pending(); items != 1 {
				t.Fatalf("pending=%d after refusal", items)
			}
			writer.refuse = false
			inline.Resize(30, 5)
			if _, err := p.compose(false); err != nil {
				t.Fatal(err)
			}
			if root.Progress().WrittenSequence != 1 || receipt.Snapshot().State != publication.Written {
				t.Fatalf("committed progress=%+v receipt=%+v", root.Progress(), receipt.Snapshot())
			}
			if len(writer.frames) != 1 || strings.Count(string(writer.frames[0]), "permanent界") != 1 {
				t.Fatalf("publication bytes=%q", writer.frames)
			}
			if _, err := p.compose(false); err != nil {
				t.Fatal(err)
			}
			if len(writer.frames) != 1 {
				t.Fatal("the accepted contribution was replayed")
			}
		})
	}
}

// A single slot is released only by Drain, so cleanup cannot depend on the
// scheduler making the final frame leave an asynchronous writer quickly enough.
type drainingWriter struct {
	mu       sync.Mutex
	queued   uint64
	written  uint64
	pending  []byte
	frames   [][]byte
	drainErr error
	changes  chan struct{}
}

func (w *drainingWriter) Queue(frame []byte) (uint64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending != nil {
		return 0, term.ErrQueueFull
	}
	w.queued++
	w.pending = frame
	return w.queued, nil
}

func (w *drainingWriter) Changes() <-chan struct{} { return w.changes }
func (*drainingWriter) Err() error                 { return nil }

func (w *drainingWriter) Written() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.written
}

func (w *drainingWriter) Drain(time.Duration) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.drainErr != nil {
		return w.drainErr
	}
	if w.pending != nil {
		w.frames = append(w.frames, w.pending)
		w.pending = nil
		w.written = w.queued
		select {
		case w.changes <- struct{}{}:
		default:
		}
	}
	return nil
}

func TestInlineShutdownDrainsItsLastFrameBeforeCleanup(t *testing.T) {
	inline := grid.NewInline(30, 5)
	publisher, err := publication.New(publication.Config{Width: 30})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := publisher.TryPublish(permanentLine("permanent"))
	if err != nil {
		t.Fatal(err)
	}
	root, err := scene.NewRoot(scene.RootConfig{Element: scene.NewElement(scene.ElementConfig{
		Paint: func(frame scene.Frame) { frame.Text(0, 0, "live", grid.Style{}) },
	})})
	if err != nil {
		t.Fatal(err)
	}
	writer := &drainingWriter{changes: make(chan struct{}, 1)}
	p := &program{root: root, canvas: inline, inline: inline, publisher: publisher, writer: writer, tasks: newTaskQueue()}
	t.Cleanup(p.stop)
	if err := p.finish(); err != nil {
		t.Fatal(err)
	}
	if len(writer.frames) != 2 || !strings.Contains(string(writer.frames[0]), "permanent") ||
		!strings.Contains(string(writer.frames[0]), "live") || !strings.HasSuffix(string(writer.frames[1]), "\x1b[?25h") {
		t.Fatalf("shutdown frames=%q", writer.frames)
	}
	if snapshot := receipt.Snapshot(); snapshot.State != publication.Written || snapshot.Sequence != 1 {
		t.Fatalf("shutdown receipt=%+v", snapshot)
	}
	if items, _ := publisher.Pending(); items != 0 {
		t.Fatalf("shutdown left %d accepted contributions pending", items)
	}
}

func TestInlineCleanupPreservesADrainFailure(t *testing.T) {
	writer := &drainingWriter{drainErr: term.ErrDrainTimeout, changes: make(chan struct{}, 1)}
	if _, err := writer.Queue([]byte("accepted")); err != nil {
		t.Fatal(err)
	}
	p := &program{inline: grid.NewInline(30, 5), writer: writer}
	if err := p.finishBlock(); !errors.Is(err, ErrFrameTimeout) || !errors.Is(err, term.ErrDrainTimeout) {
		t.Fatalf("cleanup over pending output=%v", err)
	}
	if writer.queued != 1 || string(writer.pending) != "accepted" {
		t.Fatal("failed drain changed the accepted output")
	}
	writer.drainErr = nil
	if err := p.finishBlock(); err != nil {
		t.Fatal(err)
	}
	if writer.queued != 2 || len(writer.pending) == 0 {
		t.Fatal("refused cleanup consumed its retry")
	}
}
