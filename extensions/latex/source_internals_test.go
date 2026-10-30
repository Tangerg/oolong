package latex

import (
	"strings"
	"testing"
)

func TestCommentDepthIsRejectedBeforeRecursiveParsing(t *testing.T) {
	source := strings.Repeat("{ %}\n", 300) + "x" + strings.Repeat("}", 300)
	if err := validateSource(withoutComments(source)); err == nil {
		t.Fatal("recursive input passed the entry guard")
	}
	if node, err := parse(source); err == nil || node != nil {
		t.Fatalf("node=%v err=%v", node, err)
	}
	for _, source := range []string{"{x %}^^[\n}", "x%unclosed {\n+y", `\{x\}`, "x^% comment\n2"} {
		if _, err := parse(source); err != nil {
			t.Errorf("%q: %v", source, err)
		}
	}
}
