package scene_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func inputProjection(t *testing.T, history int) (*scene.Root, *int, *int) {
	t.Helper()
	value, hit := 1, 0
	var projection scene.Snapshot[int]
	r, err := scene.NewRoot(scene.RootConfig{InputHistory: history, Element: scene.NewElement(scene.ElementConfig{
		Paint: func(frame scene.Frame) { projection.Stage(frame, value) },
		Handle: func(event scene.Event) scene.EventResult {
			if mouse, ok := event.Input().(input.Mouse); ok && mouse.Action == input.MouseDown {
				hit, _ = event.Read(&projection)
				return scene.EventResult{Consumed: true}
			}
			return scene.EventResult{}
		},
	})})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r, &value, &hit
}

func mouseAt(r *scene.Root, at time.Time, sequence uint64) scene.CapturedInput {
	return r.CaptureInput(input.Mouse{Action: input.MouseDown, At: at}, func(asked time.Time) (uint64, bool) {
		return sequence, asked.Equal(at)
	})
}

func TestZeroByteGeometryUsesItsAdmissionTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r, value, hit := inputProjection(t, 0)
		draw(t, r, 1, 1, 1)
		r.Wrote(1)
		time.Sleep(time.Nanosecond)
		oldArrival := time.Now()
		captured := mouseAt(r, oldArrival, 1)
		time.Sleep(time.Nanosecond)
		*value = 2
		draw(t, r, 1, 1, 0)
		time.Sleep(time.Nanosecond)
		r.HandleInput(mouseAt(r, time.Now(), 1))
		if *hit != 2 {
			t.Fatalf("new zero-byte geometry = %d", *hit)
		}
		r.HandleInput(mouseAt(r, oldArrival, 1))
		if *hit != 1 {
			t.Fatalf("delayed input acquired later zero-byte geometry: %d", *hit)
		}
		r.HandleInput(captured)
		if *hit != 1 {
			t.Fatalf("captured input lost its immutable geometry: %d", *hit)
		}
	})
}

func TestAdmissionGapDoesNotAuthorizeThePreviousMap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r, value, hit := inputProjection(t, 0)
		draw(t, r, 1, 1, 1)
		r.Wrote(1)
		*value = 2
		candidate, err := r.Prepare(grid.NewSurface(1, 1).View())
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Nanosecond)
		arrival := time.Now()
		captured := mouseAt(r, arrival, 2)
		time.Sleep(time.Nanosecond)
		candidate.Commit(2)
		r.HandleInput(captured)
		r.HandleInput(mouseAt(r, arrival, 2))
		if *hit != 0 {
			t.Fatal("input between transport completion and commit acquired a hit map")
		}
		time.Sleep(time.Nanosecond)
		r.HandleInput(mouseAt(r, time.Now(), 2))
		if *hit != 2 {
			t.Fatalf("committed geometry still waited for the owner's Wrote notification: %d", *hit)
		}
		candidate, err = r.Prepare(grid.NewSurface(1, 1).View())
		if err != nil {
			t.Fatal(err)
		}
		candidate.Abort()
		r.HandleInput(mouseAt(r, time.Now(), 2))
		if *hit != 2 {
			t.Fatal("aborted admission kept its coordinate barrier")
		}
	})
}

func TestInputHistoryKeepsCapturedMapsAndRejectsEvictedEvidence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r, value, hit := inputProjection(t, 2)
		draw(t, r, 1, 1, 1)
		time.Sleep(time.Nanosecond)
		arrival := time.Now()
		captured := mouseAt(r, arrival, 1)
		for _, sequence := range []uint64{3, 5} {
			time.Sleep(time.Nanosecond)
			*value++
			draw(t, r, 1, 1, sequence)
		}
		r.Wrote(7)
		r.HandleInput(mouseAt(r, arrival, 1))
		if *hit != 0 {
			t.Fatal("evicted evidence acquired a newer hit map")
		}
		r.HandleInput(captured)
		if *hit != 1 {
			t.Fatal("history retirement revoked already captured input")
		}
		time.Sleep(time.Nanosecond)
		r.HandleInput(mouseAt(r, time.Now(), 7))
		if *hit != 3 {
			t.Fatal("non-layout writer sequences prevented association with the last layout")
		}
		*hit = 0
		r.HandleInput(r.CaptureInput(input.Mouse{Action: input.MouseDown}, func(time.Time) (uint64, bool) { return 7, false }))
		if *hit != 0 {
			t.Fatal("unknown completion used the latest written projection")
		}
	})
}
