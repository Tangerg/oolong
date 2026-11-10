package scene_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"
)

// Owner-supplied enums crossed into observations unchecked, while the same
// owners' action states were refused. An automation client cannot interpret them.
func TestObserveRefusesUninterpretableSemantics(t *testing.T) {
	for name, semantic := range map[string]scene.Semantic{
		"role":         {Role: scene.Role(200)},
		"state":        {Role: scene.RoleButton, State: 1 << 15},
		"availability": {Role: scene.RoleListItem, Availability: scene.ContentAvailability(9)},
		"relation":     {Role: scene.RoleButton, Relations: []scene.Relation{{Kind: scene.RelationKind(7)}}},
	} {
		t.Run(name, func(t *testing.T) {
			r := root(t, scene.NewElement(scene.ElementConfig{Semantics: func() scene.Semantic { return semantic }}))
			if _, err := r.Observe(scene.Query{}); !errors.Is(err, scene.ErrInvalid) {
				t.Fatalf("Observe = %v, want ErrInvalid", err)
			}
		})
	}
}

// A disabled target used to be described as outside the active scope.
func TestUnavailableActionsSayWhy(t *testing.T) {
	activate := scene.NewAction(scene.ActionConfig[struct{}, struct{}]{ID: scene.ActionActivate, Run: func(context.Context, struct{}) (scene.ActionResult[struct{}], error) {
		return scene.ActionResult[struct{}]{}, nil
	}})
	r := root(t, scene.NewElement(scene.ElementConfig{Key: "off", Enabled: func() bool { return false }, Actions: []scene.ActionBinding{activate.Binding()}}))
	observation, err := r.Observe(scene.Query{})
	if err != nil || len(observation.Nodes) != 1 || len(observation.Nodes[0].Actions) != 1 {
		t.Fatalf("observation = %+v, %v", observation, err)
	}
	if got := observation.Nodes[0].Actions[0].Availability; !got.Disabled || got.Reason != "target is disabled" {
		t.Fatalf("availability = %+v", got)
	}
}
