package scene_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestObservationByteBudgetPaginatesCompleteNodes(t *testing.T) {
	leaf := func(key string) scene.Element {
		return scene.NewElement(scene.ElementConfig{Key: key, Semantics: func() scene.Semantic {
			return scene.Semantic{Value: "data"}
		}})
	}
	r := root(t, scene.NewGroup(scene.GroupConfig{Items: []scene.Item{
		item("first", leaf("first")), item("second", leaf("second")),
	}}).Element())
	query := scene.Query{ByteLimit: len("seconddata")}
	first, err := r.Observe(query)
	if err != nil || len(first.Nodes) != 1 || first.Nodes[0].Key != "first" || first.Nodes[0].Value != "data" {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	if first.Next == (scene.Cursor{}) || !first.Truncated {
		t.Fatal("byte budget omitted a node without a continuation")
	}
	if _, findErr := first.FindUnique(scene.Match{Key: "first"}); !errors.Is(findErr, scene.ErrIncompleteObservation) {
		t.Fatalf("partial byte-budget page claimed uniqueness: %v", findErr)
	}
	query.Cursor = first.Next
	second, err := r.Observe(query)
	if err != nil || len(second.Nodes) != 1 || second.Nodes[0].Key != "second" || second.Nodes[0].Value != "data" {
		t.Fatalf("second page = %+v, %v", second, err)
	}
	if second.Next != (scene.Cursor{}) {
		t.Fatal("final page did not finish")
	}
}

func TestObservationNeverTruncatesIdentityToFitItsBudget(t *testing.T) {
	const budget = 128
	oversized := strings.Repeat("x", budget+1)
	for _, field := range []string{"key", "content", "action", "part"} {
		t.Run(field, func(t *testing.T) {
			cfg := scene.ElementConfig{Key: "node"}
			switch field {
			case "key":
				cfg.Key = oversized
			case "content":
				cfg.Semantics = func() scene.Semantic { return scene.Semantic{ContentID: oversized} }
			case "action":
				cfg.Actions = []scene.ActionBinding{observationAction(scene.ActionID(oversized)).Binding()}
			case "part":
				cfg.Part = scene.PartID(oversized)
			}
			r := root(t, scene.NewElement(cfg))
			page, err := r.Observe(scene.Query{ByteLimit: budget, TextLimit: 1})
			if !errors.Is(err, scene.ErrObservationBudget) || len(page.Nodes) != 0 || page.Next != (scene.Cursor{}) {
				t.Fatalf("oversized %s produced partial identity: %+v, %v", field, page, err)
			}
			full, err := r.Observe(scene.Query{})
			if err != nil || len(full.Nodes) != 1 {
				t.Fatalf("budget rejection changed the model: %+v, %v", full, err)
			}
			var identity string
			switch field {
			case "key":
				identity = full.Nodes[0].Key
			case "content":
				identity = full.Nodes[0].ContentID
			case "action":
				identity = string(full.Nodes[0].Actions[0].ID)
			case "part":
				identity = string(full.Nodes[0].Target.Part())
			}
			if identity != oversized {
				t.Fatal("observation changed an identity")
			}
		})
	}
}

func TestObservationChargesParentAndRelationIdentities(t *testing.T) {
	const part = scene.PartID("component-part")
	child := scene.NewElement(scene.ElementConfig{Key: "child"})
	parent := scene.NewGroup(scene.GroupConfig{
		Element: scene.ElementConfig{Key: "parent", Part: part},
		Items:   []scene.Item{item("child", child)},
	})
	r := root(t, parent.Element())
	first, err := r.Observe(scene.Query{Limit: 1})
	if err != nil || first.Next == (scene.Cursor{}) {
		t.Fatal(first, err)
	}
	query := scene.Query{Cursor: first.Next, ByteLimit: len("child")}
	if _, err = r.Observe(query); !errors.Is(err, scene.ErrObservationBudget) {
		t.Fatalf("parent identity escaped the byte budget: %v", err)
	}
	query.ByteLimit += len(part)
	page, err := r.Observe(query)
	if err != nil || len(page.Nodes) != 1 || page.Nodes[0].Parent != parent.Element().Target() {
		t.Fatalf("parent identity was changed or rejected at its exact budget: %+v, %v", page, err)
	}

	related := scene.NewElement(scene.ElementConfig{Part: part})
	control := scene.NewElement(scene.ElementConfig{Key: "control", Semantics: func() scene.Semantic {
		return scene.Semantic{Relations: []scene.Relation{{Kind: scene.Controls, Target: related.Target()}}}
	}})
	other := root(t, scene.NewGroup(scene.GroupConfig{Items: []scene.Item{
		item("related", related), item("control", control),
	}}).Element())
	query = scene.Query{ByteLimit: len("control")}
	if _, err = other.Observe(query); !errors.Is(err, scene.ErrObservationBudget) {
		t.Fatalf("relation identity escaped the byte budget: %v", err)
	}
	query.ByteLimit += len(part)
	page, err = other.Observe(query)
	if err != nil || len(page.Nodes) != 1 || len(page.Nodes[0].Relations) != 1 || page.Nodes[0].Relations[0].Target != related.Target() {
		t.Fatalf("relation identity was changed or rejected at its exact budget: %+v, %v", page, err)
	}
}

func TestObservationBoundsActionPresentationWithoutChangingItsOwner(t *testing.T) {
	const label = "界面操作"
	const reason = "可以执行"
	action := scene.NewAction(scene.ActionConfig[struct{}, struct{}]{
		ID: scene.ActionActivate, Label: label,
		Availability: func(struct{}) scene.Availability { return scene.Availability{Reason: reason} },
		Run: func(context.Context, struct{}) (scene.ActionResult[struct{}], error) {
			return scene.ActionResult[struct{}]{State: scene.Applied}, nil
		},
	})
	r := root(t, scene.NewElement(scene.ElementConfig{Key: "control", Actions: []scene.ActionBinding{action.Binding()}}))
	page, err := r.Observe(scene.Query{TextLimit: 4})
	if err != nil || len(page.Nodes) != 1 || len(page.Nodes[0].Actions) != 1 {
		t.Fatal(page, err)
	}
	got := page.Nodes[0].Actions[0]
	if got.Label != "界" || got.Availability.Reason != "可" || !utf8.ValidString(got.Label+got.Availability.Reason) {
		t.Fatalf("action presentation was not clipped at a rune boundary: %+v", got)
	}
	if got.TruncatedText != scene.LabelTruncated|scene.ReasonTruncated || got.ID != action.ID() {
		t.Fatalf("clipping lost evidence or changed identity: %+v", got)
	}
	full, err := r.Observe(scene.Query{})
	if err != nil || full.Nodes[0].Actions[0].Label != label || full.Nodes[0].Actions[0].Availability.Reason != reason {
		t.Fatalf("projection modified the action: %+v, %v", full, err)
	}
}

func TestObservationDetailBudgetIncludesActionsAndLiveRelations(t *testing.T) {
	for _, counts := range []struct{ actions, relations int }{{3, 0}, {0, 3}, {1, 2}} {
		var element scene.Element
		actions := make([]scene.ActionBinding, counts.actions)
		ids := []scene.ActionID{scene.ActionActivate, scene.ActionSubmit, scene.ActionDismiss}
		for i := range actions {
			actions[i] = observationAction(ids[i]).Binding()
		}
		element = scene.NewElement(scene.ElementConfig{Key: "node", Actions: actions, Semantics: func() scene.Semantic {
			relations := make([]scene.Relation, counts.relations)
			for i := range relations {
				relations[i] = scene.Relation{Kind: scene.Controls, Target: element.Target()}
			}
			return scene.Semantic{Relations: relations}
		}})
		r := root(t, element)
		page, err := r.Observe(scene.Query{DetailLimit: 2})
		if !errors.Is(err, scene.ErrObservationBudget) || len(page.Nodes) != 0 {
			t.Fatalf("%+v exceeded detail budget: %+v, %v", counts, page, err)
		}
		page, err = r.Observe(scene.Query{DetailLimit: 3})
		if err != nil || len(page.Nodes) != 1 || len(page.Nodes[0].Actions)+len(page.Nodes[0].Relations) != 3 {
			t.Fatalf("exact detail budget rejected %+v: %+v, %v", counts, page, err)
		}
	}
}

func TestObservationDetailBudgetPaginatesWithoutSplittingActions(t *testing.T) {
	leaf := func(key string) scene.Element {
		return scene.NewElement(scene.ElementConfig{Key: key, Actions: []scene.ActionBinding{
			observationAction(scene.ActionActivate).Binding(),
		}})
	}
	r := root(t, scene.NewGroup(scene.GroupConfig{Items: []scene.Item{
		item("first", leaf("first")), item("second", leaf("second")),
	}}).Element())
	query := scene.Query{DetailLimit: 1}
	for _, key := range []string{"first", "second"} {
		page, err := r.Observe(query)
		if err != nil || len(page.Nodes) != 1 || page.Nodes[0].Key != key || len(page.Nodes[0].Actions) != 1 {
			t.Fatalf("detail page for %s = %+v, %v", key, page, err)
		}
		query.Cursor = page.Next
	}
	if query.Cursor != (scene.Cursor{}) {
		t.Fatal("detail pagination did not finish")
	}
}

func TestObservationRejectsNegativePayloadBudgets(t *testing.T) {
	r := root(t, scene.NewElement(scene.ElementConfig{Key: "node"}))
	for _, query := range []scene.Query{{ByteLimit: -1}, {DetailLimit: -1}} {
		if _, err := r.Observe(query); !errors.Is(err, scene.ErrInvalid) {
			t.Fatalf("Observe(%+v) = %v", query, err)
		}
	}
}

func observationAction(id scene.ActionID) *scene.Action[struct{}, struct{}] {
	return scene.NewAction(scene.ActionConfig[struct{}, struct{}]{
		ID: id,
		Run: func(context.Context, struct{}) (scene.ActionResult[struct{}], error) {
			return scene.ActionResult[struct{}]{}, nil
		},
	})
}
