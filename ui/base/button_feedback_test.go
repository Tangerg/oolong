package base_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// A refused key or pointer activation has no caller to return to. Its error was
// recorded but shown nowhere, so neither the user nor an assistive client knew.
func TestAPhysicalActivationFailureIsObservable(t *testing.T) {
	refused := errors.New("server unavailable")
	button := base.NewButton(base.ButtonConfig{Key: "send", Label: "Send", Activate: func(context.Context) (scene.ActionResult[struct{}], error) {
		return scene.ActionResult[struct{}]{}, refused
	}})
	root := semanticOwner(t, button.Element())
	button.Do(base.Activate)
	observation, err := root.Observe(scene.Query{})
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range observation.Nodes {
		if node.Key == "send" {
			if !node.State.Has(scene.StateInvalid) || node.Description != refused.Error() {
				t.Fatalf("semantics = %+v, want the refusal", node.Semantic)
			}
			return
		}
	}
	t.Fatal("button not observed")
}
