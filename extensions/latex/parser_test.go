package latex_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/oolong/extensions/latex"
)

func TestCommentDepthCannotBypassTheParserBudget(t *testing.T) {
	source := strings.Repeat("{ %}\n", 300) + "x" + strings.Repeat("}", 300)
	if err := latex.Render(source, latex.Look{}).Err(); err == nil {
		t.Fatal("recursive input passed the entry guard")
	}
	for _, source := range []string{"{x %}^^[\n}", "x%unclosed {\n+y", `\{x\}`, "x^% comment\n2"} {
		if err := latex.Render(source, latex.Look{}).Err(); err != nil {
			t.Errorf("%q: %v", source, err)
		}
	}
}

func TestGroupLimitCannotBeDischargedInsideAnotherGroup(t *testing.T) {
	source := strings.Repeat(`\sqrt[{]}`, 300) + "x" + strings.Repeat(`]{x}`, 300)
	if err := latex.Render(source, latex.Look{}).Err(); err == nil {
		t.Fatal("a literal bracket in a brace group bypassed the parser depth limit")
	}
	if err := latex.Render(`\sqrt[{]}]{x}`, latex.Look{}).Err(); err != nil {
		t.Fatalf("valid grouped bracket: %v", err)
	}
}

func TestEscapedWhitespaceCannotConsumeTheFollowingBrace(t *testing.T) {
	for _, space := range []string{" ", "\t", "\n"} {
		source := strings.Repeat("{\\"+space+"}", 300) + "x"
		f := latex.Render(source, latex.Look{})
		if err := f.Err(); err != nil {
			t.Fatalf("adjacent groups with escaped %q: %v", space, err)
		}
		if got := f.Lines()[0].String(); got != strings.Repeat(" ", 300)+"x" {
			t.Fatalf("escaped %q consumed a brace: %q", space, got)
		}
	}
}
