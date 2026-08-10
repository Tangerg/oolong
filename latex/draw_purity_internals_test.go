package latex

import (
	"bytes"
	goast "go/ast"
	goparser "go/parser"
	"go/token"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

func TestFormulaMeasurementAndDrawingAreObservationallyPure(t *testing.T) {
	assertFormulaIsEveryDrawingReceiver(t)

	formula := Render(`x = \frac{-b \pm \sqrt{b^2 - 4ac}}{2a}`, Look{})
	before := formulaMeaningOf(formula)
	firstHeight := formula.Measure(24)
	if got := formulaMeaningOf(formula); !reflect.DeepEqual(got, before) {
		t.Fatalf("Measure changed formula meaning\n before: %#v\n  after: %#v", before, got)
	}
	if secondHeight := formula.Measure(24); secondHeight != firstHeight {
		t.Fatalf("two measures returned %d and %d", firstHeight, secondHeight)
	}
	first := captureFormula(t, formula, 24, firstHeight)
	if got := formulaMeaningOf(formula); !reflect.DeepEqual(got, before) {
		t.Fatalf("Draw changed formula meaning\n before: %#v\n  after: %#v", before, got)
	}
	second := captureFormula(t, formula, 24, firstHeight)
	if second != first {
		t.Fatal("two draws from the same formula produced different frames")
	}
}

type formulaMeaning struct {
	source string
	look   Look
	lines  []text.Line
	width  int
	err    string
}

func formulaMeaningOf(formula *Formula) formulaMeaning {
	err := ""
	if formula.err != nil {
		err = formula.err.Error()
	}
	return formulaMeaning{
		source: strings.Clone(formula.source), look: formula.look,
		lines: text.CloneLines(formula.lines), width: formula.width, err: err,
	}
}

type formulaFrame struct {
	bytes  string
	cursor grid.Cursor
}

func captureFormula(t *testing.T, formula *Formula, width, height int) formulaFrame {
	t.Helper()
	screen := grid.NewScreen(width, height)
	formula.Draw(screen.Frame())
	var output bytes.Buffer
	if err := screen.Flush(&output); err != nil {
		t.Fatal(err)
	}
	return formulaFrame{bytes: output.String(), cursor: screen.Cursor()}
}

// assertFormulaIsEveryDrawingReceiver derives the classification from source. A
// second drawable added to this module must join the purity gate instead of relying
// on a reviewer to notice it.
func assertFormulaIsEveryDrawingReceiver(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	set := token.NewFileSet()
	var drawers, measurers []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := goparser.ParseFile(set, entry.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*goast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			receiver := formulaReceiver(fn.Recv.List[0].Type)
			switch {
			case strings.HasPrefix(fn.Name.Name, "Draw"):
				drawers = append(drawers, receiver)
			case fn.Name.Name == "Measure":
				measurers = append(measurers, receiver)
			}
		}
	}
	sort.Strings(drawers)
	sort.Strings(measurers)
	if !reflect.DeepEqual(drawers, []string{"*Formula"}) {
		t.Fatalf("Draw receivers = %v; purity cases cover only *Formula", drawers)
	}
	if !reflect.DeepEqual(measurers, []string{"*Formula"}) {
		t.Fatalf("Measure receivers = %v; purity cases cover only *Formula", measurers)
	}
}

func formulaReceiver(expression goast.Expr) string {
	switch expression := expression.(type) {
	case *goast.StarExpr:
		return "*" + formulaReceiver(expression.X)
	case *goast.Ident:
		return expression.Name
	default:
		return "<unknown>"
	}
}
