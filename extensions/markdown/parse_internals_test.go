package markdown

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	gparser "github.com/yuin/goldmark/parser"

	"github.com/Tangerg/oolong/ui/core/content"
)

func TestBlockNestingLimitIncludesTheLeaf(t *testing.T) {
	for _, quotes := range []int{maxBlockNesting - 1, maxBlockNesting} {
		t.Run(strconv.Itoa(quotes), func(t *testing.T) {
			blocks, err := Render(strings.Repeat("> ", quotes)+"leaf", Look{})
			if quotes == maxBlockNesting {
				if !errors.Is(err, ErrBlockNestingLimit) || blocks != nil {
					t.Fatalf("over-limit parse: blocks=%d error=%v", len(blocks), err)
				}
				return
			}
			if err != nil || len(blocks) != 1 {
				t.Fatalf("boundary parse: blocks=%d error=%v", len(blocks), err)
			}
			rows := blocks[0].Rows(40)
			var value strings.Builder
			for _, row := range rows {
				value.WriteString(row.Content.String())
			}
			if value.String() != "leaf" {
				t.Fatalf("boundary content = %q", value.String())
			}
		})
	}
}

func TestBlockNestingLimitDoesNotCountSiblingsOrCodeMarkers(t *testing.T) {
	siblings := maxBlockNesting + 1
	blocks, err := Render(strings.Repeat("paragraph\n\n", siblings), Look{})
	if err != nil || len(blocks) != siblings {
		t.Fatalf("sibling blocks=%d error=%v", len(blocks), err)
	}
	literal := strings.Repeat("> ", maxBlockNesting*2) + "literal"
	blocks, err = Render("```\n"+literal+"\n```", Look{})
	if err != nil || len(blocks) != 1 {
		t.Fatalf("code block count=%d error=%v", len(blocks), err)
	}
	rows := blocks[0].Rows(len(literal) + 1)
	if len(rows) != 1 || rows[0].Content.String() != literal {
		t.Fatalf("code contents = %+v", rows)
	}
}

type openedBlocksSpy struct {
	gparser.Context
	calls, maximum, lastPosition int
}

func (c *openedBlocksSpy) SetOpenedBlocks(blocks []gparser.Block) {
	c.calls++
	c.maximum = max(c.maximum, len(blocks))
	if len(blocks) != 0 {
		c.lastPosition = blocks[len(blocks)-1].Node.Pos()
	}
	c.Context.SetOpenedBlocks(blocks)
}

func TestBlockNestingLimitStopsBeforeTheNextBlock(t *testing.T) {
	for _, quotes := range []int{maxBlockNesting + 1, maxBlockNesting * 32} {
		t.Run(strconv.Itoa(quotes), func(t *testing.T) {
			spy := &openedBlocksSpy{Context: gparser.NewContext()}
			blocks, err := render(strings.Repeat("> ", quotes)+"leaf", Look{}, spy)
			if !errors.Is(err, ErrBlockNestingLimit) || blocks != nil {
				t.Errorf("over-limit parse: blocks=%d error=%v", len(blocks), err)
			}
			if spy.maximum != maxBlockNesting || spy.calls != maxBlockNesting+1 {
				t.Errorf("opened blocks=%d callbacks=%d", spy.maximum, spy.calls)
			}
			if want := 2 * (maxBlockNesting - 1); spy.lastPosition != want {
				t.Errorf("last accepted block at byte %d, want %d", spy.lastPosition, want)
			}
		})
	}
}

type panickingBlockContext struct {
	gparser.Context
	failure any
}

func (c panickingBlockContext) SetOpenedBlocks([]gparser.Block) { panic(c.failure) }

func TestParsePropagatesUnexpectedPanics(t *testing.T) {
	for _, test := range []struct {
		name    string
		failure any
	}{
		{name: "string", failure: "unexpected parser failure"},
		{name: "error", failure: errors.New("unexpected parser failure")},
	} {
		t.Run(test.name, func(t *testing.T) {
			failure := test.failure
			defer func() {
				if got := recover(); got != failure {
					t.Fatalf("panic = %v, want original %v", got, failure)
				}
			}()
			_, _ = render("text", Look{}, panickingBlockContext{Context: gparser.NewContext(), failure: failure})
		})
	}
}

func TestRendererMarksBlockNestingFailureAsFailed(t *testing.T) {
	source := "before\n\n" + strings.Repeat("> ", maxBlockNesting) + "leaf"
	renderer := NewRenderer(RendererConfig{})
	body, err := renderer(t.Context(), source)
	if !errors.Is(err, ErrBlockNestingLimit) || body != nil {
		t.Errorf("renderer body=%T error=%v", body, err)
	}
	document, err := content.NewStreamDocument(content.StreamConfig{Format: "markdown"})
	if err != nil {
		t.Fatal(err)
	}
	if err := document.Append([]byte(source)); err != nil {
		t.Fatal(err)
	}
	if err := document.End(nil); err != nil {
		t.Fatal(err)
	}
	if err := document.Accept(document.Snapshot().Prepare(t.Context(), renderer)); err != nil {
		t.Fatal(err)
	}
	change := document.Current()
	if change.State != content.Failed || change.Body != nil || !errors.Is(change.PrepareError, ErrBlockNestingLimit) || change.Publishable() {
		t.Fatalf("failed parse state = %+v", change)
	}
}
