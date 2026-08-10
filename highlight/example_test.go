package highlight_test

import (
	"fmt"

	"github.com/Tangerg/oolong/highlight"
)

func ExampleLines() {
	lines := highlight.Lines("go", "for range 3 {\n\tfmt.Println(\"hello\")\n}", "github-dark")
	for _, line := range lines {
		fmt.Println(line.String())
	}

	// Output:
	// for range 3 {
	// 	fmt.Println("hello")
	// }
}
