package text_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/text"
)

func TestSingleLineTurnsEachLineEndingIntoOneSpace(t *testing.T) {
	for input, want := range map[string]string{
		"plain":             "plain",
		"one\ntwo":          "one two",
		"one\r\ntwo\rthree": "one two three",
		"tail\n":            "tail ",
		"\n\n":              "  ",
	} {
		if got := text.SingleLine(input); got != want {
			t.Errorf("SingleLine(%q) = %q, want %q", input, got, want)
		}
	}
}
