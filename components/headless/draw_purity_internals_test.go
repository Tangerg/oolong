package headless

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/core/fuzzy"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
)

// drawPurityCase names the semantic projection that Draw is forbidden to change.
// Presentation snapshots and layout caches are intentionally absent: they exist to
// describe the frame just built, while the values here say what the component means.
type drawPurityCase struct {
	name   string
	width  int
	height int
	draw   func(grid.View)
	state  func() any
}

func TestDrawIsObservationallyPure(t *testing.T) {
	cases := headlessDrawPurityCases()
	dynamic := make(map[string]bool, len(cases))
	for _, tc := range cases {
		if dynamic[tc.name] {
			t.Fatalf("Draw receiver %s has two purity cases", tc.name)
		}
		dynamic[tc.name] = true
	}
	// These value drawers are immutable adapters or derived presentation values.
	// Listing them beside the dynamic cases makes a new Draw implementation fail
	// until its ownership class and, when stateful, its semantic projection are named.
	passive := map[string]bool{
		"Static":           true,
		"TranscriptLayout": true,
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
		file, err := parser.ParseFile(set, entry.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range file.Decls {
			fn, ok := declaration.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "Draw" || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			found = append(found, receiverIdentity(fn.Recv.List[0].Type))
		}
	}
	sort.Strings(found)
	for _, name := range found {
		if !dynamic[name] && !passive[name] {
			t.Errorf("Draw receiver %s has no purity classification", name)
		}
	}
	for name := range dynamic {
		if !slices.Contains(found, name) {
			t.Errorf("purity case %s has no production Draw method", name)
		}
	}
	for name := range passive {
		if !slices.Contains(found, name) {
			t.Errorf("passive classification %s has no production Draw method", name)
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

type purityWidget struct {
	text    string
	height  int
	focused bool
}

func (w *purityWidget) Draw(frame Frame) {
	frame.Text(0, 0, w.text, grid.Style{})
}

func (w *purityWidget) Measure(int) int { return max(w.height, 1) }

func (w *purityWidget) Focus(has bool) { w.focused = has }

func (*purityWidget) Handle(input.Event) bool { return false }

type purityModal struct{ *purityWidget }

func (*purityModal) Place(space image.Point) layout.Placement {
	return layout.Placement{Width: min(space.X, 12), Height: min(space.Y, 3)}
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
	blurred      bool
}

func meaningOfEditor(editor *Editor) editorMeaning {
	line, column := editor.Cursor()
	return editorMeaning{
		text: editor.Text(), line: line, column: column,
		selection: editor.Selected(), blurred: editor.blurred,
	}
}

func widgetPurityCase(name string, widget Widget, state func() any) drawPurityCase {
	root := NewRoot(widget)
	return drawPurityCase{name: name, width: 24, height: 6, draw: root.Draw, state: state}
}

func headlessDrawPurityCases() []drawPurityCase {
	var cases []drawPurityCase

	contentModal := &purityModal{&purityWidget{text: "content", height: 2}}
	contentDialog := NewDialog(&Stack{}, "content", contentModal)
	contentDialog.Show()
	contentDialog.Content().Focus(true)
	cases = append(cases, widgetPurityCase("*DialogContent", contentDialog.Content(), func() any {
		return struct {
			semantic SemanticNode
			focused  bool
		}{contentDialog.Semantics(), contentModal.focused}
	}))

	triggerDialog := NewDialog(&Stack{}, "triggered", &purityModal{&purityWidget{text: "dialog"}})
	trigger := triggerDialog.Trigger("open", &purityWidget{text: "open"})
	trigger.Focus(true)
	cases = append(cases, widgetPurityCase("*DialogTrigger", trigger, func() any {
		return trigger.Semantics()
	}))

	tabChild := &purityWidget{text: "pane", height: 2}
	tabs := NewTabs(Tab{Title: "one", Of: tabChild}, Tab{Title: "two", Of: &purityWidget{text: "other"}})
	tabs.Focus(true)
	cases = append(cases, widgetPurityCase("*Tabs", tabs, func() any { return tabs.Semantics() }))

	textValue := "seed"
	textField := &Text{Label: "name", Value: Bind(&textValue)}
	textField.ensure()
	textField.Focus(true)
	textField.editor.MoveLeft()
	cases = append(cases, widgetPurityCase("*Text", textField, func() any {
		return struct {
			editor editorMeaning
			value  string
			err    error
		}{meaningOfEditor(&textField.editor), textValue, textField.Error()}
	}))

	selectedValue := "two"
	selectField := &Select[string]{
		Label: "choice", Value: Bind(&selectedValue),
	}
	selectField.SetOptions(Options("one", "two", "three"))
	selectField.ensure()
	selectField.Focus(true)
	cases = append(cases, widgetPurityCase("*Select", selectField, func() any {
		chosen, ok := selectField.Chosen()
		return struct {
			chosen Option[string]
			ok     bool
			value  string
			index  int
		}{chosen, ok, selectedValue, selectField.list.Selected()}
	}))

	takenValue := []string{"two"}
	multiField := &MultiSelect[string]{
		Label: "many", Value: Bind(&takenValue),
	}
	multiField.SetOptions(Options("one", "two", "three"))
	multiField.ensure()
	multiField.Focus(true)
	multiField.list.Select(2)
	cases = append(cases, widgetPurityCase("*MultiSelect", multiField, func() any {
		return struct {
			taken []string
			index int
		}{multiField.Taken(), multiField.list.Selected()}
	}))

	answer := true
	confirm := &Confirm{Label: "continue", Value: Bind(&answer)}
	confirm.ensure()
	confirm.Focus(true)
	cases = append(cases, widgetPurityCase("*Confirm", confirm, func() any {
		return struct {
			answer, value bool
			err           error
		}{confirm.Answer(), answer, confirm.Error()}
	}))

	completion := &Completion{MaxRows: 2}
	completion.Offer(Token{Start: 1, End: 3, Query: "tw"}, []Candidate{{Text: "two"}, {Text: "twelve"}})
	completion.Do(SelectNext)
	cases = append(cases, widgetPurityCase("*Completion", completion, func() any {
		current, currentOK := completion.Current()
		token, tokenOK := completion.Token()
		return struct {
			current   Candidate
			currentOK bool
			token     Token
			tokenOK   bool
			open      bool
		}{current, currentOK, token, tokenOK, completion.Open()}
	}))

	list := &List[string]{
		Row: func(view grid.View, _ int, item string, _ bool) {
			view.Text(0, 0, item, grid.Style{})
		},
	}
	list.SetItems([]string{"one", "two", "three"})
	list.Select(1)
	list.Focus(true)
	list.Scroll().Layout(3, 2)
	listCase := widgetPurityCase("*List", list, func() any {
		return struct {
			selected int
			focused  bool
			offset   int
		}{list.Selected(), list.Focused(), list.Scroll().Offset()}
	})
	listCase.height = 2
	cases = append(cases, listCase)

	tree := NewTree(Node[string]{Item: "root", Children: []Node[string]{{Item: "leaf"}}})
	tree.Row = func(view grid.View, _ int, row Shown[string], _ bool) {
		view.Text(0, 0, row.Item, grid.Style{})
	}
	tree.Open(0)
	tree.Select(1)
	tree.Focus(true)
	tree.Scroll().Layout(2, 2)
	treeCase := widgetPurityCase("*Tree", tree, func() any {
		return struct {
			selected int
			focused  bool
			rows     []Shown[string]
		}{tree.Selected(), tree.Focused(), tree.Rows()}
	})
	treeCase.height = 2
	cases = append(cases, treeCase)

	editor := NewEditor()
	editor.Insert("one two")
	editor.MoveLeft()
	editor.Anchor()
	editor.MoveWordLeft()
	editor.Focus(true)
	cases = append(cases, widgetPurityCase("*Editor", editor, func() any {
		return meaningOfEditor(editor)
	}))

	filter := &Filter[string]{
		Row: func(view grid.View, _ int, item string, _ fuzzy.Match, _ bool) {
			view.Text(0, 0, item, grid.Style{})
		},
	}
	filter.SetText(func(item string) string { return item })
	filter.SetItems([]string{"alpha", "beta", "alphabet"})
	filter.SetPattern("alp")
	filter.Select(1)
	filter.Focus(true)
	filter.Scroll().Layout(filter.Matched(), 2)
	filterCase := widgetPurityCase("*Filter", filter, func() any {
		return struct {
			pattern  string
			matched  int
			selected int
			focused  bool
		}{filter.Pattern(), filter.Matched(), filter.Selected(), filter.Focused()}
	})
	filterCase.height = 2
	cases = append(cases, filterCase)

	stackBase := &purityWidget{text: "base"}
	stackModal := &purityModal{&purityWidget{text: "modal", height: 2}}
	stack := NewStack(stackBase)
	stack.Push(stackModal)
	stack.Focus(true)
	cases = append(cases, widgetPurityCase("*Stack", stack, func() any {
		return struct {
			depth        int
			top          Modal
			baseFocused  bool
			modalFocused bool
		}{stack.Depth(), stack.Top(), stackBase.focused, stackModal.focused}
	}))

	formValue := "value"
	formText := &Text{Label: "field", Value: Bind(&formValue)}
	form := NewForm(formText)
	form.Focus(true)
	cases = append(cases, widgetPurityCase("*Form", form, func() any {
		return struct {
			focused Field
			value   string
			editor  editorMeaning
			err     error
		}{form.Focused(), formValue, meaningOfEditor(formText.Editor()), form.Error()}
	}))

	viewportContent := &purityWidget{text: "tall", height: 8}
	viewport := NewViewport(viewportContent)
	viewport.Scroll().Layout(8, 3)
	viewport.Scroll().By(2)
	viewport.Focus(true)
	viewportCase := widgetPurityCase("*Viewport", viewport, func() any {
		return struct {
			offset, total, window int
			following             bool
			focused               bool
		}{
			viewport.scroll.offset, viewport.scroll.total, viewport.scroll.window,
			viewport.scroll.following, viewportContent.focused,
		}
	})
	viewportCase.height = 3
	cases = append(cases, viewportCase)

	oldChild := &purityWidget{text: "old"}
	newChild := &purityWidget{text: "new"}
	container := Rows(Item{Size: layout.Flex(1), Of: oldChild})
	container.Set(Item{Size: layout.Flex(1), Of: newChild})
	cases = append(cases, widgetPurityCase("*Container", container, func() any {
		return struct {
			focused            Widget
			oldFocus, newFocus bool
		}{container.holder, oldChild.focused, newChild.focused}
	}))

	rootChild := &purityWidget{text: "root", focused: true}
	root := NewRoot(rootChild)
	cases = append(cases, drawPurityCase{
		name: "*Root", width: 24, height: 6, draw: root.Draw,
		state: func() any { return rootChild.focused },
	})

	transcript := &Transcript{}
	transcript.Resize(24)
	transcript.Append(&purityBlock{text: "kept", height: 2})
	cases = append(cases, drawPurityCase{
		name: "*Transcript", width: 24, height: 2,
		draw: func(view grid.View) { transcript.Draw(view, transcript.StartRow()) },
		state: func() any {
			return struct {
				length, width, height, start int
				first                        BlockID
			}{transcript.Len(), transcript.Width(), transcript.Height(), transcript.StartRow(), transcript.FirstBlock()}
		},
	})

	return cases
}
