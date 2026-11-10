package base_test

import (
	"errors"
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestFieldValidationReasonsReachSemanticObservation(t *testing.T) {
	problem := errors.New("answer is prohibited by policy")
	fields := []struct {
		name  string
		field base.Field
	}{
		{"text", base.NewText(base.TextConfig{Check: func(string) error { return problem }})},
		{"confirm", base.NewConfirm(base.ConfirmConfig{Check: func(bool) error { return problem }})},
		{"select", base.NewSelect(base.SelectConfig[string]{Options: base.Options("one"), Same: base.Equal[string], Check: func(string) error { return problem }})},
		{"multiselect", base.NewMultiSelect(base.MultiSelectConfig[string]{Options: base.Options("one"), Same: base.Equal[string], Check: func([]string) error { return problem }})},
	}
	for _, test := range fields {
		t.Run(test.name, func(t *testing.T) {
			root := semanticOwner(t, test.field.Element())
			if err := test.field.Validate(); !errors.Is(err, problem) {
				t.Fatal(err)
			}
			node := observedNode(t, root, test.field.Element())
			if !node.State.Has(scene.StateInvalid) || node.Description != problem.Error() {
				t.Fatalf("validation feedback = %+v", node.Semantic)
			}
		})
	}
}

func TestTextProjectsValidationAndPasteFeedbackWithoutChangingTheirOwners(t *testing.T) {
	validation := errors.New("answer is required")
	clipboard := errors.New("clipboard unavailable")
	transport := &queuedClipboard{}
	field := base.NewText(base.TextConfig{
		Label: "Password", Initial: "secret", Clipboard: transport,
		Check: func(string) error { return validation },
	})
	field.SetMask("*")
	root := semanticOwner(t, field.Element())
	if err := field.Validate(); !errors.Is(err, validation) {
		t.Fatal(err)
	}
	field.Do(base.Paste)
	transport.receivers[0]("", clipboard)
	node := observedNode(t, root, field.Element())
	if !node.State.Has(scene.StateInvalid) || node.Description != validation.Error()+"\n"+clipboard.Error() || !node.Sensitive || node.Value != "" {
		t.Fatalf("combined feedback or sensitive value = %+v", node.Semantic)
	}
	if !errors.Is(field.Error(), validation) || errors.Is(field.Error(), clipboard) || !errors.Is(field.PasteError(), clipboard) {
		t.Fatal("presentation merged the independent error owners")
	}
	if height := field.HeightForWidth(60); height != 3 {
		t.Fatalf("field height = %d, want label, value and one feedback row", height)
	}
	surface := grid.NewSurface(60, 3)
	frame, err := root.Prepare(surface.View())
	if err != nil {
		t.Fatal(err)
	}
	frame.Commit(1)
	if rows := surface.Rows(); !slices.Equal(rows, []string{"Password", "******", validation.Error() + "; " + clipboard.Error()}) {
		t.Fatalf("feedback rows = %q", rows)
	}
	before := field.Cursor()
	if root.Handle(input.Mouse{Pos: image.Pt(1, 2), Action: input.MouseDown, Button: input.ButtonLeft}) || field.Cursor() != before {
		t.Fatal("feedback row was treated as an edit position")
	}
	field.SetCheck(nil)
	field.Do(base.Paste)
	transport.receivers[1]("", nil)
	node = observedNode(t, root, field.Element())
	if field.HeightForWidth(60) != 2 || node.State.Has(scene.StateInvalid) || node.Description != "" {
		t.Fatalf("cleared owners left feedback behind: %+v", node.Semantic)
	}
}

func TestEditorPasteFeedbackHasItsOwnNoneditableRow(t *testing.T) {
	problem := errors.New("clipboard unavailable")
	transport := &queuedClipboard{}
	editor := base.NewEditor(base.EditorConfig{Initial: "abc", MaxRows: 1, Clipboard: transport})
	root := semanticOwner(t, editor.Element())
	editor.Do(base.Paste)
	transport.receivers[0]("", problem)
	if height := editor.HeightForWidth(40); height != 2 {
		t.Fatalf("height = %d, want content cap plus feedback", height)
	}
	surface := grid.NewSurface(40, 2)
	frame, err := root.Prepare(surface.View())
	if err != nil {
		t.Fatal(err)
	}
	frame.Commit(1)
	if !slices.Equal(surface.Rows(), []string{"abc", problem.Error()}) {
		t.Fatalf("editor feedback rows = %q", surface.Rows())
	}
	line, offset := editor.Cursor()
	if root.Handle(input.Mouse{Pos: image.Pt(1, 1), Action: input.MouseDown, Button: input.ButtonLeft}) {
		t.Fatal("feedback row accepted editor input")
	}
	if afterLine, afterOffset := editor.Cursor(); afterLine != line || afterOffset != offset {
		t.Fatal("feedback click changed the caret")
	}
	node := observedNode(t, root, editor.Element())
	if !node.State.Has(scene.StateInvalid) || node.Description != problem.Error() || node.Value != "abc" {
		t.Fatalf("editor feedback = %+v", node.Semantic)
	}
}

func TestOperationFeedbackDoesNotBecomeAFieldValidationFailure(t *testing.T) {
	transport := &clip{refuse: true}
	field := base.NewText(base.TextConfig{Initial: "valid", Clipboard: transport})
	field.Do(base.Paste)
	form := base.NewForm(base.FormConfig{Fields: []base.Field{field}})
	result, err := form.Submit()
	if err != nil || result.State != scene.Applied || field.Error() != nil || !errors.Is(field.PasteError(), errors.ErrUnsupported) {
		t.Fatalf("paste feedback contaminated validation: result=%+v, form=%v, field=%v, paste=%v", result, err, field.Error(), field.PasteError())
	}
}

func TestConfirmProjectsBothValidationAndDomainFailure(t *testing.T) {
	validation, operation := errors.New("answer required"), errors.New("domain refused")
	domain := &confirmDomain{reject: true, problem: operation}
	field := base.NewConfirm(base.ConfirmConfig{Control: domain, Check: func(bool) error { return validation }})
	t.Cleanup(field.Close)
	root := semanticOwner(t, field.Element())
	if err := field.Validate(); !errors.Is(err, validation) {
		t.Fatal(err)
	}
	field.Say(true)
	node := observedNode(t, root, field.Element())
	if !node.State.Has(scene.StateInvalid) || !strings.Contains(node.Description, validation.Error()) || !strings.Contains(node.Description, operation.Error()) {
		t.Fatalf("confirmation feedback = %+v", node.Semantic)
	}
	if field.HeightForWidth(40) != 2 || !errors.Is(field.Error(), validation) || !errors.Is(field.ChangeError(), operation) {
		t.Fatal("confirmation feedback changed its validation or layout contract")
	}
}

func observedNode(t *testing.T, root *scene.Root, element scene.Element) scene.SemanticNode {
	t.Helper()
	observation, err := root.Observe(scene.Query{})
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range observation.Nodes {
		if node.Target == element.Target() {
			return node
		}
	}
	t.Fatalf("element %v absent from observation", element.Target())
	return scene.SemanticNode{}
}
