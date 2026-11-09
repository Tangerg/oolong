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
	for _, test := range []struct {
		name          string
		replaceFields bool
		wantErr       error
	}{
		{"replace fields", true, base.ErrSubmissionStale},
		{"edit previous answer", false, base.ErrValidationSuperseded},
	} {
		t.Run(test.name, func(t *testing.T) {
			var form *base.Form
			submitted := false
			first := base.NewText(base.TextConfig{})
			second := base.NewText(base.TextConfig{Check: func(string) error {
				if test.replaceFields {
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
			_, err := newPrompter(t, kit.PrompterConfig{Input: strings.NewReader("one\ntwo\n"), Output: io.Discard}).Ask(form)
			if !errors.Is(err, test.wantErr) || submitted {
				t.Fatalf("err=%v want=%v submitted=%t", err, test.wantErr, submitted)
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
	result, err := newPrompter(t, kit.PrompterConfig{Input: strings.NewReader("\nok\n"), Output: &out}).Ask(form)
	if err != nil || result.State != scene.Applied {
		t.Fatalf("state=%v err=%v", result.State, err)
	}
	if got, want := out.String(), "Name[2J\nrequired\n> bad]0;titlebell\nName[2J\nrequired\n> "; got != want {
		t.Fatalf("spoken form output=%q, want %q", got, want)
	}
}
