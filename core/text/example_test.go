package text_test

import (
	"fmt"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

func ExampleDecode() {
	// What a command wrote, colour and all. The style an interface chose is what it
	// arrives in, and the sequences in it say the rest.
	body := grid.Style{FG: grid.RGBColor(0xE2, 0xE6, 0xEF)}
	for _, line := range text.Decode("ok \x1b[32mpassed\x1b[0m\nnext", body) {
		for _, span := range line {
			fmt.Printf("%q %v\n", span.Text, span.Style.FG.RGB())
		}
	}

	// Output:
	// "ok " {226 230 239}
	// "passed" {0 128 0}
	// "next" {226 230 239}
}

func ExampleDecoder() {
	// Output arrives in whatever pieces a read produced — here with a colour opened
	// in one and closed in another, and a sequence split down the middle.
	var d text.Decoder
	for _, chunk := range []string{"building \x1b[3", "3mtwo\x1b[0m targets\nlin", "king\n"} {
		for _, line := range d.Feed(chunk) {
			fmt.Printf("%q\n", line.String())
		}
	}
	fmt.Printf("still open: %q\n", d.Open().String())

	// Output:
	// "building two targets"
	// "linking"
	// still open: ""
}

func ExamplePrintable() {
	fmt.Printf("%q\n", text.Printable("name\tvalue\x00\xff"))
	// Output: "name\tvalue�"
}
