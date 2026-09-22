package markdown_test

import (
	"fmt"

	"github.com/Tangerg/oolong/markdown"
)

func ExampleStream() {
	// An answer arriving a few words at a time. What is certainly finished comes back
	// from Feed and is never looked at again; what is still being written is
	// re-rendered as often as anybody asks.
	var stream markdown.Stream
	stream.SetLook(markdown.Look{Glyphs: markdown.Glyphs{Bullet: "•"}})

	var doc markdown.Doc
	for _, chunk := range []string{"Two things:\n\n- th", "e first\n- the second\n\nAnd a l", "ast word."} {
		blocks, err := stream.Feed(chunk)
		if err != nil {
			fmt.Println(err)
			return
		}
		doc.Append(blocks...)
		open, err := stream.Open()
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Printf("published %d, open %d\n", doc.Len(), len(open))
	}
	blocks, err := stream.Flush()
	if err != nil {
		fmt.Println(err)
		return
	}
	doc.Append(blocks...)
	fmt.Printf("published %d, %d rows at 20 columns\n", doc.Len(), doc.HeightForWidth(20))

	// Output:
	// published 0, open 2
	// published 1, open 3
	// published 1, open 3
	// published 4, 6 rows at 20 columns
}
