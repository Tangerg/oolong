package kit_test

import (
	"bufio"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

func newPrompter(t *testing.T, cfg kit.PrompterConfig) *kit.Prompter {
	t.Helper()
	prompter, err := kit.NewPrompter(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return prompter
}

type spokenEffectOwner struct {
	snapshot base.ConfirmSnapshot
	outcome  scene.ActionState
	problem  error
	notify   func()
	pending  *base.ConfirmRequest
	requests int
}

func (o *spokenEffectOwner) Snapshot() base.ConfirmSnapshot { return o.snapshot }

func (o *spokenEffectOwner) Subscribe(notify func()) func() {
	o.notify = notify
	return func() { o.notify = nil }
}

func (o *spokenEffectOwner) Change(_ context.Context, request base.ConfirmRequest) (scene.ActionResult[base.ConfirmSnapshot], error) {
	o.requests++
	if o.outcome == scene.Started {
		o.pending = &request
	} else {
		o.snapshot = base.ConfirmSnapshot{Value: request.Value, Revision: o.snapshot.Revision + 1}
		if o.notify != nil {
			o.notify()
		}
	}
	return scene.ActionResult[base.ConfirmSnapshot]{State: o.outcome, Value: o.snapshot}, o.problem
}

func TestPrompterStopsAfterAPartiallyAcceptedDomainFailure(t *testing.T) {
	problem := errors.New("accepted change has a later failure")
	for _, outcome := range []scene.ActionState{scene.Applied, scene.Started} {
		owner := &spokenEffectOwner{outcome: outcome, problem: problem}
		field := base.NewConfirm(base.ConfirmConfig{Control: owner})
		t.Cleanup(field.Close)
		prompter := newPrompter(t, kit.PrompterConfig{Input: strings.NewReader("yes\nnext\n"), Output: io.Discard})
		form := base.NewForm(base.FormConfig{Fields: []base.Field{field}, Submit: func(base.SubmissionID) (scene.ActionState, error) {
			t.Error("partial domain failure submitted the form")
			return scene.Applied, nil
		}})
		if _, err := prompter.Ask(form); !errors.Is(err, base.ErrReplyUnavailable) || !errors.Is(err, problem) || owner.requests != 1 {
			t.Fatalf("domain failure was retried or lost: %v, requests=%d", err, owner.requests)
		}
		if field.Answer() != (outcome == scene.Applied) || (owner.pending != nil) != (outcome == scene.Started) {
			t.Fatal("failed spoken collection rolled back accepted domain effects")
		}
		next := base.NewText(base.TextConfig{})
		if _, err := prompter.Ask(base.NewForm(base.FormConfig{Fields: []base.Field{next}})); err != nil || next.Text() != "next" {
			t.Fatalf("domain failure consumed the next answer: text=%q, error=%v", next.Text(), err)
		}
	}
}

func TestPrompterRetainsInputAcrossForms(t *testing.T) {
	prompter := newPrompter(t, kit.PrompterConfig{Input: strings.NewReader("first\nsecond\n"), Output: io.Discard})
	for _, want := range []string{"first", "second"} {
		field := base.NewText(base.TextConfig{})
		form := base.NewForm(base.FormConfig{Fields: []base.Field{field}})
		result, err := prompter.Ask(form)
		if err != nil || result.State != scene.Applied || field.Text() != want {
			t.Fatalf("answer=%q result=%v err=%v, want %q", field.Text(), result, err, want)
		}
	}
}

func TestPrompterHasNoImplicitScannerBudget(t *testing.T) {
	answer := strings.Repeat("x", 64<<10)
	field := base.NewText(base.TextConfig{})
	if err := field.Reply(answer); err != nil {
		t.Fatal(err)
	}
	field.SetText("")
	prompter := newPrompter(t, kit.PrompterConfig{Input: strings.NewReader(answer + "\n"), Output: io.Discard})
	result, err := prompter.Ask(base.NewForm(base.FormConfig{Fields: []base.Field{field}}))
	if err != nil || result.State != scene.Applied || field.Text() != answer {
		t.Fatalf("length=%d state=%v err=%v", len(field.Text()), result.State, err)
	}
}

func TestPrompterCountsAnswersWithoutLineEndings(t *testing.T) {
	bufferBytes := bufio.NewReader(strings.NewReader("")).Size()
	for _, size := range []int{0, 1, bufferBytes - 1, bufferBytes, bufferBytes + 1} {
		for _, ending := range []string{"\n", "\r\n", "", "\r"} {
			if size == 0 && ending == "" {
				continue
			}
			answer := strings.Repeat("x", size)
			field := base.NewText(base.TextConfig{})
			prompter := newPrompter(t, kit.PrompterConfig{
				Input: strings.NewReader(answer + ending), Output: io.Discard, MaxAnswerBytes: max(1, size),
			})
			result, err := prompter.Ask(base.NewForm(base.FormConfig{Fields: []base.Field{field}}))
			if err != nil || result.State != scene.Applied || field.Text() != answer {
				t.Errorf("size=%d ending=%q: length=%d state=%v err=%v", size, ending, len(field.Text()), result.State, err)
			}
		}
	}
}

func TestPrompterDiscardsOneOversizedAnswerAndPreservesTheNext(t *testing.T) {
	for _, ending := range []string{"\n", "\r\n"} {
		input := strings.Repeat("x", 64<<10) + ending + "next\n"
		prompter := newPrompter(t, kit.PrompterConfig{Input: strings.NewReader(input), Output: io.Discard, MaxAnswerBytes: 4})
		field := base.NewText(base.TextConfig{Initial: "untouched"})
		form := base.NewForm(base.FormConfig{Fields: []base.Field{field}})
		if _, err := prompter.Ask(form); !errors.Is(err, kit.ErrAnswerTooLong) {
			t.Fatalf("oversized answer err=%v", err)
		}
		if field.Text() != "untouched" {
			t.Fatal("oversized answer reached the field")
		}
		result, err := prompter.Ask(form)
		if err != nil || result.State != scene.Applied || field.Text() != "next" {
			t.Fatalf("following answer=%q state=%v err=%v", field.Text(), result.State, err)
		}
	}
}

func TestPrompterLimitsBytesBeforeFieldNormalization(t *testing.T) {
	for _, test := range []struct {
		name   string
		answer string
		limit  int
		fits   bool
	}{
		{"exact UTF-8 bytes", "界", 3, true},
		{"split UTF-8 budget", "界", 2, false},
		{"combining cluster", "a\u0301", 2, false},
		{"internal carriage return", "a\rb", 2, false},
		{"discarded control", "a\x00b", 2, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, ending := range []string{"\n", "\r\n", ""} {
				field := base.NewText(base.TextConfig{Initial: "untouched"})
				prompter := newPrompter(t, kit.PrompterConfig{
					Input: iotest.OneByteReader(strings.NewReader(test.answer + ending)), Output: io.Discard, MaxAnswerBytes: test.limit,
				})
				result, err := prompter.Ask(base.NewForm(base.FormConfig{Fields: []base.Field{field}}))
				if test.fits {
					if err != nil || result.State != scene.Applied || field.Text() != test.answer {
						t.Fatalf("ending=%q answer=%q state=%v err=%v", ending, field.Text(), result.State, err)
					}
				} else if !errors.Is(err, kit.ErrAnswerTooLong) || field.Text() != "untouched" {
					t.Fatalf("ending=%q answer=%q err=%v", ending, field.Text(), err)
				}
			}
		})
	}
}

type promptWriteFunc func([]byte) (int, error)

func (f promptWriteFunc) Write(p []byte) (int, error) { return f(p) }

func TestPrompterDoesNotReadAnAnswerAfterPromptOutputFails(t *testing.T) {
	for _, failedText := range []string{"Name", "Required", "> "} {
		t.Run(failedText, func(t *testing.T) {
			failure := errors.New("output failed")
			fail := true
			output := promptWriteFunc(func(p []byte) (int, error) {
				if fail && strings.Contains(string(p), failedText) {
					return 0, failure
				}
				return len(p), nil
			})
			input := strings.NewReader("answer\n")
			prompter := newPrompter(t, kit.PrompterConfig{Input: input, Output: output})
			field := base.NewText(base.TextConfig{Label: "Name\nRequired", Initial: "untouched"})
			form := base.NewForm(base.FormConfig{Fields: []base.Field{field}})
			if _, err := prompter.Ask(form); !errors.Is(err, failure) {
				t.Fatalf("output error=%v", err)
			}
			if field.Text() != "untouched" || input.Len() != len("answer\n") {
				t.Fatalf("failed prompt consumed input: field=%q remaining=%d", field.Text(), input.Len())
			}
			fail = false
			result, err := prompter.Ask(form)
			if err != nil || result.State != scene.Applied || field.Text() != "answer" {
				t.Fatalf("recovered output: answer=%q state=%v err=%v", field.Text(), result.State, err)
			}
		})
	}
}

func TestPrompterRefusesShortPromptWritesBeforeReadingAnAnswer(t *testing.T) {
	for _, shortText := range []string{"Name", "Required", "> "} {
		t.Run(shortText, func(t *testing.T) {
			output := promptWriteFunc(func(p []byte) (int, error) {
				if strings.Contains(string(p), shortText) {
					return len(p) - 1, nil
				}
				return len(p), nil
			})
			input := strings.NewReader("answer\n")
			prompter := newPrompter(t, kit.PrompterConfig{Input: input, Output: output})
			field := base.NewText(base.TextConfig{Label: "Name\nRequired", Initial: "untouched"})
			form := base.NewForm(base.FormConfig{Fields: []base.Field{field}})
			if _, err := prompter.Ask(form); !errors.Is(err, io.ErrShortWrite) {
				t.Fatalf("short prompt write = %v, want io.ErrShortWrite", err)
			}
			if field.Text() != "untouched" || input.Len() != len("answer\n") {
				t.Fatalf("incomplete prompt consumed input: field=%q remaining=%d", field.Text(), input.Len())
			}
		})
	}
}

func TestPrompterDoesNotRetryAfterShortValidationOutput(t *testing.T) {
	output := promptWriteFunc(func(p []byte) (int, error) {
		if strings.Contains(string(p), "invalid") {
			return 0, nil
		}
		return len(p), nil
	})
	field := base.NewText(base.TextConfig{Check: func(answer string) error {
		if answer == "bad" {
			return errors.New("invalid answer")
		}
		return nil
	}})
	form := base.NewForm(base.FormConfig{Fields: []base.Field{field}})
	prompter := newPrompter(t, kit.PrompterConfig{Input: strings.NewReader("bad\ngood\n"), Output: output})
	if _, err := prompter.Ask(form); !errors.Is(err, io.ErrShortWrite) || field.Text() != "bad" {
		t.Fatalf("short validation output: answer=%q err=%v", field.Text(), err)
	}
	result, err := prompter.Ask(form)
	if err != nil || result.State != scene.Applied || field.Text() != "good" {
		t.Fatalf("following answer=%q state=%v err=%v", field.Text(), result.State, err)
	}
}

func TestPrompterPreservesTheNextAnswerWhenValidationOutputFails(t *testing.T) {
	failure := errors.New("output failed")
	output := promptWriteFunc(func(p []byte) (int, error) {
		if strings.Contains(string(p), "invalid") {
			return 0, failure
		}
		return len(p), nil
	})
	field := base.NewText(base.TextConfig{Check: func(answer string) error {
		if answer == "bad" {
			return errors.New("invalid answer")
		}
		return nil
	}})
	form := base.NewForm(base.FormConfig{Fields: []base.Field{field}})
	prompter := newPrompter(t, kit.PrompterConfig{Input: strings.NewReader("bad\ngood\n"), Output: output})
	if _, err := prompter.Ask(form); !errors.Is(err, failure) || field.Text() != "bad" {
		t.Fatalf("rejected answer=%q err=%v", field.Text(), err)
	}
	result, err := prompter.Ask(form)
	if err != nil || result.State != scene.Applied || field.Text() != "good" {
		t.Fatalf("following answer=%q state=%v err=%v", field.Text(), result.State, err)
	}
}

func TestPrompterCanContinueAfterAValidationPanic(t *testing.T) {
	const failure = "validation panicked"
	first := base.NewText(base.TextConfig{Check: func(string) error { panic(failure) }})
	second := base.NewText(base.TextConfig{})
	prompter := newPrompter(t, kit.PrompterConfig{Input: strings.NewReader("first\nsecond\n"), Output: io.Discard})
	func() {
		defer func() {
			if got := recover(); got != failure {
				t.Fatalf("panic=%v, want %v", got, failure)
			}
		}()
		_, _ = prompter.Ask(base.NewForm(base.FormConfig{Fields: []base.Field{first}}))
	}()
	result, err := prompter.Ask(base.NewForm(base.FormConfig{Fields: []base.Field{second}}))
	if err != nil || result.State != scene.Applied || second.Text() != "second" {
		t.Fatalf("following answer=%q state=%v err=%v", second.Text(), result.State, err)
	}
}

type promptReadFailure struct{ err error }

func (r promptReadFailure) Read([]byte) (int, error) { return 0, r.err }

func TestPrompterPreservesInputFailureWithoutSubmittingPartialAnswers(t *testing.T) {
	failure := errors.New("input failed")
	for _, limit := range []int{0, 2} {
		field := base.NewText(base.TextConfig{Initial: "untouched"})
		prompter := newPrompter(t, kit.PrompterConfig{
			Input: io.MultiReader(strings.NewReader("partial"), promptReadFailure{failure}), Output: io.Discard, MaxAnswerBytes: limit,
		})
		_, err := prompter.Ask(base.NewForm(base.FormConfig{Fields: []base.Field{field}}))
		if !errors.Is(err, failure) || field.Text() != "untouched" {
			t.Fatalf("limit=%d err=%v answer=%q", limit, err, field.Text())
		}
		if limit != 0 && !errors.Is(err, kit.ErrAnswerTooLong) {
			t.Fatalf("input failure hid the exceeded budget: %v", err)
		}
	}
}

func TestPrompterRejectsReentrantQuestionCollection(t *testing.T) {
	prompter := newPrompter(t, kit.PrompterConfig{Input: strings.NewReader("first\nsecond\n"), Output: io.Discard})
	second := base.NewText(base.TextConfig{})
	secondForm := base.NewForm(base.FormConfig{Fields: []base.Field{second}})
	first := base.NewText(base.TextConfig{Check: func(string) error {
		_, err := prompter.Ask(secondForm)
		if !errors.Is(err, kit.ErrPromptActive) {
			t.Errorf("nested collection err=%v", err)
		}
		return nil
	}})
	if _, err := prompter.Ask(base.NewForm(base.FormConfig{Fields: []base.Field{first}})); err != nil {
		t.Fatal(err)
	}
	if second.Text() != "" {
		t.Fatal("reentrant call consumed another form's answer")
	}
	if _, err := prompter.Ask(secondForm); err != nil || second.Text() != "second" {
		t.Fatalf("following collection answer=%q err=%v", second.Text(), err)
	}
}

func TestPrompterValidatesItsInputBoundary(t *testing.T) {
	var absent *strings.Reader
	var absentOutput *strings.Builder
	for _, cfg := range []kit.PrompterConfig{
		{Output: io.Discard},
		{Input: strings.NewReader("")},
		{Input: absent, Output: io.Discard},
		{Input: strings.NewReader(""), Output: absentOutput},
		{Input: strings.NewReader(""), Output: io.Discard, MaxAnswerBytes: -1},
	} {
		if prompter, err := kit.NewPrompter(cfg); err == nil || prompter != nil {
			t.Fatalf("invalid construction returned %v, %v", prompter, err)
		}
	}
	for _, prompter := range []*kit.Prompter{nil, {}} {
		if _, err := prompter.Ask(nil); err == nil {
			t.Fatal("uninitialized prompter accepted a call")
		}
	}
	field := base.NewText(base.TextConfig{})
	prompter := newPrompter(t, kit.PrompterConfig{Input: strings.NewReader(""), Output: io.Discard})
	if _, err := prompter.Ask(nil); err == nil {
		t.Fatal("a missing form was reported as an unchanged submission")
	}
	if _, err := prompter.Ask(base.NewForm(base.FormConfig{Fields: []base.Field{field}})); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("unanswered field err=%v", err)
	}
}
