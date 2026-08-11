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
	dynamic := make(map[string]bool, len(cases))
	for _, tc := range cases {
		if dynamic[tc.name] {
			t.Fatalf("Draw receiver %s has two purity cases", tc.name)
		}
		dynamic[tc.name] = true
	}
	// These are immutable appearance values. The explicit list is also a
	// completeness guard: every new Draw receiver must be declared passive here or
	// be given a semantic-state contract above.
	passive := map[string]bool{
		"Box":         true,
		"Cell":        true,
		"Help":        true,
		"Image":       true,
		"Label":       true,
		"LineNumbers": true,
		"*Message":    true,
		"Overlay":     true,
		"Palette":     true,
		"Progress":    true,
		"Scrollbar":   true,
		"Table":       true,
	}
	for name := range passive {
		if dynamic[name] {
			t.Fatalf("Draw receiver %s is classified as both stateful and passive", name)
		}
	}
	assertDrawersClassified(t, dynamic, passive)

	for _, tc := range cases {
		t.Run(strings.TrimPrefix(tc.name, "*"), func(t *testing.T) {
			before := tc.state()
			if tc.measure != nil {
				first := tc.measure(tc.width)
				if after := tc.state(); !reflect.DeepEqual(after, before) {
					t.Fatalf("first Measure changed semantic state\n before: %#v\n  after: %#v", before, after)
				}
				if second := tc.measure(tc.width); second != first {
					t.Fatalf("two Measure calls from the same state returned %d and %d", first, second)
				}
				if after := tc.state(); !reflect.DeepEqual(after, before) {
					t.Fatalf("second Measure changed semantic state\n before: %#v\n  after: %#v", before, after)
				}
			}
			first := captureDraw(t, tc.width, tc.height, tc.draw)
			if after := tc.state(); !reflect.DeepEqual(after, before) {
				t.Fatalf("first Draw changed semantic state\n before: %#v\n  after: %#v", before, after)
			}
			second := captureDraw(t, tc.width, tc.height, tc.draw)
			if after := tc.state(); !reflect.DeepEqual(after, before) {
				t.Fatalf("second Draw changed semantic state\n before: %#v\n  after: %#v", before, after)
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

func assertDrawersClassified(t *testing.T, dynamic, passive map[string]bool) {
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
		if !dynamic[name] && !passive[name] {
			t.Errorf("Draw entry-point receiver %s has no purity classification", name)
		}
	}
	for _, name := range measured {
		if !dynamic[name] && !passive[name] {
			t.Errorf("Measure receiver %s has no purity classification", name)
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
		maxRows: editor.MaxRows, singleLine: editor.SingleLine, mask: editor.Mask,
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

	diffView := NewDiff(Dark(), Unicode(), []diff.Hunk{{
		Lines: diff.Script{{Kind: diff.Added, Text: "one two three"}},
	}})
	diffView.ShowNumbers(true)
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
	dialog.Show()
	cases = append(cases, widgetPurityCase("*DialogPanel", dialog.Panel(), func() any {
		return struct {
			semantic headless.SemanticNode
			editor   editorMeaning
		}{dialog.Semantics(), meaningOfEditor(dialogBody)}
	}))

	panelBody := &headless.Editor{}
	panelBody.Insert("body")
	panelBody.Focus(true)
	panel := NewPanel(Theme{}, Unicode(), panelBody)
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
	settings := NewSettings(SettingsConfig[int]{
		Items: []int{0, 1},
		Label: func(index int) string { return []string{"theme", "wrap"}[index] },
		Value: func(index int) string { return settingValues[index] },
	})
	settings.Controller().Select(1)
	cases = append(cases, widgetPurityCase("*Settings", settings, func() any {
		return struct {
			selected int
			values   []string
		}{settings.Selected(), slices.Clone(settingValues)}
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
	composer.SetText("hello")
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
			}{paragraph.Lines(), paragraph.Indent, paragraph.MaxRows, paragraph.Links}
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
	formField := &headless.Text{Label: "field", Value: headless.Bind(&formValue)}
	formController := headless.NewForm(formField)
	formController.Focus(true)
	form := NewForm(FormConfig{Controller: formController, Title: "form"})
	cases = append(cases, widgetPurityCase("*Form", form, func() any {
		return struct {
			focused headless.Field
			value   string
			editor  editorMeaning
			err     error
		}{formController.Focused(), formValue, meaningOfEditor(formField.Editor()), formController.Error()}
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
