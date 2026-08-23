package kit

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/diff"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/text"
)

type drawPurityCase struct {
	name    string
	width   int
	height  int
	draw    func(grid.View)
	measure func(int) int
	state   func() any
}

func TestLayoutAndDrawAreObservationallyPure(t *testing.T) {
	cases := kitDrawPurityCases()
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
				t.Fatal("two Draw calls from the same state produced different frames")
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
		file, err := parser.ParseFile(set, entry.Name(), nil, 0)
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

type purityBlock struct {
	text   string
	height int
}

func (b *purityBlock) Draw(view grid.View) { view.Text(0, 0, b.text, grid.Style{}) }

func (b *purityBlock) Measure(int) int { return max(b.height, 1) }

type editorMeaning struct {
	text         string
	line, column int
	selection    string
	look         headless.Look
	placeholder  string
	keys         *keymap.Map
	clipboard    headless.Clipboard
	maxRows      int
	singleLine   bool
	mask         string
	gutter       headless.RowGutter
	cursor       grid.CursorStyle
}

func meaningOfEditor(editor *headless.Editor) editorMeaning {
	line, column := editor.Cursor()
	return editorMeaning{
		text: editor.Text(), line: line, column: column,
		selection: editor.Selected(), look: editor.Look,
		placeholder: editor.Placeholder, keys: editor.Keys, clipboard: editor.Clipboard,
		maxRows: editor.MaxRows, singleLine: editor.SingleLine(), mask: editor.Mask(),
		gutter: editor.Gutter, cursor: editor.CursorStyle,
	}
}

func widgetPurityCase(name string, widget headless.Widget, state func() any) drawPurityCase {
	root := headless.NewRoot(widget)
	var measure func(int) int
	if sized, ok := widget.(layout.Measurer); ok {
		measure = sized.Measure
	}
	return drawPurityCase{
		name: name, width: 24, height: 6,
		draw: root.Draw, measure: measure, state: state,
	}
}

func kitDrawPurityCases() []drawPurityCase {
	var cases []drawPurityCase

	box := Box{Glyphs: ASCII(), Padding: layout.Uniform(1), Title: "box"}
	cases = append(cases, drawPurityCase{
		name: "Box", width: 16, height: 4,
		draw: func(view grid.View) { box.Draw(view) },
	})

	cell := LabelCell(Label{Text: "cell"})
	cases = append(cases, drawPurityCase{
		name: "Cell", width: 8, height: 1,
		draw: func(view grid.View) { cell.Draw(view, grid.Style{}) }, measure: cell.Measure,
	})

	help := Help{Show: []keymap.Action{"accept"}}
	cases = append(cases, drawPurityCase{
		name: "Help", width: 16, height: 1, draw: help.Draw, measure: help.Measure,
	})

	imageView := Image{Alt: "unavailable"}
	cases = append(cases, drawPurityCase{
		name: "Image", width: 16, height: 1, draw: imageView.Draw, measure: imageView.Measure,
	})

	label := Label{Text: "label", Ellipsis: "…"}
	cases = append(cases, drawPurityCase{
		name: "Label", width: 8, height: 1, draw: label.Draw, measure: label.Measure,
	})

	numbers := LineNumbers{Separator: "│"}
	rows := []text.Row{{Text: "one", Line: 1}, {Text: "two", Line: 2}}
	cases = append(cases, drawPurityCase{
		name: "LineNumbers", width: 4, height: 2,
		draw: func(view grid.View) { numbers.Draw(view, rows) },
	})

	entry := &Entry{Label: "source", Body: "passive content"}
	cases = append(cases, drawPurityCase{
		name: "*Entry", width: 16, height: 4, draw: entry.Draw, measure: entry.Measure,
	})

	overlay := Overlay{Width: 8, Height: 2}
	cases = append(cases, drawPurityCase{
		name: "Overlay", width: 16, height: 4,
		draw: func(view grid.View) { overlay.Draw(view) },
	})

	palette := Palette{Empty: "nothing found"}
	cases = append(cases, drawPurityCase{
		name: "Palette", width: 16, height: 2, draw: palette.Draw, measure: palette.Measure,
	})

	progress := Progress{Glyphs: ASCII(), Done: 1, Total: 3, Label: "work", Percent: true}
	cases = append(cases, drawPurityCase{
		name: "Progress", width: 20, height: 1, draw: progress.Draw, measure: progress.Measure,
	})

	scrollbar := Scrollbar{Total: 20, Window: 5, Offset: 4, Glyphs: ASCII()}
	cases = append(cases, drawPurityCase{
		name: "Scrollbar", width: 1, height: 5, draw: scrollbar.Draw,
	})

	table := Table{
		Columns: []Column{{Title: "name"}}, Rows: 1, Header: true,
		Cell: func(int, int) Cell { return LabelCell(Label{Text: "value"}) },
	}
	cases = append(cases, drawPurityCase{
		name: "Table", width: 12, height: 2, draw: table.Draw, measure: table.Measure,
	})

	diffView := NewDiff(DiffConfig{Theme: Dark(), Glyphs: Unicode(), Hunks: []diff.Hunk{{
		Lines: diff.Script{{Kind: diff.Added, Text: "one two three"}},
	}}})

	diffView.SetNumbers(true)
	cases = append(cases, drawPurityCase{
		name: "*Diff", width: 10, height: 3,
		draw: diffView.Draw, measure: diffView.Measure,
		state: func() any {
			return struct {
				hunks   []diff.Hunk
				theme   Theme
				glyphs  Glyphs
				numbers bool
			}{diffView.Hunks(), diffView.theme, diffView.glyphs, diffView.numbers}
		},
	})

	dialogBody := &headless.Editor{}
	dialogBody.Insert("body")
	dialogBody.Focus(true)
	dialog := NewDialog(DialogConfig{Stack: &headless.Stack{}, Glyphs: Unicode(), Title: "title", Body: dialogBody})
	dialog.Controller().Show()
	cases = append(cases, widgetPurityCase("*DialogPanel", dialog.Panel(), func() any {
		return struct {
			semantic headless.SemanticNode
			editor   editorMeaning
		}{dialog.Semantics(), meaningOfEditor(dialogBody)}
	}))

	panelBody := &headless.Editor{}
	panelBody.Insert("body")
	panelBody.Focus(true)
	panel := NewPanel(PanelConfig{Box: Box{Theme: Theme{}, Glyphs: Unicode()}, Content: panelBody})
	panel.Box.Title = "panel"
	cases = append(cases, widgetPurityCase("*Panel", panel, func() any {
		return meaningOfEditor(panelBody)
	}))

	firstPane := &headless.Editor{}
	firstPane.Insert("first")
	secondPane := &headless.Editor{}
	secondPane.Insert("second")
	tabs := NewTabs(TabsConfig{
		Glyphs: Unicode(),
		Items: []headless.Tab{
			{Title: "first", Of: firstPane},
			{Title: "second", Of: secondPane},
		},
	})
	tabs.Controller().Select(1)
	tabs.Focus(true)
	cases = append(cases, widgetPurityCase("*Tabs", tabs, func() any {
		return struct {
			semantic headless.SemanticNode
			first    editorMeaning
			second   editorMeaning
		}{tabs.Controller().Semantics(), meaningOfEditor(firstPane), meaningOfEditor(secondPane)}
	}))

	slider := NewSlider(SliderConfig{Glyphs: Unicode(), Maximum: 10, Label: "speed"})
	slider.Controller().Set(4)
	slider.Focus(true)
	cases = append(cases, widgetPurityCase("*Slider", slider, func() any {
		return slider.Semantics()
	}))

	settingValues := []string{"dark", "on"}
	settingChanges := 0
	settings := NewSettings(SettingsConfig[int]{
		Items:  []int{0, 1},
		Label:  func(index int) string { return []string{"theme", "wrap"}[index] },
		Value:  func(index int) string { return settingValues[index] },
		Change: func(int, int, keymap.Action) bool { settingChanges++; return true },
	})
	settings.Controller().Select(1)
	cases = append(cases, widgetPurityCase("*Settings", settings, func() any {
		return struct {
			selected int
			values   []string
			changes  int
		}{settings.Controller().Selected(), slices.Clone(settingValues), settingChanges}
	}))

	content := &headless.Transcript{}
	content.Append(&purityBlock{text: "retained", height: 2})
	var transcriptScroll headless.Scroll
	transcript := &Transcript{Content: content, Scroll: &transcriptScroll, Current: -1}
	headless.NewRoot(transcript).Draw(grid.NewSurface(24, 3).View())
	transcriptCase := widgetPurityCase("*Transcript", transcript, func() any {
		return struct {
			length, width, height, start int
			offset                       int
			following                    bool
		}{
			content.Len(), content.Width(), content.Height(), content.StartRow(),
			transcriptScroll.Offset(), transcriptScroll.AtBottom(),
		}
	})
	transcriptCase.height = 3
	cases = append(cases, transcriptCase)

	spinner := &Spinner{Glyphs: Glyphs{Spinner: []string{"a", "b"}}, Label: "working", frame: 1}
	cases = append(cases, drawPurityCase{
		name: "*Spinner", width: 24, height: 1,
		draw: func(view grid.View) { spinner.Draw(view) }, measure: spinner.Measure,
		state: func() any {
			return struct {
				frame  int
				label  string
				frames []string
			}{spinner.frame, spinner.Label, slices.Clone(spinner.Glyphs.Spinner)}
		},
	})

	composer := &Composer{Prompt: "> ", MaxRows: 3}
	composer.Editor().Placeholder = "say something"
	composer.Editor().SetText("hello")
	composer.Editor().MoveLeft()
	composer.Focus(true)
	cases = append(cases, widgetPurityCase("*Composer", composer, func() any {
		return meaningOfEditor(composer.Editor())
	}))

	paragraph := NewParagraph("one two three four", grid.Style{})
	paragraph.Indent = 1
	cases = append(cases, drawPurityCase{
		name: "*Paragraph", width: 10, height: 3,
		draw: func(view grid.View) { paragraph.Draw(view) }, measure: paragraph.Measure,
		state: func() any {
			return struct {
				lines         any
				indent, limit int
				links         bool
			}{paragraph.Lines(), paragraph.Indent, paragraph.MaxRows, paragraph.linkConfig.Enabled}
		},
	})

	code := NewCode([]text.Line{
		text.Of("package main", grid.Style{}),
		text.Of("func main() {}", grid.Style{}),
	})
	code.Gutter = LineNumbers{}
	cases = append(cases, drawPurityCase{
		name: "*Code", width: 18, height: 3,
		draw: code.Draw, measure: code.Measure,
		state: func() any {
			return code.Lines()
		},
	})

	formValue := "answer"
	fieldChecks, formChecks, formDone, formGaveUp := 0, 0, 0, 0
	formField := &headless.Text{
		Label: "field", Value: headless.Bind(&formValue),
		Check: func(string) error { fieldChecks++; return nil },
	}
	formController := headless.NewForm(formField)
	formController.Check = func() error { formChecks++; return nil }
	formController.Done = func() { formDone++ }
	formController.GaveUp = func() { formGaveUp++ }
	formController.Focus(true)
	form := NewForm(FormConfig{Controller: formController, Title: "form"})
	cases = append(cases, widgetPurityCase("*Form", form, func() any {
		return struct {
			focused   headless.Field
			value     string
			editor    editorMeaning
			err       error
			callbacks [4]int
		}{
			formController.Focused(), formValue, meaningOfEditor(formField.Editor()), formController.Error(),
			[4]int{fieldChecks, formChecks, formDone, formGaveUp},
		}
	}))

	status := &Status{Glyphs: ASCII(), Doing: "working", Elapsed: "2s"}
	status.Tick()
	cases = append(cases, drawPurityCase{
		name: "*Status", width: 24, height: 1,
		draw: func(view grid.View) { status.Draw(view) }, measure: status.Measure,
		state: func() any {
			return struct {
				doing, elapsed string
				frame          int
				frames         []string
			}{status.Doing, status.Elapsed, status.spinner.frame, slices.Clone(status.spinner.Glyphs.Spinner)}
		},
	})

	treeController := headless.NewTree(
		headless.Node[string]{Item: "root", Children: []headless.Node[string]{{Item: "leaf"}}},
	)
	treeController.Open(0)
	treeController.Select(1)
	treeController.Focus(true)
	tree := NewTree(TreeConfig[string]{
		Controller: treeController, Text: func(item string) string { return item }, Indent: 2,
	})
	cases = append(cases, widgetPurityCase("*Tree", tree, func() any {
		return struct {
			selected int
			focused  bool
			rows     []headless.Shown[string]
		}{treeController.Selected(), treeController.Focused(), treeController.Rows()}
	}))

	return cases
}
