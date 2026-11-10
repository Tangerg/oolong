package text_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/core/text"
)

func TestNormalizeLineEndings(t *testing.T) {
	if got := text.NormalizeLineEndings("a\r\nb\rc\nd\r\r\n"); got != "a\nb\nc\nd\n\n" {
		t.Fatalf("normalized = %q", got)
	}
}
