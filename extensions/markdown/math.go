package markdown

import (
	"bytes"

	"github.com/yuin/goldmark/ast"
	gparser "github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// mathBlock is display mathematics delimited by $$ lines. It remains private:
// Markdown owns the source syntax, while the DisplayMath extension decides whether
// and how that source becomes terminal rows.
type mathBlock struct{ ast.BaseBlock }

var kindMathBlock = ast.NewNodeKind("MathBlock")

func (*mathBlock) IsRaw() bool { return true }

func (*mathBlock) Kind() ast.NodeKind { return kindMathBlock }

func (m *mathBlock) Dump(source []byte, level int) {
	ast.DumpHelper(m, source, level, nil, nil)
}

type mathBlockParser struct{}

func (*mathBlockParser) Trigger() []byte { return []byte{'$'} }

func (*mathBlockParser) Open(_ ast.Node, reader text.Reader, _ gparser.Context) (ast.Node, gparser.State) {
	line, _ := reader.PeekLine()
	if !mathFence(line, reader.LineOffset()) {
		return nil, gparser.NoChildren
	}
	return &mathBlock{}, gparser.NoChildren
}

func (*mathBlockParser) Continue(node ast.Node, reader text.Reader, _ gparser.Context) gparser.State {
	line, segment := reader.PeekLine()
	if mathFence(line, reader.LineOffset()) {
		reader.AdvanceToEOL()
		return gparser.Close
	}
	node.Lines().Append(segment)
	reader.AdvanceToEOL()
	return gparser.Continue | gparser.NoChildren
}

func mathFence(line []byte, column int) bool {
	indent, pos := util.IndentWidth(line, column)
	rest, ok := bytes.CutPrefix(line[pos:], []byte("$$"))
	return indent < 4 && ok && util.IsBlank(rest)
}

func (*mathBlockParser) Close(ast.Node, text.Reader, gparser.Context) {}

func (*mathBlockParser) CanInterruptParagraph() bool { return true }
func (*mathBlockParser) CanAcceptIndentedLine() bool { return false }
