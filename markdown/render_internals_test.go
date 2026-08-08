package markdown

import (
	"testing"

	"github.com/yuin/goldmark/ast"

	"github.com/Tangerg/oolong/core/grid"
)

func TestRendererDepthUsesAnExplicitStackAndPersistentRails(t *testing.T) {
	const depth = 32_768
	root := ast.NewDocument()
	var parent ast.Node = root
	for range depth {
		quote := ast.NewBlockquote()
		parent.AppendChild(parent, quote)
		parent = quote
	}
	paragraph := ast.NewParagraph()
	parent.AppendChild(parent, paragraph)
	paragraph.AppendChild(paragraph, ast.NewString([]byte("deep")))

	rendered := renderer{look: Look{Glyphs: Glyphs{Bar: "|"}}}
	rendered.render(root, frame{})
	if len(rendered.blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(rendered.blocks))
	}
	block := rendered.blocks[0]
	if block.Indent != depth*2 {
		t.Fatalf("indent = %d, want %d", block.Indent, depth*2)
	}
	if len(block.Rail) != depth {
		t.Fatalf("rail segments = %d, want %d", len(block.Rail), depth)
	}
	if got := block.Lines[0].String(); got != "deep" {
		t.Fatalf("text = %q, want deep", got)
	}
}

func TestInlineAndPlainWalkArbitraryASTDepthWithoutRecursion(t *testing.T) {
	const depth = 32_768
	root := ast.NewDocument()
	var parent ast.Node = root
	for range depth {
		emphasis := ast.NewEmphasis(1)
		parent.AppendChild(parent, emphasis)
		parent = emphasis
	}
	parent.AppendChild(parent, ast.NewString([]byte("deep")))

	rendered := renderer{}
	lines := rendered.inline(root, grid.Style{})
	if len(lines) != 1 || lines[0].String() != "deep" {
		t.Fatalf("inline rendering = %v", lines)
	}
	if got := rendered.plain(root); got != "deep" {
		t.Fatalf("plain text = %q, want deep", got)
	}
}
