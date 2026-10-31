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
			form = base.NewForm(base.FormConfig{Fields: []base.Field{first, second}, Submit: func(id base.SubmissionID) (scene.ActionResult[base.SubmissionID], error) {
				submitted = true
				return scene.ActionResult[base.SubmissionID]{State: scene.Applied, Value: id}, nil
			}})
			_, err := kit.Ask(form, strings.NewReader("one\ntwo\n"), io.Discard)
			if want := map[string]error{"replace fields": base.ErrSubmissionStale, "edit previous answer": base.ErrValidationSuperseded}[mode]; !errors.Is(err, want) || submitted {
				t.Fatalf("err=%v want=%v submitted=%t", err, want, submitted)
			}
		})
	}
}
