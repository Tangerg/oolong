package program_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestSceneCapabilityFollowsTheSessionWithoutFrameAuthority(t *testing.T) {
	var checkEnded func()
	err := runFixture(t.Context(), program.ScreenConfig{Host: newHost(t), Build: func(rt *program.Runtime) (scene.Element, error) {
		view := rt.Scene()
		if _, ok := any(view).(interface{ Wrote(sequence uint64) }); ok {
			t.Error("semantic caller can advance the physical writer watermark")
		}
		if _, err := view.Observe(scene.Query{}); !errors.Is(err, scene.ErrDetached) {
			t.Errorf("observation before attachment=%v", err)
		}
		if _, err := view.Invoke(t.Context(), scene.Target{}, scene.ActionActivate, nil); !errors.Is(err, scene.ErrDetached) {
			t.Errorf("invocation before attachment=%v", err)
		}
		if view.Changes() != nil {
			t.Error("unattached capability invented a change notification")
		}
		activated := 0
		action := scene.NewAction(scene.ActionConfig[struct{}, int]{ID: scene.ActionActivate, Run: func(context.Context, struct{}) (scene.ActionResult[int], error) {
			activated++
			return scene.ActionResult[int]{State: scene.Applied, Value: activated}, nil
		}})
		e := scene.NewElement(scene.ElementConfig{Key: "button", Actions: []scene.ActionBinding{action.Binding()}})
		checkEnded = func() {
			if _, err := view.Observe(scene.Query{}); !errors.Is(err, program.ErrStopped) {
				t.Errorf("stopped observation=%v", err)
			}
			if _, err := view.Invoke(t.Context(), e.Target(), scene.ActionActivate, nil); !errors.Is(err, program.ErrStopped) {
				t.Errorf("stopped invocation=%v", err)
			}
			select {
			case <-view.Changes():
			default:
				t.Error("stopped semantic capability did not settle change waiters")
			}
		}
		err := rt.Dispatcher().Post(func() {
			defer rt.Quit()
			current, err := view.Observe(scene.Query{})
			if err != nil || len(current.Nodes) != 1 || current.Nodes[0].Target != e.Target() {
				t.Errorf("attached observation=%+v, %v", current, err)
				return
			}
			before := view.Changes()
			if _, err := view.Invoke(t.Context(), e.Target(), scene.ActionActivate, nil); err != nil || activated != 1 {
				t.Errorf("action calls=%d err=%v", activated, err)
			}
			select {
			case <-before:
			default:
				t.Error("application action did not notify the semantic reader")
			}
		})
		return e, err
	}})
	if err != nil {
		t.Fatal(err)
	}
	checkEnded()
}
