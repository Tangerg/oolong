package highlight_test

import (
	"fmt"

	"github.com/Tangerg/oolong/highlight"
)

func ExampleRenderer_Lines() {
	renderer := highlight.New("github-dark")
	lines := renderer.Lines("go", "for range 3 {\n\tfmt.Println(\"hello\")\n}")
	for _, line := range lines {
		fmt.Println(line.String())
	}

	// Output:
	// for range 3 {
	// 	fmt.Println("hello")
	// }
}
