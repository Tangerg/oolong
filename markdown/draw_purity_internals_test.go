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
	name    string
	width   int
	height  int
	draw    func(grid.View)
	measure func(int) int
	state   func() any
}

func TestLayoutAndDrawAreObservationallyPure(t *testing.T) {
	cases := markdownDrawPurityCases()
	covered := make(map[string]bool, len(cases))
	for _, tc := range cases {
		if covered[tc.name] {
			t.Fatalf("Draw receiver %s has two purity cases", tc.name)
		}
		covered[tc.name] = true
	}
	assertDrawersCovered(t, covered)

	for _, tc := range cases {
		t.Run(strings.TrimPrefix(tc.name, "*"), func(t *testing.T) {
			var before any
			if tc.state != nil {
				before = tc.state()
			}
			if tc.measure != nil {
				first := tc.measure(tc.width)
				if tc.state != nil {
					if after := tc.state(); !reflect.DeepEqual(after, before) {
						t.Fatalf("first Measure changed semantic state\n before: %#v\n  after: %#v", before, after)
					}
				}
				if second := tc.measure(tc.width); second != first {
					t.Fatalf("two Measure calls from the same state returned %d and %d", first, second)
				}
				if tc.state != nil {
					if after := tc.state(); !reflect.DeepEqual(after, before) {
						t.Fatalf("second Measure changed semantic state\n before: %#v\n  after: %#v", before, after)
					}
				}
			}
			first := captureDraw(t, tc.width, tc.height, tc.draw)
			if tc.state != nil {
				if after := tc.state(); !reflect.DeepEqual(after, before) {
					t.Fatalf("first Draw changed semantic state\n before: %#v\n  after: %#v", before, after)
				}
			}
			second := captureDraw(t, tc.width, tc.height, tc.draw)
			if tc.state != nil {
				if after := tc.state(); !reflect.DeepEqual(after, before) {
					t.Fatalf("second Draw changed semantic state\n before: %#v\n  after: %#v", before, after)
				}
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

func assertDrawersCovered(t *testing.T, covered map[string]bool) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	set := token.NewFileSet()
	var found, measured []string
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
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			name := receiverIdentity(fn.Recv.List[0].Type)
			switch {
			case strings.HasPrefix(fn.Name.Name, "Draw"):
				found = append(found, name)
			case fn.Name.Name == "Measure":
				measured = append(measured, name)
			}
		}
	}
	sort.Strings(found)
	for _, name := range found {
		if !covered[name] {
			t.Errorf("Draw entry-point receiver %s has no executable purity case", name)
		}
	}
	for _, name := range measured {
		if !covered[name] {
			t.Errorf("Measure receiver %s has no executable purity case", name)
		}
	}
	for name := range covered {
		if !slices.Contains(found, name) {
			t.Errorf("purity case %s has no production Draw entry point", name)
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
	table  string
	marker string
	rail   string
	indent int
	rule   bool
	fixed  bool
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
		for _, line := range block.lines {
			body.WriteString(line.String())
			body.WriteByte('\n')
		}
		var tableBody strings.Builder
		if block.table != nil {
			for _, tableRow := range block.table.rows {
				for _, cell := range tableRow {
					tableBody.WriteString(cell.String())
					tableBody.WriteByte('\t')
				}
				tableBody.WriteByte('\n')
			}
		}
		meaning.blocks = append(meaning.blocks, blockMeaning{
			text:   body.String(),
			table:  tableBody.String(),
			marker: block.marker.String(),
			rail:   block.rail.String(),
			indent: block.indent,
			rule:   block.rule,
			fixed:  block.fixed,
			gap:    block.blankBefore,
		})
	}
	return meaning
}

func plain(s string) text.Line { return text.Of(s, grid.Style{}) }

func markdownDrawPurityCases() []drawPurityCase {
	block := Block{lines: []text.Line{plain("passive block")}}
	cases := []drawPurityCase{{
		name: "Block", width: 14, height: 2, draw: block.Draw, measure: block.Measure,
	}}

	// One document carrying every shape Draw computes from a width rather than reads
	// from a block: wrapping, a marker, a rail, an indent, and a rule stretched to
	// the room it is drawn in. The view is shorter than the wrapped rows, so drawing
	// also stops part way through — the path that would tempt an implementation to
	// trim the memo it just built.
	doc := &Doc{}
	doc.SetBlocks([]Block{
		{lines: []text.Line{plain("a heading")}},
		{
			lines:  []text.Line{plain("an item long enough to wrap twice over")},
			marker: plain("- "), indent: 2, blankBefore: true,
		},
		{
			lines: []text.Line{plain("a quotation that also wraps")},
			rail:  plain("| "), indent: 2, blankBefore: true,
		},
		{lines: []text.Line{plain("-")}, rule: true, blankBefore: true},
		{
			table: &table{
				rows: [][]text.Line{
					{plain("name"), plain("description")},
					{plain("one"), plain("a value long enough to wrap")},
				},
				header: true, separator: " | ", divider: "-",
			},
			blankBefore: true,
		},
		{lines: []text.Line{plain("after the rule")}, blankBefore: true},
	})
	return append(cases, drawPurityCase{
		name: "*Doc", width: 14, height: 6,
		draw: doc.Draw, measure: doc.Measure,
		state: func() any { return meaningOfDoc(doc) },
	})
}
