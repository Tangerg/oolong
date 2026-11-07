package markdown

import (
	"fmt"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	gparser "github.com/yuin/goldmark/parser"
	gtext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

const (
	maxBlockNesting = 256
	// Math must claim its opening delimiter before Goldmark's paragraph parser.
	mathBlockPriority = 650
)

// ErrBlockNestingLimit rejects excessive active block nesting before rendering
// or invoking extensions, so it never accompanies partial blocks.
var ErrBlockNestingLimit = fmt.Errorf("markdown: more than %d simultaneously open blocks", maxBlockNesting)

type blockNestingExceeded struct{}

type blockNestingContext struct{ gparser.Context }

func (c blockNestingContext) SetOpenedBlocks(blocks []gparser.Block) {
	if len(blocks) > maxBlockNesting {
		panic(blockNestingExceeded{})
	}
	c.Context.SetOpenedBlocks(blocks)
}

func parse(source []byte, context gparser.Context) (root ast.Node, err error) {
	// Goldmark exposes each opened block but no error return; only this private
	// unwind crosses its callback stack, before parsing another nesting level.
	defer func() {
		switch failure := recover().(type) {
		case nil:
		case blockNestingExceeded:
			err = ErrBlockNestingLimit
		default:
			panic(failure)
		}
	}()
	if context == nil {
		context = gparser.NewContext()
	}
	return parser().Parse(gtext.NewReader(source), gparser.WithContext(blockNestingContext{context})), nil
}

// Each parse has its own context, so one lazily initialized parser can safely
// serve full-source snapshots without rebuilding extension priorities per chunk.
var parser = sync.OnceValue(func() gparser.Parser {
	return goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(gparser.WithBlockParsers(
			util.Prioritized(&mathBlockParser{}, mathBlockPriority),
		)),
	).Parser()
})
