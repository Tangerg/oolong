package scene_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestOwnerOutcomesHaveOneAdmissionRuleForTypedAndSemanticActions(t *testing.T) {
	problem := errors.New("owner reported a later failure")
	for _, test := range []struct {
		name  string
		state scene.ActionState
		valid bool
	}{
		{"unchanged", scene.Unchanged, true},
		{"applied", scene.Applied, true},
		{"started", scene.Started, true},
		{"above range", scene.Started + 1, false},
		{"largest", ^scene.ActionState(0), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.state.Validate(); (err == nil) != test.valid || !test.valid && !errors.Is(err, scene.ErrInvalid) {
				t.Fatalf("state validation = %v, valid=%v", err, test.valid)
			}
			action := scene.NewAction(scene.ActionConfig[struct{}, bool]{
				ID: scene.ActionActivate,
				Run: func(context.Context, struct{}) (scene.ActionResult[bool], error) {
					return scene.ActionResult[bool]{State: test.state, Value: true}, problem
				},
			})
			want := scene.ActionResult[bool]{}
			if test.valid {
				want = scene.ActionResult[bool]{State: test.state, Value: true}
			}
			result, err := action.Invoke(t.Context(), struct{}{})
			if result != want || !errors.Is(err, problem) || errors.Is(err, scene.ErrInvalid) == test.valid {
				t.Fatalf("typed outcome = %+v, %v; want %+v", result, err, want)
			}
			element := scene.NewElement(scene.ElementConfig{Actions: []scene.ActionBinding{action.Binding()}})
			sceneRoot := root(t, element)
			invoked, err := sceneRoot.Invoke(t.Context(), element.Target(), scene.ActionActivate, nil)
			if invoked.State != want.State || invoked.Value != want.Value || !errors.Is(err, problem) || errors.Is(err, scene.ErrInvalid) == test.valid {
				t.Fatalf("semantic outcome = %+v, %v; want %+v", invoked, err, want)
			}
		})
	}
}
