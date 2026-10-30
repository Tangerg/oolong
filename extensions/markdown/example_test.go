package markdown_test

import (
	"context"
	"fmt"

	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/ui/core/content"
)

func ExampleNewRenderer() {
	document, err := content.NewStreamDocument(content.StreamConfig{Format: "markdown"})
	if err != nil {
		fmt.Println(err)
		return
	}
	renderer := markdown.NewRenderer(markdown.RendererConfig{})
	for _, chunk := range []string{"[a link][target]\n\n", "[target]: https://example.test\n"} {
		if err := document.Append([]byte(chunk)); err != nil {
			fmt.Println(err)
			return
		}
		if err := document.Accept(document.Snapshot().Prepare(context.Background(), renderer)); err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println("publishable:", document.Current().Publishable())
	}
	if err := document.End(nil); err != nil {
		fmt.Println(err)
		return
	}
	if err := document.Accept(document.Snapshot().Prepare(context.Background(), renderer)); err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("publishable:", document.Current().Publishable())
	// Output:
	// publishable: false
	// publishable: false
	// publishable: true
}
