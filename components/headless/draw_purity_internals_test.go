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
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
)

// drawPurityCase names the semantic projection that every Draw entry point is
// forbidden to change.
// Presentation snapshots and layout caches are intentionally absent: they exist to
// describe the frame just built, while the values here say what the component means.
type drawPurityCase struct {
	name    string
	width   int
	height  int
	draw    func(grid.View)
	measure func(int) int
	state   func() any
}

func TestLayoutAndDrawAreObservationallyPure(t *testing.T) {
	cases := headlessDrawPurityCases()
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

type observedAccessor[T any] struct {
	value         T
	reads, writes int
}

func (a *observedAccessor[T]) Value() T {
	a.reads++
	return a.value
}

func (a *observedAccessor[T]) Set(value T) {
	a.writes++
	a.value = value
}

func TestMeasurementDoesNotInitializeControlledChoices(t *testing.T) {
	one := &observedAccessor[string]{value: "two"}
	selection := &Select[string]{Value: one}
	selection.SetOptions(Options("one", "two"))
	if got := selection.Measure(20); got != 2 {
		t.Fatalf("select measured %d rows, want 2", got)
	}
	if selection.seeded || one.reads != 0 || one.writes != 0 {
		t.Fatalf("select measurement initialized state: seeded=%t, reads=%d, writes=%d",
			selection.seeded, one.reads, one.writes)
	}

	many := &observedAccessor[[]string]{value: []string{"two"}}
	multiple := &MultiSelect[string]{Value: many}
	multiple.SetOptions(Options("one", "two"))
	if got := multiple.Measure(20); got != 2 {
		t.Fatalf("multi-select measured %d rows, want 2", got)
	}
	if multiple.seeded || many.reads != 0 || many.writes != 0 {
		t.Fatalf("multi-select measurement initialized state: seeded=%t, reads=%d, writes=%d",
			multiple.seeded, many.reads, many.writes)
	}
}

func TestDrawingProjectsControlledFieldsWithoutInitializingThem(t *testing.T) {
	textValue := &observedAccessor[string]{value: "seed"}
	textField := &Text{Value: textValue}
	textFrame := captureDraw(t, 20, 1, NewRoot(textField).Draw)
	if !strings.Contains(textFrame.bytes, "seed") {
		t.Fatalf("text field did not project its caller-owned value: %q", textFrame.bytes)
	}
	if textField.seeded || textValue.reads == 0 || textValue.writes != 0 {
		t.Fatalf("text draw changed ownership: seeded=%t, reads=%d, writes=%d",
			textField.seeded, textValue.reads, textValue.writes)
	}

	one := &observedAccessor[string]{value: "two"}
	selection := &Select[string]{Value: one}
	selection.SetOptions(Options("one", "two"))
	captureDraw(t, 20, 2, NewRoot(selection).Draw)
	if selection.seeded || selection.list.Selected() != 0 || one.reads == 0 || one.writes != 0 {
		t.Fatalf("select draw changed ownership: seeded=%t, selected=%d, reads=%d, writes=%d",
			selection.seeded, selection.list.Selected(), one.reads, one.writes)
	}

	many := &observedAccessor[[]string]{value: []string{"two"}}
	multiple := &MultiSelect[string]{Value: many}
	multiple.SetOptions(Options("one", "two"))
	beforeTaken := slices.Clone(multiple.taken)
	captureDraw(t, 20, 2, NewRoot(multiple).Draw)
	if multiple.seeded || !slices.Equal(multiple.taken, beforeTaken) || many.reads == 0 || many.writes != 0 {
		t.Fatalf("multi-select draw changed ownership: seeded=%t, taken=%v, reads=%d, writes=%d",
			multiple.seeded, multiple.taken, many.reads, many.writes)
	}

	answer := &observedAccessor[bool]{value: true}
	confirmation := &Confirm{Value: answer}
	captureDraw(t, 20, 1, NewRoot(confirmation).Draw)
	if confirmation.answer.local || answer.reads == 0 || answer.writes != 0 {
		t.Fatalf("confirm draw changed ownership: local=%t, reads=%d, writes=%d",
			confirmation.answer.local, answer.reads, answer.writes)
	}
}

type fieldProjection func(Frame)

func (draw fieldProjection) Draw(frame Frame) { draw(frame) }

func TestDrawingProjectsLaterCallerValuesWithoutSynchronizing(t *testing.T) {
	look := Look{Taken: "x", Free: "-"}

	textValue := &observedAccessor[string]{value: "old"}
	textField := &Text{Value: textValue}
	textField.ensure()
	textValue.value = "new"
	beforeText := meaningOfEditor(&textField.editor)
	frame := captureDraw(t, 8, 1, NewRoot(textField).Draw)
	if !strings.Contains(frame.bytes, "new") || !reflect.DeepEqual(meaningOfEditor(&textField.editor), beforeText) {
		t.Fatalf("text draw did not purely project later owner value: %q", frame.bytes)
	}

	one := &observedAccessor[string]{value: "one"}
	selection := &Select[string]{Value: one}
	selection.SetOptions(Options("one", "two"))
	selection.ensure()
	one.value = "two"
	selectSurface := grid.NewSurface(8, 2)
	NewRoot(fieldProjection(func(frame Frame) { selection.drawField(frame, look) })).Draw(selectSurface.View())
	first, _ := selectSurface.CellAt(0, 0)
	second, _ := selectSurface.CellAt(0, 1)
	if first.Content() != "-" || second.Content() != "x" || selection.list.Selected() != 0 || one.writes != 0 {
		t.Fatalf("select projection marks=(%q,%q) cursor=%d writes=%d", first.Content(), second.Content(), selection.list.Selected(), one.writes)
	}

	many := &observedAccessor[[]string]{value: []string{"one"}}
	multiple := &MultiSelect[string]{Value: many}
	multiple.SetOptions(Options("one", "two"))
	multiple.ensure()
	many.value = []string{"two"}
	beforeTaken := slices.Clone(multiple.taken)
	multiSurface := grid.NewSurface(8, 2)
	NewRoot(fieldProjection(func(frame Frame) { multiple.drawField(frame, look) })).Draw(multiSurface.View())
	first, _ = multiSurface.CellAt(0, 0)
	second, _ = multiSurface.CellAt(0, 1)
	if first.Content() != "-" || second.Content() != "x" || !slices.Equal(multiple.taken, beforeTaken) || many.writes != 0 {
		t.Fatalf("multi projection marks=(%q,%q) taken=%v writes=%d", first.Content(), second.Content(), multiple.taken, many.writes)
	}

	answer := &observedAccessor[bool]{}
	confirmation := &Confirm{Value: answer}
	answer.value = true
	confirmSurface := grid.NewSurface(16, 1)
	NewRoot(fieldProjection(func(frame Frame) { confirmation.drawField(frame, look) })).Draw(confirmSurface.View())
	first, _ = confirmSurface.CellAt(0, 0)
	if first.Content() != "x" || confirmation.answer.local || answer.writes != 0 {
		t.Fatalf("confirm projection mark=%q local=%t writes=%d", first.Content(), confirmation.answer.local, answer.writes)
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

type purityWidget struct {
	text    string
	height  int
	focused bool
}

type purityBlock struct {
	text   string
	height int
}

func (b purityBlock) Draw(view grid.View) { view.Text(0, 0, b.text, grid.Style{}) }

func (b purityBlock) Measure(int) int { return max(b.height, 1) }

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

type editorMeaning struct {
	text         string
	revision     uint64
	line, column int
	selection    string
	anchor       Caret
	selecting    bool
	wantColumn   int
	rowEnd       Caret
	rowEndSet    bool
	elements     []Element
	elementIDs   identitySequence
	matcherKeys  input.Keys
	blurred      bool
	look         Look
	placeholder  string
	keys         *keymap.Map
	clipboard    Clipboard
	maxRows      int
	singleLine   bool
	mask         string
	gutter       RowGutter
	cursor       grid.CursorStyle
	undo, redo   []editorState
	kills        []editorKill
	typing       bool
	continuation editorContinuation
	yank         editorYank
}

func meaningOfEditor(editor *Editor) editorMeaning {
	line, column := editor.Cursor()
	return editorMeaning{
		text: editor.Text(), revision: editor.Revision(), line: line, column: column,
		selection: editor.Selected(), anchor: editor.anchor, selecting: editor.selecting,
		wantColumn: editor.wantColumn, rowEnd: editor.rowEnd, rowEndSet: editor.rowEndSet,
		elements: editor.Elements(), elementIDs: editor.elementIDs,
		matcherKeys: editor.matcher.Keys(), blurred: editor.blurred,
		look: editor.Look, placeholder: editor.Placeholder,
		keys: editor.Keys, clipboard: editor.Clipboard,
		maxRows: editor.MaxRows, singleLine: editor.SingleLine(),
		mask: editor.Mask(), gutter: editor.Gutter, cursor: editor.CursorStyle,
		undo: cloneEditorStates(editor.history.undo), redo: cloneEditorStates(editor.history.redo),
		kills: cloneEditorKills(editor.kills.entries), typing: editor.typing,
		continuation: editor.continuation, yank: editor.yank,
	}
}

func cloneEditorStates(states []editorState) []editorState {
	out := slices.Clone(states)
	for i := range out {
		out[i].lines = slices.Clone(out[i].lines)
		out[i].marks = slices.Clone(out[i].marks)
	}
	return out
}

func cloneEditorKills(kills []editorKill) []editorKill {
	out := slices.Clone(kills)
	for i := range out {
		out[i].value = strings.Clone(out[i].value)
		out[i].before = slices.Clone(out[i].before)
		out[i].after = slices.Clone(out[i].after)
	}
	return out
}

func widgetPurityCase(name string, widget Widget, state func() any) drawPurityCase {
	root := NewRoot(widget)
	var measure func(int) int
	if sized, ok := widget.(layout.Measurer); ok {
		measure = sized.Measure
	}
	return drawPurityCase{
		name: name, width: 24, height: 6,
		draw: root.Draw, measure: measure, state: state,
	}
}

func headlessDrawPurityCases() []drawPurityCase {
	var cases []drawPurityCase

	static := Static{Of: purityBlock{text: "static", height: 1}}
	cases = append(cases, drawPurityCase{
		name: "Static", width: 24, height: 2,
		draw: func(view grid.View) { static.Draw(Frame{View: view}) }, measure: static.Measure,
	})

	transcriptBlock := purityBlock{text: "transcript", height: 1}
	transcriptLayout := TranscriptLayout{state: transcriptState{
		blocks: []placed{{block: transcriptBlock, height: 1}}, width: 24, rows: 1,
	}}
	cases = append(cases, drawPurityCase{
		name: "TranscriptLayout", width: 24, height: 2,
		draw: func(view grid.View) { transcriptLayout.Draw(view, 0) },
	})

	contentModal := &purityModal{&purityWidget{text: "content", height: 2}}
	contentDialog := NewDialog(DialogConfig{Stack: &Stack{}, Title: "content", Content: contentModal})
	contentDialog.Show()
	contentDialog.Content().Focus(true)
	cases = append(cases, widgetPurityCase("*DialogContent", contentDialog.Content(), func() any {
		return struct {
			semantic SemanticNode
			focused  bool
		}{contentDialog.Semantics(), contentModal.focused}
	}))

	triggerDialog := NewDialog(DialogConfig{Stack: &Stack{}, Title: "triggered", Content: &purityModal{&purityWidget{text: "dialog"}}})
	trigger := triggerDialog.Trigger("open", &purityWidget{text: "open"})
	trigger.Focus(true)
	cases = append(cases, widgetPurityCase("*DialogTrigger", trigger, func() any {
		return trigger.Semantics()
	}))

	tabChild := &purityWidget{text: "pane", height: 2}
	tabs := NewTabs(TabsConfig{Items: []Tab{{Title: "one", Of: tabChild}, {Title: "two", Of: &purityWidget{text: "other"}}}})
	tabs.Focus(true)
	cases = append(cases, widgetPurityCase("*Tabs", tabs, func() any { return tabs.Semantics() }))

	textValue := "seed"
	textChecks := 0
	textField := &Text{
		Label: "name", Value: Bind(&textValue),
		Check: func(string) error { textChecks++; return nil },
	}
	textField.ensure()
	textField.Focus(true)
	textField.editor.KillToStart()
	textField.editor.Undo()
	textField.editor.MoveLeft()
	cases = append(cases, widgetPurityCase("*Text", textField, func() any {
		return struct {
			editor editorMeaning
			value  string
			err    error
			checks int
		}{meaningOfEditor(&textField.editor), textValue, textField.Error(), textChecks}
	}))

	selectedValue := "two"
	selectChecks := 0
	selectField := &Select[string]{
		Label: "choice", Value: Bind(&selectedValue),
		Check: func(string) error { selectChecks++; return nil },
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
			checks int
		}{chosen, ok, selectedValue, selectField.list.Selected(), selectChecks}
	}))

	takenValue := []string{"two"}
	multiChecks := 0
	multiField := &MultiSelect[string]{
		Label: "many", Value: Bind(&takenValue),
		Check: func([]string) error { multiChecks++; return nil },
	}
	multiField.SetOptions(Options("one", "two", "three"))
	multiField.ensure()
	multiField.Focus(true)
	multiField.list.Select(2)
	cases = append(cases, widgetPurityCase("*MultiSelect", multiField, func() any {
		return struct {
			taken  []string
			index  int
			checks int
		}{multiField.Taken(), multiField.list.Selected(), multiChecks}
	}))

	answer := true
	confirmChecks := 0
	confirm := &Confirm{
		Label: "continue", Value: Bind(&answer),
		Check: func(bool) error { confirmChecks++; return nil },
	}
	confirm.Focus(true)
	cases = append(cases, widgetPurityCase("*Confirm", confirm, func() any {
		return struct {
			answer, value bool
			err           error
			checks        int
		}{confirm.Answer(), answer, confirm.Error(), confirmChecks}
	}))

	accepts := 0
	completion := &Completion{
		MaxRows: 2,
		Accept:  func(Candidate, Token) { accepts++ },
	}
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
			accepts   int
		}{current, currentOK, token, tokenOK, completion.Open(), accepts}
	}))

	list := &List[string]{
		Row: func(view grid.View, _ int, item string, _ bool) {
			view.Text(0, 0, item, grid.Style{})
		},
	}
	list.SetItems([]string{"one", "two", "three"})
	list.Select(1)
	list.Focus(true)
	stageScrollForTest(list.Scroll(), 3, 2)
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
	stageScrollForTest(tree.Scroll(), 2, 2)
	treeCase := widgetPurityCase("*Tree", tree, func() any {
		return struct {
			selected int
			focused  bool
			rows     []Shown[string]
		}{tree.Selected(), tree.Focused(), tree.Rows()}
	})
	treeCase.height = 2
	cases = append(cases, treeCase)

	editor := &Editor{}
	editor.Insert("one two")
	editor.KillToStart()
	editor.Undo()
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
	stageScrollForTest(filter.Scroll(), filter.Matched(), 2)
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
	stageScrollForTest(viewport.Scroll(), 8, 3)
	viewport.Scroll().By(2)
	viewport.Focus(true)
	viewportCase := widgetPurityCase("*Viewport", viewport, func() any {
		return struct {
			state   scrollState
			focused bool
		}{
			viewport.scroll.current, viewportContent.focused,
		}
	})
	viewportCase.height = 3
	cases = append(cases, viewportCase)

	oldChild := &purityWidget{text: "old"}
	newChild := &purityWidget{text: "new"}
	container := NewContainer(layout.Down, Item{Size: layout.Flex(1), Of: oldChild})
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

	return cases
}
