package kit_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

func TestAskSubmitsOnlyTheQuestionsAndAnswersItCollected(t *testing.T) {
	for _, mode := range []string{"replace fields", "edit previous answer"} {
		t.Run(mode, func(t *testing.T) {
			var form *base.Form
			submitted := false
			first := base.NewText(base.TextConfig{})
			second := base.NewText(base.TextConfig{Check: func(string) error {
				if mode == "replace fields" {
					form.Set(base.NewText(base.TextConfig{Initial: "never asked"}))
				} else {
					first.SetText("never answered")
				}
				return nil
			}})
			form = base.NewForm(base.FormConfig{Fields: []base.Field{first, second}, Submit: func(base.SubmissionID) (scene.ActionState, error) {
				submitted = true
				return scene.Applied, nil
			}})
			_, err := kit.Ask(form, strings.NewReader("one\ntwo\n"), io.Discard)
			if want := map[string]error{"replace fields": base.ErrSubmissionStale, "edit previous answer": base.ErrValidationSuperseded}[mode]; !errors.Is(err, want) || submitted {
				t.Fatalf("err=%v want=%v submitted=%t", err, want, submitted)
			}
		})
	}
}

func TestAskWritesPromptsAndValidationErrorsAsTerminalText(t *testing.T) {
	field := base.NewText(base.TextConfig{Label: "Name\x1b[2J\nrequired", Check: func(answer string) error {
		if answer == "" {
			return errors.New("bad\x1b]0;title\abell")
		}
		return nil
	}})
	form := base.NewForm(base.FormConfig{Fields: []base.Field{field}})
	var out strings.Builder
	result, err := kit.Ask(form, strings.NewReader("\nok\n"), &out)
	if err != nil || result.State != scene.Applied {
		t.Fatalf("state=%v err=%v", result.State, err)
	}
	if got, want := out.String(), "Name[2J\nrequired\n> bad]0;titlebell\nName[2J\nrequired\n> "; got != want {
		t.Fatalf("spoken form output=%q, want %q", got, want)
	}
}
