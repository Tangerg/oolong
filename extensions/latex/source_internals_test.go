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

func TestGroupLimitCannotBeDischargedInsideAnotherGroup(t *testing.T) {
	source := strings.Repeat(`\sqrt[{]}`, maxGroups+1) + "x" + strings.Repeat(`]{x}`, maxGroups+1)
	if node, err := parse(source); err == nil || node != nil {
		t.Fatal("a literal bracket in a brace group bypassed the parser depth limit")
	}
	if _, err := parse(`\sqrt[{]}]{x}`); err != nil {
		t.Fatalf("valid grouped bracket: %v", err)
	}
}

func TestEscapedWhitespaceCannotConsumeTheFollowingBrace(t *testing.T) {
	for _, space := range []string{" ", "\t", "\n"} {
		source := strings.Repeat("{\\"+space+"}", maxGroups+1) + "x"
		f := Render(source, Look{})
		if err := f.Err(); err != nil {
			t.Fatalf("adjacent groups with escaped %q: %v", space, err)
		}
		if got := f.Lines()[0].String(); got != strings.Repeat(" ", maxGroups+1)+"x" {
			t.Fatalf("escaped %q consumed a brace: %q", space, got)
		}
	}
}
