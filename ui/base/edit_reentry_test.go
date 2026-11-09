package base_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

type callbackClipboard struct{ copy func(string) bool }

func (c callbackClipboard) Copy(text string) bool { return c.copy(text) }
func (callbackClipboard) Read(context.Context, func(string, error)) error {
	return errors.ErrUnsupported
}

func TestCutCannotDeleteASelectionChangedDuringCopy(t *testing.T) {
	for _, change := range []string{"content", "selection"} {
		t.Run(change, func(t *testing.T) {
			var e base.Editor
			e.SetText("first second")
			e.SelectAll()
			copied := ""
			e.SetClipboard(callbackClipboard{copy: func(text string) bool {
				copied = text
				if change == "content" {
					e.SetText("replacement")
					e.SelectAll()
				} else {
					e.SetCursor(0, len("first"))
				}
				return true
			}})
			if e.Cut() {
				t.Fatal("cut deleted a selection that was not copied")
			}
			want := "first second"
			if change == "content" {
				want = "replacement"
			}
			if e.Text() != want || copied != "first second" {
				t.Fatalf("text=%q copied=%q", e.Text(), copied)
			}
		})
	}
}
