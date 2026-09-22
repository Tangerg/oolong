package latex_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/latex"
)

func TestScriptValidationAndTokenization(t *testing.T) {
	for _, source := range []string{"x^", "x_", `\hat{x}`, "x" + strings.Repeat("^\t", 1024) + "a"} {
		if latex.Render(source, latex.Look{}).Err() == nil {
			t.Fatalf("accepted unsupported formula %q", source)
		}
	}
	for _, pair := range [][2]string{{"x^23", "x^{2}3"}, {"x_12", "x_{1}2"}, {"x^abc", "x^{a}bc"}} {
		a, b := latex.Render(pair[0], latex.Look{}), latex.Render(pair[1], latex.Look{})
		if a.Err() != nil || b.Err() != nil {
			t.Fatalf("valid scripts: %v, %v", a.Err(), b.Err())
		}
		if !slices.Equal(formulaRows(t, a, 40), formulaRows(t, b, 40)) {
			t.Fatalf("%q != %q", pair[0], pair[1])
		}
	}
}
