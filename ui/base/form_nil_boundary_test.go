package base_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type absentFormField struct{ base.Field }

func (*absentFormField) Element() scene.Element {
	return scene.NewElement(scene.ElementConfig{})
}

func TestFormRejectsTypedNilFieldAtConstruction(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != "base: nil form field" {
			t.Fatalf("typed nil field = %v, want collection-boundary refusal", recovered)
		}
	}()
	base.NewForm(base.FormConfig{Fields: []base.Field{(*absentFormField)(nil)}})
}

func TestFormRefusesTypedNilBeforeRevokingItsCurrentAttempt(t *testing.T) {
	field := base.NewText(base.TextConfig{Initial: "retained answer"})
	cancellations := 0
	form := base.NewForm(base.FormConfig{
		Fields: []base.Field{field},
		Submit: func(base.SubmissionID) (scene.ActionState, error) { return scene.Started, nil },
		Cancel: func(base.SubmissionID) error { cancellations++; return nil },
	})
	if _, err := form.Submit(); err != nil {
		t.Fatal(err)
	}
	before := form.Submission()
	func() {
		defer func() {
			if recovered := recover(); recovered != "base: nil form field" {
				t.Errorf("typed nil replacement = %v, want collection-boundary refusal", recovered)
			}
		}()
		form.Set((*absentFormField)(nil))
	}()
	if !slices.Equal(form.Fields(), []base.Field{field}) || form.Submission() != before || cancellations != 0 {
		t.Fatal("refused replacement changed the fields or pending submission")
	}
}

type absentConfirmControl struct{ base.ConfirmControl }

func (*absentConfirmControl) Subscribe(func()) func() {
	panic("absent control reached subscription")
}

func TestConfirmRejectsTypedNilControlBeforeSubscribing(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != "base: nil confirm control" {
			t.Fatalf("typed nil control = %v, want owner-boundary refusal", recovered)
		}
	}()
	base.NewConfirm(base.ConfirmConfig{Control: (*absentConfirmControl)(nil)})
}
