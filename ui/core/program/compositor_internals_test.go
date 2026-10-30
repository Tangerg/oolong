package program

import (
	"errors"
	"strings"
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
	writer := &admissionWriter{refuse: true, changes: make(chan struct{})}
	p := &program{root: root, canvas: inline, inline: inline, publisher: publisher, writer: writer, tasks: newTaskQueue()}
	t.Cleanup(p.stop)
	if _, err := p.compose(false); !errors.Is(err, term.ErrQueueFull) {
		t.Fatalf("admission = %v", err)
	}
	if root.Progress().LayoutRevision != 0 || receipt.Snapshot().Sequence != 0 {
		t.Fatal("refused bytes advanced a scene or publication receipt")
	}
	if items, _ := publisher.Pending(); items != 1 {
		t.Fatalf("pending=%d after refusal", items)
	}
	writer.refuse = false
	if _, err := p.compose(false); err != nil {
		t.Fatal(err)
	}
	if root.Progress().WrittenSequence != 1 || receipt.Snapshot().State != publication.Written {
		t.Fatalf("committed progress=%+v receipt=%+v", root.Progress(), receipt.Snapshot())
	}
	if len(writer.frames) != 1 || strings.Count(string(writer.frames[0]), "permanent") != 1 {
		t.Fatalf("publication bytes=%q", writer.frames)
	}
	if _, err := p.compose(false); err != nil {
		t.Fatal(err)
	}
	if len(writer.frames) != 1 {
		t.Fatal("the accepted contribution was replayed")
	}
}
