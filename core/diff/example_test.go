package diff_test

import (
	"fmt"
	"strings"

	"github.com/Tangerg/oolong/core/diff"
)

func ExampleBetween() {
	before := strings.Split("a\nb\nc", "\n")
	after := strings.Split("a\nB\nc", "\n")
	fmt.Print(diff.Between(before, after))

	// Output:
	//  a
	// -b
	// +B
	//  c
}

func ExampleScript_Hunks() {
	// A file that changed in one place is a hundred lines of which three matter, and
	// hunks are what is left once the rest is dropped.
	before := strings.Split("1\n2\n3\n4\n5\n6\n7", "\n")
	after := strings.Split("1\n2\n3\nX\n5\n6\n7", "\n")

	for _, hunk := range diff.Between(before, after).Hunks(1) {
		fmt.Printf("@@ -%d +%d @@\n%s", hunk.Old, hunk.New, hunk)
	}

	// Output:
	// @@ -3 +3 @@
	//  3
	// -4
	// +X
	//  5
}

func ExampleScript_Same() {
	same := strings.Split("a\nb", "\n")
	fmt.Println(diff.Between(same, same).Same())

	// Output:
	// true
}
