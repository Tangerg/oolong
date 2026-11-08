package base_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/internal/scenetest"
)

type validationPolicyField interface {
	base.Field
	InvalidateValidation()
}

func fieldWithPolicy(kind string) (validationPolicyField, func(func() error)) {
	switch kind {
	case "text":
		field := base.NewText(base.TextConfig{Initial: "answer"})
		return field, func(check func() error) { field.SetCheck(answerIndependentCheck[string](check)) }
	case "select":
		field := base.NewSelect(base.SelectConfig[string]{Same: base.Equal[string], Options: base.Options("answer")})
		return field, func(check func() error) { field.SetCheck(answerIndependentCheck[string](check)) }
	case "multi-select":
		field := base.NewMultiSelect(base.MultiSelectConfig[string]{Same: base.Equal[string], Options: base.Options("answer"), Initial: []string{"answer"}})
		return field, func(check func() error) { field.SetCheck(answerIndependentCheck[[]string](check)) }
	case "confirm":
		field := base.NewConfirm(base.ConfirmConfig{Initial: true})
		return field, func(check func() error) { field.SetCheck(answerIndependentCheck[bool](check)) }
	default:
		panic("unknown field type")
	}
}

func answerIndependentCheck[T any](check func() error) func(T) error {
	if check == nil {
		return nil
	}
	return func(T) error { return check() }
}

func TestFormCannotSubmitAfterValidationPolicyReplacement(t *testing.T) {
	for _, kind := range []string{"text", "select", "multi-select", "confirm"} {
		for _, phase := range []string{"later field", "whole form"} {
			t.Run(kind+"/"+phase, func(t *testing.T) {
				first, install := fieldWithPolicy(kind)
				denied := errors.New("replacement rule rejects this answer")
				before := first.Revision()
				replace := func() {
					receipt := first.Validation()
					if receipt == nil || !receipt() {
						t.Fatal("first check has no current receipt")
					}
					install(func() error { return denied })
					if receipt() || first.Validation() != nil || first.Error() != nil || first.Revision() != before {
						t.Fatal("policy replacement kept old validation or changed the answer")
					}
				}
				submitted := false
				config := base.FormConfig{
					Fields: []base.Field{first},
					Submit: func(base.SubmissionID) (scene.ActionState, error) {
						submitted = true
						return scene.Applied, nil
					},
				}
				if phase == "later field" {
					config.Fields = append(config.Fields, base.NewText(base.TextConfig{Check: func(string) error { replace(); return nil }}))
				} else {
					config.Check = func() error { replace(); return nil }
				}
				form := base.NewForm(config)
				if _, err := form.Submit(); submitted || !errors.Is(err, base.ErrValidationSuperseded) {
					t.Fatalf("obsolete policy submitted=%v error=%v", submitted, err)
				}
				if err := first.Validate(); !errors.Is(err, denied) || !errors.Is(first.Error(), denied) {
					t.Fatalf("replacement rule: result=%v field error=%v", err, first.Error())
				}
			})
		}
	}
}

func TestFieldPolicyChangesRevokeResultsWithoutRunningChecks(t *testing.T) {
	for _, kind := range []string{"text", "select", "multi-select", "confirm"} {
		t.Run(kind, func(t *testing.T) {
			field, install := fieldWithPolicy(kind)
			denied := errors.New("dependent rule rejects the answer")
			allowed, calls := true, 0
			check := func() error {
				calls++
				if !allowed {
					return denied
				}
				return nil
			}
			install(check)
			if calls != 0 {
				t.Fatal("installing a policy executed it")
			}
			if err := field.Validate(); err != nil {
				t.Fatal(err)
			}
			receipt, before := field.Validation(), field.Revision()
			allowed = false
			field.InvalidateValidation()
			if calls != 1 || receipt() || field.Validation() != nil || field.Revision() != before {
				t.Fatal("dependency invalidation reran a check or kept its receipt")
			}
			if err := field.Validate(); !errors.Is(err, denied) {
				t.Fatalf("updated dependency was not checked: %v", err)
			}
			receipt = field.Validation()
			install(check)
			if calls != 2 || receipt() || field.Error() != nil || field.Validation() != nil || field.Revision() != before {
				t.Fatal("reinstalling a rule kept a receipt or error from its previous installation")
			}
			install(nil)
			if err := field.Validate(); err != nil || field.Error() != nil || calls != 2 {
				t.Fatalf("removing a rule did not disable its checks: error=%v calls=%d", err, calls)
			}
		})
	}
}

func TestFieldPolicyReplacementDuringCheckCannotPublish(t *testing.T) {
	for _, kind := range []string{"text", "select", "multi-select", "confirm"} {
		t.Run(kind, func(t *testing.T) {
			field, install := fieldWithPolicy(kind)
			old, latest := errors.New("old rule"), errors.New("latest rule")
			install(func() error {
				install(func() error { return latest })
				return old
			})
			if err := field.Validate(); !errors.Is(err, base.ErrValidationSuperseded) || field.Error() != nil || field.Validation() != nil {
				t.Fatalf("obsolete check published: result=%v field error=%v", err, field.Error())
			}
			if err := field.Validate(); !errors.Is(err, latest) {
				t.Fatalf("replacement check: %v", err)
			}
		})
	}
}

func TestFieldPolicyCannotChangeDuringProjection(t *testing.T) {
	for _, kind := range []string{"text", "select", "multi-select", "confirm"} {
		for _, phase := range []string{"observation", "presentation"} {
			t.Run(kind+"/"+phase, func(t *testing.T) {
				field, install := fieldWithPolicy(kind)
				if err := field.Validate(); err != nil {
					t.Fatal(err)
				}
				receipt, before := field.Validation(), field.Revision()
				denied := errors.New("forbidden replacement")
				mutate := func() {
					for _, change := range []func(){func() { install(func() error { return denied }) }, field.InvalidateValidation} {
						func() {
							defer func() {
								if err, _ := recover().(error); !errors.Is(err, scene.ErrBusy) {
									t.Errorf("projection changed validation: %v", err)
								}
							}()
							change()
						}()
					}
				}
				if phase == "observation" {
					field.Element().ReadOnly(mutate)
				} else {
					if err := field.Element().SetPaint(func(frame scene.Frame) { mutate(); field.Draw(frame) }); err != nil {
						t.Fatal(err)
					}
					scenes := scenetest.New()
					t.Cleanup(scenes.Close)
					scenes.Mount(field).Draw(grid.NewSurface(20, 1).View())
				}
				if !receipt() || field.Revision() != before {
					t.Fatal("rejected mutation changed validation or answer")
				}
				if err := field.Validate(); err != nil {
					t.Fatalf("rejected mutation installed its policy: %v", err)
				}
			})
		}
	}
}
