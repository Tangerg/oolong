package latex_test

import (
	"fmt"

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
