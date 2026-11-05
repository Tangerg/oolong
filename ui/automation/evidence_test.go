package automation_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/automation"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestLocatorRequiresEvidenceForTheWholeNameAndSearch(t *testing.T) {
	called := 0
	action := scene.NewAction(scene.ActionConfig[struct{}, struct{}]{ID: scene.ActionActivate, Run: func(context.Context, struct{}) (scene.ActionResult[struct{}], error) {
		called++
		return scene.ActionResult[struct{}]{State: scene.Applied}, nil
	}})
	button := func(name string) scene.Element {
		return scene.NewElement(scene.ElementConfig{Actions: []scene.ActionBinding{action.Binding()}, Semantics: func() scene.Semantic {
			return scene.Semantic{Role: scene.RoleButton, Name: name, Description: "a deliberately long description"}
		}})
	}
	first, second := button("Save"), button("Save draft")
	g := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Element: first}, {Element: second}}})
	root, err := scene.NewRoot(scene.RootConfig{Element: g.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	d, err := automation.NewDriver(&owner{root: root, done: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	page, err := d.Observe(t.Context(), scene.Query{Limit: 1})
	if err != nil || page.Next == (scene.Cursor{}) {
		t.Fatalf("first page: %+v, %v", page, err)
	}
	for _, tc := range []struct {
		label string
		query scene.Query
		match scene.Match
		want  error
	}{
		{"last page omits another button", scene.Query{Limit: 1, Cursor: page.Next}, scene.Match{Role: scene.RoleButton}, scene.ErrIncompleteObservation},
		{"displayed prefix is not an exact name", scene.Query{Root: second.Target(), TextLimit: 4}, scene.Match{Name: "Save"}, scene.ErrNotFound},
		{"hidden suffix cannot prove absence", scene.Query{Root: second.Target(), TextLimit: 4}, scene.Match{Name: "Save draft"}, scene.ErrIncompleteObservation},
		{"unseen page cannot prove absence", scene.Query{Limit: 1}, scene.Match{Name: "Save draft"}, scene.ErrIncompleteObservation},
	} {
		t.Run(tc.label, func(t *testing.T) {
			before := called
			_, invokeErr := d.Locate(tc.query, tc.match).Invoke(t.Context(), scene.ActionActivate, nil)
			if invokeErr == nil || called != before {
				t.Fatalf("action ran on partial evidence: calls=%d err=%v", called-before, invokeErr)
			}
			if !errors.Is(invokeErr, tc.want) {
				t.Fatalf("resolution = %v, want %v", invokeErr, tc.want)
			}
		})
	}
	target, err := d.Find(t.Context(), scene.Query{TextLimit: 4}, scene.Match{Name: "Save"})
	if err != nil || target != first.Target() {
		t.Fatalf("exact name lost because another name or description was clipped: %v, %v", target, err)
	}
}

func TestObservationAndInvocationAgreeOnNodeEligibility(t *testing.T) {
	enabled := true
	action := scene.NewAction(scene.ActionConfig[struct{}, struct{}]{ID: scene.ActionActivate, Run: func(context.Context, struct{}) (scene.ActionResult[struct{}], error) {
		return scene.ActionResult[struct{}]{State: scene.Applied}, nil
	}})
	element := scene.NewElement(scene.ElementConfig{Enabled: func() bool { return enabled }, Actions: []scene.ActionBinding{action.Binding()}, Semantics: func() scene.Semantic {
		return scene.Semantic{Role: scene.RoleButton, State: scene.StateDisabled | scene.StateSelected}
	}})
	root, err := scene.NewRoot(scene.RootConfig{Element: element})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, allowed := range []bool{true, false} {
		enabled = allowed
		element.Invalidate()
		seen, err := root.Observe(scene.Query{})
		if err != nil {
			t.Fatal(err)
		}
		node := seen.Nodes[0]
		if node.State.Has(scene.StateDisabled) == allowed || node.Actions[0].Availability.Disabled == allowed || !node.State.Has(scene.StateSelected) {
			t.Fatalf("node and action disagree with eligibility %t: %+v", allowed, node)
		}
		_, err = root.Invoke(t.Context(), element.Target(), scene.ActionActivate, nil)
		if (allowed && err != nil) || (!allowed && !errors.Is(err, scene.ErrUnavailable)) {
			t.Fatalf("invoke for enabled=%t: %v", allowed, err)
		}
	}
}
