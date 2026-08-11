package latex_test

import (
	"fmt"

	"github.com/Tangerg/oolong/core/text"
	"github.com/Tangerg/oolong/latex"
)

func ExampleRender() {
	formula := latex.Render(`E = mc^2`, latex.Look{})
	for _, row := range formula.Rows(20) {
		fmt.Printf("%*s%s\n", row.Offset, "", row.Text)
	}

	// Output:
	//      2
	// E = mc
}

func ExampleRender_observeErrors() {
	failures := 0
	render := func(_ string, source string) []text.Line {
		formula := latex.Render(source, latex.Look{})
		if formula.Err() != nil {
			failures++
		}
		return formula.Lines()
	}

	lines := render("", `\frac{`)
	fmt.Println(lines[0].String())
	fmt.Println("failures:", failures)

	// Output:
	// \frac{
	// failures: 1
}
