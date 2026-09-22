package headless_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
)

func TestYankPopOwnsInsertedBytesRatherThanSnappedCursor(t *testing.T) {
	var e headless.Editor
	for _, value := range []string{"X", "🇦"} {
		e.SetText(value)
		e.KillToStart()
	}
	e.SetText("🇧")
	e.SetCursor(0, 0)
	e.Yank()
	e.YankPop()
	if got := e.Text(); got != "X🇧" {
		t.Fatalf("yank-pop lost original suffix: %q", got)
	}
	e.Undo()
	if e.Text() != "🇦🇧" {
		t.Fatalf("undo: %q", e.Text())
	}
}

func TestControlledNormalizationEndsYankContinuation(t *testing.T) {
	value := &transformingAccessor[string]{}
	field := &headless.Text{Value: value}
	for _, body := range []string{"old", "abcdef"} {
		field.SetText(body)
		field.Do(headless.KillToStart)
	}
	field.SetText("X")
	value.transform = func(s string) string { return s[:min(len(s), 2)] }
	field.Do(headless.Yank)
	if field.Text() != "Xa" {
		t.Fatal(field.Text())
	}
	field.Do(headless.YankPop)
	if field.Text() != "Xa" {
		t.Fatalf("continued a normalized yank: %q", field.Text())
	}
}

func TestMaskedSpansUseDisplayedGeometry(t *testing.T) {
	var e headless.Editor
	e.SetText("中文")
	e.SetMask("*")
	spans := e.Spans(headless.Caret{}, headless.Caret{Col: len("中文")}, 12)
	if len(spans) != 1 || spans[0].Width != 2 {
		t.Fatalf("masked spans: %#v", spans)
	}
}

func TestFormOwnsTraversalAndMultiChordSubmission(t *testing.T) {
	answer := false
	confirm := &headless.Confirm{Value: headless.Bind(&answer)}
	next := &headless.Text{}
	form := headless.NewForm(confirm, next)
	form.Focus(true)
	form.Handle(input.Key{Code: input.Tab})
	if form.Focused() != next || answer {
		t.Fatal("Tab changed answer instead of focus")
	}
	keys := &keymap.Map{}
	keys.Bind(headless.Submit, input.Ctrl.Rune('x'), input.Ctrl.Rune('s'))
	form.Keys = keys
	submitted := false
	form.Done = func() { submitted = true }
	form.Handle(input.Key{Code: input.Character, Rune: 'x', Mods: input.Ctrl})
	form.Handle(input.Key{Code: input.Character, Rune: 's', Mods: input.Ctrl})
	if !submitted {
		t.Fatal("form lost multi-chord action")
	}
}

func TestDialogRejectsStackClosureBeforeMembershipChanges(t *testing.T) {
	value := &transformingAccessor[bool]{value: true, transform: func(bool) bool { return true }}
	stack := headless.NewStack(&focusProbe{})
	dialog := headless.NewDialog(headless.DialogConfig{Stack: stack, Open: value, Content: &panel{}})
	root := headless.NewRoot(stack)
	root.Draw(grid.NewSurface(20, 5).View())
	root.Handle(input.Key{Code: input.Esc})
	if !dialog.Open() || stack.Top() != dialog.Content() {
		t.Fatal("rejected close removed modal")
	}
	if stack.Pop() {
		t.Fatal("Pop bypassed controlled ownership")
	}
}

func TestTreeAcceptsAcyclicSharedPrefix(t *testing.T) {
	nodes := make([]headless.Node[string], 2)
	nodes[0].Item = "leaf"
	nodes[1] = headless.Node[string]{Item: "branch", Children: nodes[:1]}
	tree := headless.NewTree(nodes...)
	tree.Open(1)
	if len(tree.Rows()) != 3 {
		t.Fatalf("rows: %#v", tree.Rows())
	}
}

func TestElementPayloadReachabilityIncludesUndoAndRedo(t *testing.T) {
	var e headless.Editor
	element := e.InsertElement(1, "attachment")
	e.RemoveElement(element.ID)
	if !slices.Contains(e.RetainedElementIDs(), element.ID) {
		t.Fatal("history lost payload ownership")
	}
	e.Undo()
	if len(e.Elements()) != 1 {
		t.Fatal("undo failed to restore element")
	}
	e.Redo()
	e.ForgetHistory()
	if len(e.RetainedElementIDs()) != 0 {
		t.Fatal("discarded history retained payload")
	}
}

func TestTabReflowRechecksCarriedText(t *testing.T) {
	for _, test := range []struct {
		source string
		width  int
	}{{"abcd\tZ", 5}, {"a \tZ", 8}} {
		var e headless.Editor
		e.SetText(test.source)
		rows := paintWidget(test.width, e.HeightForWidth(test.width), &e)
		if !strings.Contains(strings.Join(rows, ""), "Z") {
			t.Fatalf("%q: %q", test.source, rows)
		}
	}
}

type notifyingBool struct {
	value   bool
	changed func()
}

func (a *notifyingBool) Value() bool { return a.value }
func (a *notifyingBool) Set(v bool) {
	a.value = v
	if a.changed != nil {
		a.changed()
	}
}

func TestDialogCanSynchronouslyPublishAcceptedClosure(t *testing.T) {
	value := &notifyingBool{value: true}
	stack := headless.NewStack(&focusProbe{})
	dialog := headless.NewDialog(headless.DialogConfig{Stack: stack, Open: value, Content: &panel{}})
	value.changed = func() { dialog.Sync() }
	root := headless.NewRoot(stack)
	root.Draw(grid.NewSurface(20, 5).View())
	root.Handle(input.Key{Code: input.Esc})
	if dialog.Open() || stack.Top() != nil {
		t.Fatal("accepted closure did not settle")
	}
}
