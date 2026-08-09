package markdown

import (
	"bytes"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

// drawPurityCase names the semantic projection that Draw is forbidden to change.
//
// A document is the value that goes into a transcript and into terminal scrollback,
// so drawing it is the one place where losing purity writes wrong bytes into history
// the program can never take back. The wrap memo is deliberately absent from the
// projection: it describes the frame just built, while Doc.blocks says what the
// document means.
type drawPurityCase struct {
	name   string
	width  int
	height int
	draw   func(grid.View)
	state  func() any
}

func TestDrawIsObservationallyPure(t *testing.T) {
	cases := markdownDrawPurityCases()
	dynamic := make(map[string]bool, len(cases))
	for _, tc := range cases {
		if dynamic[tc.name] {
			t.Fatalf("Draw receiver %s has two purity cases", tc.name)
		}
		dynamic[tc.name] = true
	}
	// Nothing here draws without owning state. Listing the classification anyway
	// makes a future immutable drawer say so rather than be forgotten.
	passive := map[string]bool{}
	for name := range passive {
		if dynamic[name] {
			t.Fatalf("Draw receiver %s is classified as both stateful and passive", name)
		}
	}
	assertDrawersClassified(t, dynamic, passive)

	for _, tc := range cases {
		t.Run(strings.TrimPrefix(tc.name, "*"), func(t *testing.T) {
			before := tc.state()
			first := captureDraw(t, tc.width, tc.height, tc.draw)
			if after := tc.state(); !reflect.DeepEqual(after, before) {
				t.Fatalf("first Draw changed semantic state\n before: %#v\n  after: %#v", before, after)
			}
			second := captureDraw(t, tc.width, tc.height, tc.draw)
			if after := tc.state(); !reflect.DeepEqual(after, before) {
				t.Fatalf("second Draw changed semantic state\n before: %#v\n  after: %#v", before, after)
			}
			if !reflect.DeepEqual(second, first) {
				t.Fatalf("two Draw calls from the same state produced different frames")
			}
		})
	}
}

type capturedFrame struct {
	bytes  string
	cursor grid.Cursor
}

func captureDraw(t *testing.T, width, height int, draw func(grid.View)) capturedFrame {
	t.Helper()
	screen := grid.NewScreen(width, height)
	draw(screen.Frame())
	var output bytes.Buffer
	if err := screen.Flush(&output); err != nil {
		t.Fatalf("flush captured frame: %v", err)
	}
	return capturedFrame{bytes: output.String(), cursor: screen.Cursor()}
}

func assertDrawersClassified(t *testing.T, dynamic, passive map[string]bool) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	set := token.NewFileSet()
	var found []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := goparser.ParseFile(set, entry.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || !strings.HasPrefix(fn.Name.Name, "Draw") || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			found = append(found, receiverIdentity(fn.Recv.List[0].Type))
		}
	}
	sort.Strings(found)
	for _, name := range found {
		if !dynamic[name] && !passive[name] {
			t.Errorf("Draw entry-point receiver %s has no purity classification", name)
		}
	}
	for name := range dynamic {
		if !slices.Contains(found, name) {
			t.Errorf("purity case %s has no production Draw entry point", name)
		}
	}
	for name := range passive {
		if !slices.Contains(found, name) {
			t.Errorf("passive classification %s has no production Draw entry point", name)
		}
	}
}

func receiverIdentity(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.StarExpr:
		return "*" + receiverIdentity(expression.X)
	case *ast.IndexExpr:
		return receiverIdentity(expression.X)
	case *ast.IndexListExpr:
		return receiverIdentity(expression.X)
	case *ast.Ident:
		return expression.Name
	default:
		return "<unknown>"
	}
}

// blockMeaning is one block reduced to values that cannot alias the document.
type blockMeaning struct {
	text   string
	marker string
	rail   string
	indent int
	rule   bool
	gap    bool
}

type docMeaning struct{ blocks []blockMeaning }

// meaningOfDoc projects a document's semantic state into values of its own.
//
// A slice taken out of [Doc] would share the backing array, so a Draw that changed a
// block in place would compare equal to itself and prove nothing. Rendering the
// meaning into fresh strings removes that possibility.
func meaningOfDoc(doc *Doc) docMeaning {
	meaning := docMeaning{}
	for _, block := range doc.blocks {
		var body strings.Builder
		for _, line := range block.Lines {
			body.WriteString(line.String())
			body.WriteByte('\n')
		}
		meaning.blocks = append(meaning.blocks, blockMeaning{
			text:   body.String(),
			marker: block.Marker.String(),
			rail:   block.Rail.String(),
			indent: block.Indent,
			rule:   block.Rule,
			gap:    block.Gap,
		})
	}
	return meaning
}

func plain(s string) text.Line { return text.Of(s, grid.Style{}) }

func markdownDrawPurityCases() []drawPurityCase {
	// One document carrying every shape Draw computes from a width rather than reads
	// from a block: wrapping, a marker, a rail, an indent, and a rule stretched to
	// the room it is drawn in. The view is shorter than the wrapped rows, so drawing
	// also stops part way through — the path that would tempt an implementation to
	// trim the memo it just built.
	doc := &Doc{}
	doc.SetBlocks([]Block{
		{Lines: []text.Line{plain("a heading")}},
		{
			Lines:  []text.Line{plain("an item long enough to wrap twice over")},
			Marker: plain("- "), Indent: 2, Gap: true,
		},
		{
			Lines: []text.Line{plain("a quotation that also wraps")},
			Rail:  plain("| "), Indent: 2, Gap: true,
		},
		{Lines: []text.Line{plain("-")}, Rule: true, Gap: true},
		{Lines: []text.Line{plain("after the rule")}, Gap: true},
	})
	return []drawPurityCase{{
		name: "*Doc", width: 14, height: 6,
		draw:  doc.Draw,
		state: func() any { return meaningOfDoc(doc) },
	}}
}
