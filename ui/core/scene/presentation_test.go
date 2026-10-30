package scene_test

import (
	"image"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestInputReadsItsWrittenProjectionRatherThanNewAdmission(t *testing.T) {
	var projection scene.Snapshot[int]
	firstRow := 10
	hit := -1
	e := scene.NewElement(scene.ElementConfig{
		Paint: func(frame scene.Frame) { projection.Stage(frame, firstRow) },
		Handle: func(event scene.Event) scene.EventResult {
			mouse, ok := event.Input.(input.Mouse)
			if !ok || mouse.Action != input.MouseDown {
				return scene.EventResult{}
			}
			row, ok := event.Read(&projection)
			if !ok {
				t.Fatal("input lost its presentation projection")
			}
			hit = row + mouse.Pos.Y
			return scene.EventResult{Consumed: true}
		},
	})
	r := root(t, e)
	draw(t, r, 10, 2, 1)
	r.Wrote(1)
	click := r.CaptureInput(input.Mouse{Action: input.MouseDown, Pos: image.Pt(0, 1)})
	firstRow = 20
	draw(t, r, 10, 2, 2)
	if projection.Value() != 20 {
		t.Fatal("logical projection did not commit")
	}
	r.Wrote(2)
	r.HandleInput(click)
	if hit != 11 {
		t.Fatalf("old click used a newly admitted scrolling offset: %d", hit)
	}
}

func TestBufferedMouseTimestampAndGeometryInvalidation(t *testing.T) {
	now := time.Unix(100, 0)
	var projection scene.Snapshot[int]
	value, hit := 1, 0
	e := scene.NewElement(scene.ElementConfig{
		Paint: func(frame scene.Frame) { projection.Stage(frame, value) },
		Handle: func(event scene.Event) scene.EventResult {
			if v, ok := event.Read(&projection); ok {
				hit = v
			}
			return scene.EventResult{Consumed: true}
		},
	})
	r, err := scene.NewRoot(scene.RootConfig{Element: e, InputHistory: 2, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	draw(t, r, 5, 1, 1)
	r.Wrote(1)
	at := now.Add(time.Second)
	now = now.Add(2 * time.Second)
	value = 2
	draw(t, r, 5, 1, 2)
	r.Wrote(2)
	r.Handle(input.Mouse{Action: input.MouseDown, At: at})
	if hit != 1 {
		t.Fatalf("buffer delay rewrote arrival geometry: %d", hit)
	}
	queued := r.CaptureInput(input.Mouse{Action: input.MouseDown})
	r.InvalidateGeometry()
	hit = 0
	r.HandleInput(queued)
	if hit != 0 {
		t.Fatal("resize reused an invalidated coordinate space")
	}
	now = now.Add(time.Second)
	draw(t, r, 6, 2, 3)
	r.Wrote(3)
	r.Handle(input.Mouse{Action: input.MouseDown, At: at})
	if hit != 0 {
		t.Fatal("expired timestamp was reinterpreted against newer geometry")
	}
}

func TestSnapshotProducerAndBorrowLifetimeAreExplicit(t *testing.T) {
	var snapshot scene.Snapshot[int]
	var held scene.Frame
	var staged *scene.Staged[int]
	e := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { held = frame; staged = snapshot.Stage(frame, 1); staged.Set(2) }})
	r := root(t, e)
	p, err := r.Prepare(grid.NewSurface(1, 1).View())
	if err != nil {
		t.Fatal(err)
	}
	panics := func(f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Error("expired frame operation did not panic")
			}
		}()
		f()
	}
	panics(func() { staged.Set(3) })
	panics(func() { snapshot.Stage(held, 3) })
	p.Commit(0)
	if snapshot.Value() != 2 {
		t.Fatal("single producer refinement was not committed")
	}
	duplicate := scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) { snapshot.Stage(frame, 1); snapshot.Stage(frame, 2) }})
	if err := r.SetContent(duplicate); err != nil {
		t.Fatal(err)
	}
	panics(func() { _, _ = r.Prepare(grid.NewSurface(1, 1).View()) })
	if snapshot.Value() != 2 {
		t.Fatal("duplicate producer partially committed")
	}
}
