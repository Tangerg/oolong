package scene_test

import (
	"runtime"
	"testing"
	"time"
	"weak"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func settledCandidate(t *testing.T, commit bool) (*scene.Prepared, weak.Pointer[[1024]byte]) {
	t.Helper()
	payload := new([1024]byte)
	var projection scene.Snapshot[*[1024]byte]
	r, err := scene.NewRoot(scene.RootConfig{Element: scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		projection.Stage(frame, payload)
	}})})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	candidate, err := r.Prepare(grid.NewSurface(1, 1).View())
	if err != nil {
		t.Fatal(err)
	}
	if commit {
		candidate.Commit(0)
	} else {
		candidate.Abort()
	}
	return candidate, weak.Make(payload)
}

func TestSettledCandidatesReleaseTheirFrameContent(t *testing.T) {
	for _, completion := range []struct {
		name   string
		commit bool
	}{
		{name: "commit", commit: true},
		{name: "abort"},
	} {
		t.Run(completion.name, func(t *testing.T) {
			candidate, payload := settledCandidate(t, completion.commit)
			runtime.GC()
			if payload.Value() != nil {
				t.Fatal("a settled candidate retained its frame content after the scene closed")
			}
			candidate.Abort()
			defer func() {
				if recover() == nil {
					t.Error("settlement made a used candidate reusable")
				}
				runtime.KeepAlive(candidate)
			}()
			candidate.Commit(0)
		})
	}
}

func closedRootWithClock(t *testing.T) (*scene.Root, weak.Pointer[[1024]byte]) {
	t.Helper()
	clock := new([1024]byte)
	r, err := scene.NewRoot(scene.RootConfig{Now: func() time.Time {
		runtime.KeepAlive(clock)
		return time.Unix(1, 0)
	}})
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	return r, weak.Make(clock)
}

func TestClosedRootReleasesItsProjectionClock(t *testing.T) {
	r, clock := closedRootWithClock(t)
	runtime.GC()
	if clock.Value() != nil {
		t.Fatal("closed scene retained its projection clock's captured state")
	}
	r.Close()
	runtime.KeepAlive(r)
}
