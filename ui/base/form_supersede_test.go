package base_test

import (
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
)

// A newer submission revokes the pending one, which Settle can then no longer
// reach. Its owner used to be told nothing, so its work kept running unseen.
func TestASupersededSubmissionIsCancelled(t *testing.T) {
	var started, cancelled []base.SubmissionID
	form := base.NewForm(base.FormConfig{
		Fields: []base.Field{base.NewText(base.TextConfig{})},
		Submit: func(id base.SubmissionID) (scene.ActionState, error) {
			started = append(started, id)
			return scene.Started, nil
		},
		Cancel: func(id base.SubmissionID) error {
			cancelled = append(cancelled, id)
			return nil
		},
	})
	semanticOwner(t, form.Element())
	if _, err := form.Submit(); err != nil {
		t.Fatal(err)
	}
	if _, err := form.Submit(); err != nil {
		t.Fatal(err)
	}
	if len(started) != 2 || !slices.Equal(cancelled, started[:1]) {
		t.Fatalf("started %v, cancelled %v: want the first attempt cancelled", started, cancelled)
	}
	form.Set(base.NewText(base.TextConfig{}))
	if !slices.Equal(cancelled, started) {
		t.Fatalf("replacing the fields left the second attempt running: cancelled %v", cancelled)
	}
}
