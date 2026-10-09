package markdown_test

import (
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/markdown"
)

func TestStreamRenderingHasOneMutationOwner(t *testing.T) {
	for _, outer := range []string{"Feed", "Open", "Flush"} {
		for _, inner := range []string{"Feed", "Open", "Flush", "Reset", "SetLook"} {
			t.Run(outer+"/"+inner, func(t *testing.T) {
				var stream markdown.Stream
				var attempted, refused bool
				var look markdown.Look
				look.SetRenderer(markdown.FencedCode, func(string, string) (grid.Drawable, error) {
					if !attempted {
						attempted = true
						func() {
							defer func() { refused = recover() != nil }()
							switch inner {
							case "Feed":
								_, _ = stream.Feed("replacement")
							case "Open":
								_, _ = stream.Open()
							case "Flush":
								_, _ = stream.Flush()
							case "Reset":
								stream.Reset()
							case "SetLook":
								stream.SetLook(markdown.Look{})
							}
						}()
					}
					return nil, markdown.ErrUnhandled
				})
				stream.SetLook(look)
				mustFeed(t, &stream, "```go\noriginal\n```\n")
				var blocks []markdown.Block
				var err error
				switch outer {
				case "Feed":
					blocks, err = stream.Feed("\nnext\n")
				case "Open":
					blocks, err = stream.Open()
				case "Flush":
					blocks, err = stream.Flush()
				}
				if err != nil {
					t.Fatal(err)
				}
				if !attempted || !refused {
					t.Fatal("a renderer could replace the source or cache owned by the interrupted render")
				}
				if len(blocks) != 1 || blocks[0].Rows(40)[0].Text != "original" {
					t.Fatal("refused reentry changed the original rendering")
				}
			})
		}
	}
}

func TestStreamRendererPanicReleasesItsOperation(t *testing.T) {
	var stream markdown.Stream
	var look markdown.Look
	look.SetRenderer(markdown.FencedCode, func(string, string) (grid.Drawable, error) {
		_ = stream.Look()
		panic("renderer failed")
	})
	stream.SetLook(look)
	mustFeed(t, &stream, "```go\noriginal\n```\n")
	panicked := false
	func() {
		defer func() { panicked = recover() != nil }()
		_, _ = stream.Flush()
	}()
	if !panicked {
		t.Fatal("renderer panic was suppressed")
	}
	stream.SetLook(markdown.Look{})
	blocks, err := stream.Flush()
	if err != nil || len(blocks) != 1 || blocks[0].Rows(40)[0].Text != "original" {
		t.Fatalf("renderer panic lost source or retained rendering ownership: %v, %v", blocks, err)
	}
}
