package scene_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestTypedAndSemanticActionsPreserveCancellationCause(t *testing.T) {
	cause := errors.New("request owner ended")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	called := false
	action := scene.NewAction(scene.ActionConfig[struct{}, bool]{
		ID: scene.ActionActivate,
		Run: func(context.Context, struct{}) (scene.ActionResult[bool], error) {
			called = true
			return scene.ActionResult[bool]{State: scene.Applied, Value: true}, nil
		},
	})
	if result, err := action.Invoke(ctx, struct{}{}); !errors.Is(err, cause) || result.State != scene.Unchanged {
		t.Errorf("typed cancellation = %+v, %v", result, err)
	}
	element := scene.NewElement(scene.ElementConfig{Actions: []scene.ActionBinding{action.Binding()}})
	sceneRoot := root(t, element)
	if result, err := sceneRoot.Invoke(ctx, element.Target(), scene.ActionActivate, nil); !errors.Is(err, cause) || result.State != scene.Unchanged {
		t.Errorf("semantic cancellation = %+v, %v", result, err)
	}
	if called {
		t.Fatal("cancelled action reached its handler")
	}
}
