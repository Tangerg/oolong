package scene_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestSnapshotRevisionProjectsTheOwningLayout(t *testing.T) {
	var projection scene.Snapshot[int]
	stage := true
	r := root(t, scene.NewElement(scene.ElementConfig{Paint: func(frame scene.Frame) {
		if stage {
			projection.Stage(frame, 1)
		}
	}}))
	draw(t, r, 1, 1, 0)
	first := projection.Revision()
	stage = false
	draw(t, r, 1, 1, 0)
	if projection.Revision() != first {
		t.Fatal("unstaged geometry advanced independently")
	}
	stage = true
	draw(t, r, 1, 1, 0)
	if projection.Revision() != r.Progress().LayoutRevision {
		t.Fatalf("snapshot invented a second revision: projection=%d layout=%d", projection.Revision(), r.Progress().LayoutRevision)
	}
}

func TestCommitNeverCallsTheClockOrOtherApplicationCode(t *testing.T) {
	reads := 0
	r, err := scene.NewRoot(scene.RootConfig{Element: scene.NewElement(scene.ElementConfig{}), Now: func() time.Time {
		reads++
		return time.Unix(100, 0)
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	candidate, err := r.Prepare(grid.NewSurface(1, 1).View())
	if err != nil {
		t.Fatal(err)
	}
	candidate.Commit(0)
	if reads != 0 || r.Progress().WrittenFrame != 0 {
		t.Fatal("commit crossed the callback-free admission boundary")
	}
	r.Wrote(0)
	if reads != 1 || r.Progress().WrittenFrame != r.Progress().LayoutRevision {
		t.Fatal("explicit write confirmation did not publish the zero-byte layout")
	}
}

func TestTransportOnlyProgressWakesObserversOnce(t *testing.T) {
	r := root(t, scene.NewElement(scene.ElementConfig{}))
	draw(t, r, 1, 1, 0)
	before := r.Progress()
	changed := r.Changes()
	r.Wrote(7)
	select {
	case <-changed:
	default:
		t.Fatal("writer advancement left automation waiting indefinitely")
	}
	if after := r.Progress(); after.WrittenSequence != 7 || after.WrittenFrame != before.WrittenFrame || after.LayoutRevision != before.LayoutRevision || after.ModelRevision != before.ModelRevision {
		t.Fatalf("transport progress advanced a different fact: before=%+v after=%+v", before, after)
	}
	changed = r.Changes()
	r.Wrote(7)
	select {
	case <-changed:
		t.Fatal("an unchanged watermark generated another transition")
	default:
	}
}

func TestRootMutationsRejectPresentationAndPendingSettlement(t *testing.T) {
	for _, operation := range []struct {
		name string
		run  func(*scene.Root)
	}{{"changed", (*scene.Root).Changed}, {"close", (*scene.Root).Close}, {"invalidate geometry", (*scene.Root).InvalidateGeometry}} {
		for _, phase := range []string{"paint", "observe", "pending"} {
			t.Run(operation.name+"/"+phase, func(t *testing.T) {
				var r *scene.Root
				active, rejected := false, false
				attempt := func() {
					defer func() { rejected = recover() != nil }()
					operation.run(r)
				}
				e := scene.NewElement(scene.ElementConfig{
					Paint: func(scene.Frame) {
						if active && phase == "paint" {
							attempt()
						}
					},
					Semantics: func() scene.Semantic {
						if active && phase == "observe" {
							attempt()
						}
						return scene.Semantic{Role: scene.RoleStatus}
					},
				})
				r = root(t, e)
				before, target := r.Progress(), e.Target()
				active = true
				if phase == "observe" {
					if _, err := r.Observe(scene.Query{}); err != nil {
						t.Fatal(err)
					}
				} else {
					candidate, err := r.Prepare(grid.NewSurface(1, 1).View())
					if err != nil {
						t.Fatal(err)
					}
					if phase == "pending" {
						attempt()
					}
					candidate.Abort()
				}
				if !rejected || !r.Valid(target) || r.Progress() != before {
					t.Fatal("rejected mutation changed scene ownership or progress")
				}
			})
		}
	}
}

func TestPreviewFocusResultStopsTheOldKeyboardRoute(t *testing.T) {
	calls := 0
	leaf := scene.NewElement(scene.ElementConfig{Focusable: func() bool { return true }, Handle: func(scene.Event) scene.EventResult {
		calls++
		return scene.EventResult{}
	}})
	group := scene.NewGroup(scene.GroupConfig{Element: scene.ElementConfig{
		Focusable: func() bool { return true }, Preview: func(scene.Event) scene.EventResult { return scene.EventResult{Focus: true} },
	}, Items: []scene.Item{{Element: leaf}}})
	r := root(t, group.Element())
	if err := leaf.RequestFocus(); err != nil {
		t.Fatal(err)
	}
	r.Handle(input.Key{Rune: 'a'})
	if calls != 0 || r.Focus() != group.Element().Target() {
		t.Fatal("a changed keyboard target resumed the old route")
	}
}

func TestStoredPressCannotCaptureDuringPaintingOrAfterRelease(t *testing.T) {
	var press scene.Event
	var paintCapture error
	captureInPaint := false
	e := scene.NewElement(scene.ElementConfig{
		Paint: func(scene.Frame) {
			if captureInPaint {
				paintCapture = press.Capture()
			}
		},
		Handle: func(event scene.Event) scene.EventResult {
			if mouse, ok := event.Input.(input.Mouse); ok && mouse.Action == input.MouseDown {
				press = event
			}
			return scene.EventResult{Consumed: true}
		},
	})
	r := root(t, e)
	draw(t, r, 1, 1, 0)
	r.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft})
	captureInPaint = true
	draw(t, r, 1, 1, 0)
	if !errors.Is(paintCapture, scene.ErrBusy) {
		t.Fatalf("paint capture = %v", paintCapture)
	}
	r.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft})
	if err := press.Capture(); !errors.Is(err, scene.ErrStale) {
		t.Fatalf("stored press revived a completed gesture: %v", err)
	}
}

func TestActionQueriesCannotDetachTheTargetTheyAuthorize(t *testing.T) {
	for _, query := range []string{"enabled", "availability"} {
		t.Run(query, func(t *testing.T) {
			var group *scene.Group
			active, calls, runs := false, 0, 0
			tryDetach := func() {
				if active {
					calls++
					if err := group.Set(); !errors.Is(err, scene.ErrBusy) {
						t.Errorf("query detached its action target: %v", err)
					}
				}
			}
			action := scene.NewAction(scene.ActionConfig[struct{}, struct{}]{
				ID: scene.ActionActivate,
				Availability: func(struct{}) scene.Availability {
					if query == "availability" {
						tryDetach()
					}
					return scene.Availability{}
				},
				Run: func(context.Context, struct{}) (scene.ActionResult[struct{}], error) {
					runs++
					return scene.ActionResult[struct{}]{State: scene.Applied}, nil
				},
			})
			element := scene.NewElement(scene.ElementConfig{
				Enabled: func() bool {
					if query == "enabled" {
						tryDetach()
					}
					return true
				}, Actions: []scene.ActionBinding{action.Binding()},
			})
			group = scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Element: element}}})
			r := root(t, group.Element())
			active = true
			if result, err := r.Invoke(t.Context(), element.Target(), scene.ActionActivate, nil); err != nil || result.State != scene.Applied {
				t.Fatalf("action = %+v, %v", result, err)
			}
			if !element.Alive() || calls == 0 || runs != 1 {
				t.Fatal("action escaped the single current attachment or ran twice")
			}
		})
	}
}
