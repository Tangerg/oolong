package base_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

type callbackClipboard struct{ copy func(string) error }

func (c callbackClipboard) Copy(text string) error { return c.copy(text) }
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
			e.SetClipboard(callbackClipboard{copy: func(text string) error {
				copied = text
				if change == "content" {
					e.SetText("replacement")
					e.SelectAll()
				} else {
					e.SetCursor(base.Caret{Byte: len("first")})
				}
				return nil
			}})
			if err := e.Cut(); err != nil {
				t.Fatal(err)
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
