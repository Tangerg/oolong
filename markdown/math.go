package markdown

import (
	"bytes"

	"github.com/yuin/goldmark/ast"
	gparser "github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// mathBlock is display mathematics delimited by $$ lines. It remains private:
// Markdown owns the source syntax, while the DisplayMath extension decides whether
// and how that source becomes terminal rows.
type mathBlock struct{ ast.BaseBlock }

var kindMathBlock = ast.NewNodeKind("MathBlock")

func (*mathBlock) Kind() ast.NodeKind { return kindMathBlock }

func (m *mathBlock) Dump(source []byte, level int) {
	ast.DumpHelper(m, source, level, nil, nil)
}

type mathBlockParser struct{}

func (*mathBlockParser) Trigger() []byte { return []byte{'$'} }

func (*mathBlockParser) Open(_ ast.Node, reader text.Reader, context gparser.Context) (ast.Node, gparser.State) {
	line, _ := reader.PeekLine()
	if !mathFence(line, context.BlockOffset()) {
		return nil, gparser.NoChildren
	}
	return &mathBlock{}, gparser.NoChildren
}

func (*mathBlockParser) Continue(node ast.Node, reader text.Reader, context gparser.Context) gparser.State {
	line, segment := reader.PeekLine()
	if mathFence(line, context.BlockOffset()) {
		reader.AdvanceToEOL()
		return gparser.Close
	}
	segment.ForceNewline = true
	node.Lines().Append(segment)
	reader.AdvanceToEOL()
	return gparser.Continue | gparser.NoChildren
}

func mathFence(line []byte, offset int) bool {
	return offset >= 0 && offset <= 3 && offset <= len(line) &&
		bytes.Equal(bytes.TrimSpace(line[offset:]), []byte("$$"))
}

func (*mathBlockParser) Close(ast.Node, text.Reader, gparser.Context) {}

func (*mathBlockParser) CanInterruptParagraph() bool { return true }
func (*mathBlockParser) CanAcceptIndentedLine() bool { return false }
