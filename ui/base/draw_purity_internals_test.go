package base

import (
	"bytes"
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
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
						t.Fatalf("first HeightForWidth changed semantic state\n before: %#v\n  after: %#v", before, after)
					}
				}
				if second := tc.measure(tc.width); second != first {
					t.Fatalf("two HeightForWidth calls from the same state returned %d and %d", first, second)
				}
				if tc.state != nil {
					if after := tc.state(); !reflect.DeepEqual(after, before) {
						t.Fatalf("second HeightForWidth changed semantic state\n before: %#v\n  after: %#v", before, after)
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

type fieldProjection func(scene.Frame)

func (draw fieldProjection) Draw(frame scene.Frame) { draw(frame) }

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
			case fn.Name.Name == "HeightForWidth":
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
			t.Errorf("HeightForWidth receiver %s has no executable purity case", name)
		}
	}
	for name := range covered {
		if !slices.Contains(found, name) && !slices.Contains(measured, name) {
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

func (b purityBlock) HeightForWidth(int) int { return max(b.height, 1) }

func (w *purityWidget) Draw(frame scene.Frame) {
	frame.Text(0, 0, w.text, grid.Style{})
}

func (w *purityWidget) HeightForWidth(int) int { return max(w.height, 1) }

func (w *purityWidget) Focus(has bool) { w.focused = has }

func (*purityWidget) Handle(input.Event) bool { return false }

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
		elements: editor.Elements(), elementIDs: editor.document().elementIDs,
		matcherKeys: editor.matcher.Keys(), blurred: editor.blurred,
		look: editor.Look, placeholder: editor.Placeholder,
		keys: editor.Keys, clipboard: editor.Clipboard,
		maxRows: editor.MaxRows, singleLine: editor.SingleLine(),
		mask: editor.Mask(), gutter: editor.Gutter, cursor: editor.CursorStyle,
		undo: cloneEditorStates(editor.document().history.undo), redo: cloneEditorStates(editor.document().history.redo),
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

func widgetPurityCase(name string, widget any, state func() any) drawPurityCase {
	root := newTestRoot(widget)
	var measure func(int) int
	if sized, ok := widget.(interface{ HeightForWidth(width int) int }); ok {
		measure = sized.HeightForWidth
	}
	return drawPurityCase{
		name: name, width: 24, height: 6,
		draw: root.Draw, measure: measure, state: state,
	}
}

func headlessDrawPurityCases() []drawPurityCase {
	var cases []drawPurityCase
	candidate := Candidate{Text: "choice", Detail: "detail", Matched: []int{0, 2}}
	renderer := DefaultCompletionRenderer{}
	cases = append(cases, drawPurityCase{name: "DefaultCompletionRenderer", width: 24, height: 1, draw: func(v grid.View) { renderer.DrawRow(v, candidate, true, Look{}) }, measure: func(int) int { return renderer.Width(candidate) }})
	transcript, _ := NewTranscript(TranscriptConfig{})
	_, _ = transcript.Add(purityBlock{text: "transcript", height: 1})
	var presented TranscriptLayout
	frame := fieldProjection(func(f scene.Frame) { presented = transcript.Stage(f, 24) })
	newTestRoot(frame).Draw(grid.NewSurface(24, 1).View())
	cases = append(cases, drawPurityCase{name: "TranscriptLayout", width: 24, height: 2, draw: func(v grid.View) { presented.Draw(v, 0) }})
	field := NewText(TextConfig{Initial: "seed", Label: "name"})
	field.editor.MoveLeft()
	cases = append(cases, widgetPurityCase("*Text", field, func() any { return meaningOfEditor(&field.editor) }))
	one := NewSelect(SelectConfig[string]{Same: Equal[string], Options: Options("one", "two", "three"), Initial: "two"})
	cases = append(cases, widgetPurityCase("*Select", one, func() any {
		return struct {
			value    string
			revision uint64
		}{chosenValue(one), one.Revision()}
	}))
	many := NewMultiSelect(MultiSelectConfig[string]{Same: Equal[string], Options: Options("one", "two"), Initial: []string{"two"}})
	cases = append(cases, widgetPurityCase("*MultiSelect", many, func() any {
		return struct {
			value    []string
			revision uint64
		}{many.Taken(), many.Revision()}
	}))
	confirm := NewConfirm(ConfirmConfig{Initial: true})
	cases = append(cases, widgetPurityCase("*Confirm", confirm, func() any {
		return struct {
			value    bool
			revision uint64
		}{confirm.Answer(), confirm.Revision()}
	}))
	completion := &Completion{MaxRows: 2}
	completion.Offer(Token{Start: 1, End: 3, Query: "tw"}, []Candidate{{Text: "two"}, {Text: "twelve"}})
	completion.Do(SelectNext)
	cases = append(cases, widgetPurityCase("*Completion", completion, func() any { current, _ := completion.Current(); return current }))
	list := &List[string]{Row: func(v grid.View, _ int, item string, _ bool) { v.Text(0, 0, item, grid.Style{}) }}
	list.SetItems([]string{"one", "two", "three"})
	list.Select(1)
	cases = append(cases, widgetPurityCase("*List", list, func() any {
		return struct {
			selected int
			intent   scrollState
		}{list.Selected(), list.scroll.current}
	}))
	table := &Table[string]{Row: list.Row}
	table.SetLess(func(a, b string, _ int) bool { return a < b })
	table.SetItems([]string{"two", "one"})
	table.SortBy(0)
	cases = append(cases, widgetPurityCase("*Table", table, func() any {
		return struct {
			items    []string
			selected int
			row      func(grid.View, int, string, bool)
		}{table.Items(), table.Selected(), nil}
	}))
	tree := NewTree(Node[string]{Item: "root", Children: []Node[string]{{Item: "leaf"}}})
	tree.Open(0)
	tree.Select(1)
	cases = append(cases, widgetPurityCase("*Tree", tree, func() any { return tree.Rows() }))
	editor := NewEditor(EditorConfig{Initial: "one two"})
	editor.KillToStart()
	editor.Undo()
	editor.MoveLeft()
	editor.Anchor()
	editor.MoveWordLeft()
	cases = append(cases, widgetPurityCase("*Editor", editor, func() any { return meaningOfEditor(editor) }))
	filter := &Filter[string]{}
	filter.SetItems([]string{"alpha", "beta", "alphabet"}, func(item string) string { return item })
	filter.SetPattern("alp")
	filter.Select(1)
	cases = append(cases, widgetPurityCase("*Filter", filter, func() any {
		return struct {
			pattern  string
			selected int
		}{filter.Pattern(), filter.Selected()}
	}))
	formField := NewText(TextConfig{Initial: "value"})
	form := NewForm(FormConfig{Fields: []Field{formField}})
	cases = append(cases, widgetPurityCase("*Form", form, func() any {
		return struct {
			value      string
			submission Submission
		}{formField.Text(), form.Submission()}
	}))
	viewport := NewViewport(ViewportConfig{Content: testElement(&purityWidget{text: "tall", height: 8})})
	cases = append(cases, widgetPurityCase("*Viewport", viewport, func() any { return viewport.scroll.current }))
	tabs := NewTabs(TabsConfig{Items: []Tab{{Key: "a", Title: "one", Content: testElement(&purityWidget{text: "pane"})}, {Key: "b", Title: "two", Content: testElement(&purityWidget{text: "other"})}}})
	cases = append(cases, widgetPurityCase("*Tabs", tabs, func() any { return tabs.Selected() }))
	button := NewButton(ButtonConfig{Label: "activate", Activate: func(context.Context) (scene.ActionResult[struct{}], error) {
		return scene.ActionResult[struct{}]{State: scene.Applied}, nil
	}})
	cases = append(cases, widgetPurityCase("*Button", button, func() any { return button.Label() }))
	return cases
}
