package base

// Tests of what this package keeps to itself.
//
// The undo history's bound is a promise about memory in a long-lived process, not
// about behaviour a caller can see: an editor that kept every keystroke for a
// session that runs all day is a leak with a friendly name. Nothing outside can
// ask how many steps are held, so this asks from inside.

import (
	"image"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"github.com/Tangerg/oolong/ui/core/scene"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestEditorUndoHistoryIsBounded(t *testing.T) {
	// An unbounded history in a long-lived process is a leak with a friendly name.
	e := &Editor{}
	for i := range maxUndo + 50 {
		e.Insert("x")
		e.MoveLeft()
		_ = i
	}
	if len(e.document().history.undo) > maxUndo {
		t.Fatalf("history holds %d steps, want at most %d", len(e.document().history.undo), maxUndo)
	}
}

func TestEditorHistoryClearsPoppedSnapshots(t *testing.T) {
	history := editorHistory{}
	history.record(editorState{lines: []string{"before"}, marks: []text.Mark{{ID: 1}}})
	if _, ok := history.back(editorState{lines: []string{"after"}}); !ok {
		t.Fatal("undo stack reported empty")
	}
	for i, state := range history.undo[:cap(history.undo)] {
		if state.lines != nil || state.marks != nil {
			t.Fatalf("popped undo slot %d retained %+v", i, state)
		}
	}
	if _, ok := history.forward(editorState{lines: []string{"current"}}); !ok {
		t.Fatal("redo stack reported empty")
	}
	for i, state := range history.redo[:cap(history.redo)] {
		if state.lines != nil || state.marks != nil {
			t.Fatalf("popped redo slot %d retained %+v", i, state)
		}
	}
}

type retainedWidget struct{ payload []byte }

func (*retainedWidget) Draw(scene.Frame)       {}
func (*retainedWidget) HeightForWidth(int) int { return 1 }

type retainedField struct {
	retainedWidget
	element scene.Element
}

func (f *retainedField) Element() scene.Element {
	if f.element.IsZero() {
		f.element = scene.NewElement(scene.ElementConfig{Paint: f.Draw, Focusable: func() bool { return true }})
	}
	return f.element
}
func (*retainedField) Revision() uint64 { return 0 }

func (*retainedField) Handle(input.Event) bool { return false }
func (*retainedField) Focus(bool)              {}
func (*retainedField) Prompt() string          { return "" }
func (*retainedField) Validate() error         { return nil }
func (*retainedField) Validation() func() bool { return func() bool { return true } }
func (*retainedField) Error() error            { return nil }

type retainedModal struct{ retainedWidget }

func (*retainedModal) Handle(input.Event) bool            { return false }
func (*retainedModal) Place(image.Point) layout.Placement { return layout.Placement{} }

func TestComponentCachesReleaseRemovedChildren(t *testing.T) {
	children := []*retainedWidget{{payload: []byte("one")}, {payload: []byte("two")}, {payload: []byte("three")}}
	tabs := NewTabs(TabsConfig{Items: makeTabs(children)})
	tabs.Set(Tab{Title: "one", Content: testElement(children[0])})
	for i, part := range tabs.parts[:cap(tabs.parts)] {
		if i >= len(tabs.parts) && part != nil {
			t.Fatalf("tab %d retained removed part", i)
		}
	}
	fields := []*retainedField{{}, {}, {}}
	form := NewForm(FormConfig{Fields: []Field{fields[0], fields[1], fields[2]}})
	form.Set(fields[0])
	for i, field := range form.fields[:cap(form.fields)] {
		if i >= len(form.fields) && field != nil {
			t.Fatalf("field %d retained removed child", i)
		}
	}
	values := []int{1, 2, 3}
	tree := NewTree(Node[*int]{Item: &values[0]}, Node[*int]{Item: &values[1]}, Node[*int]{Item: &values[2]})
	tree.SetNodes(tree.Nodes()[:1])
	for i, row := range tree.list.items[:cap(tree.list.items)] {
		if i >= len(tree.list.items) && (row.Item != nil || row.id != 0) {
			t.Fatalf("tree row %d retained removed node", i)
		}
	}
}

func TestTreeReleasesOpenIdentitiesRemovedFromItsHierarchy(t *testing.T) {
	tree := NewTree(Node[string]{
		Item: "root",
		Children: []Node[string]{{
			Item: "branch", Children: []Node[string]{{Item: "leaf"}},
		}},
	})
	tree.Open(0)
	tree.Open(1)
	if len(tree.open) != 2 {
		t.Fatalf("open identities = %d, want 2", len(tree.open))
	}

	tree.SetNodes([]Node[string]{{Item: "replacement"}})
	if len(tree.open) != 0 {
		t.Fatalf("removed hierarchy retained open identities: %v", tree.open)
	}
}

func TestOwnedCollectionsReleaseOversizedBackingStorage(t *testing.T) {
	children := make([]*retainedWidget, 1024)
	for i := range children {
		children[i] = &retainedWidget{payload: []byte{byte(i)}}
	}
	tabs := NewTabs(TabsConfig{Items: makeTabs(children)})
	tabs.Set(Tab{Title: "one", Content: testElement(children[0])})
	if cap(tabs.parts) > 2*len(tabs.parts)+16 {
		t.Fatalf("tabs retained %d slots", cap(tabs.parts))
	}
	var list List[*retainedWidget]
	list.SetItems(children)
	list.SetItems(children[:1])
	if cap(list.items) > 2*len(list.items)+16 {
		t.Fatalf("list retains capacity %d for %d item", cap(list.items), len(list.items))
	}

	var filter Filter[*retainedWidget]
	readWidget := func(*retainedWidget) string { return "item" }
	filter.SetItems(children, readWidget)
	filter.SetItems(children[:1], readWidget)
	if cap(filter.items) > 2*len(filter.items)+16 {
		t.Fatalf("filter retains capacity %d for %d item", cap(filter.items), len(filter.items))
	}
	if cap(filter.list.items) > 2*len(filter.list.items)+16 {
		t.Fatalf("filtered list retains capacity %d for %d hit", cap(filter.list.items), len(filter.list.items))
	}

	fields := make([]Field, len(children))
	for i := range fields {
		field := &retainedField{}
		field.payload = []byte{byte(i)}
		fields[i] = field
	}
	form := NewForm(FormConfig{Fields: fields})
	form.HeightForWidth(10)
	form.Set(fields[0])
	form.HeightForWidth(10)
	if cap(form.fields) > 2*len(form.fields)+16 {
		t.Fatalf("form retains capacity %d for %d field", cap(form.fields), len(form.fields))
	}

	nodes := make([]Node[*retainedWidget], len(children))
	for i, child := range children {
		nodes[i] = Node[*retainedWidget]{Item: child}
	}
	tree := NewTree(nodes...)
	tree.Rows()
	tree.SetNodes(nodes[:1])
	tree.Rows()
	if cap(tree.list.items) > 2*len(tree.list.items)+16 {
		t.Fatalf("tree retains capacity %d for %d row", cap(tree.list.items), len(tree.list.items))
	}
}

func TestLongLivedModelsDetachConcreteStrings(t *testing.T) {
	backing := strings.Repeat("discarded ", 1024) + "owned"
	source := backing[len(backing)-len("owned"):]
	assertDetached := func(name, got string) {
		t.Helper()
		if unsafe.StringData(got) == unsafe.StringData(source) { //nolint:gosec // compare ownership; no pointer is dereferenced.
			t.Errorf("%s retained the caller's backing string", name)
		}
	}

	group := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{{Key: source, Element: scene.NewElement(scene.ElementConfig{})}}})
	assertDetached("group key", group.Items()[0].Key)

	tabs := NewTabs(TabsConfig{Items: []Tab{{Title: source}}})
	assertDetached("tab title", tabs.Items()[0].Title)

	dialog := NewDialog(DialogConfig{Title: source, Content: testElement(&retainedModal{})})
	assertDetached("dialog title", dialog.Title())
	dialog.SetDescription(source)
	assertDetached("dialog description", dialog.Description())

	slider := NewSlider(SliderConfig{Maximum: 1})
	slider.SetLabel(source)
	assertDetached("slider label", slider.Label())

	var filter Filter[string]
	filter.SetItems(nil, func(item string) string { return item })
	filter.SetPattern(source)
	assertDetached("filter pattern", filter.Pattern())

	var editor Editor
	editor.SetText(source)
	assertDetached("editor text", editor.Text())

	selectField := Select[string]{Same: Equal[string]}
	selectField.SetOptions([]Option[string]{{Label: source, Value: "value"}})
	assertDetached("option label", selectField.Options()[0].Label)
}

func makeTabs(children []*retainedWidget) []Tab {
	tabs := make([]Tab, len(children))
	for i, child := range children {
		tabs[i] = Tab{Title: strconv.Itoa(i), Content: testElement(child)}
	}
	return tabs
}
