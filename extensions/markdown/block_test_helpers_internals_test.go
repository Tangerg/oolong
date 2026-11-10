package markdown

import "github.com/Tangerg/oolong/ui/core/text"

// The accessors below read one body kind out of a block for assertions; a block
// of another kind reads as empty.

func (b Block) lines() []text.Line {
	lines, _ := b.body.(textBody)
	return lines
}

func (b Block) item() *listItem {
	item, _ := b.body.(*listItem)
	return item
}

func (b Block) table() *table {
	t, _ := b.body.(*table)
	return t
}

func (b Block) rule() *text.Span {
	rule, ok := b.body.(ruleBody)
	if !ok {
		return nil
	}
	return &rule.glyph
}
