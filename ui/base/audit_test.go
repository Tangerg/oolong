package base_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func TestYankPopOwnsInsertedBytesRatherThanSnappedCursor(t *testing.T) {
	var e base.Editor
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

func TestMaskedSpansUseDisplayedGeometry(t *testing.T) {
	var e base.Editor
	e.SetText("中文")
	e.SetMask("*")
	spans := e.Spans(base.Caret{}, base.Caret{Col: len("中文")}, 12)
	if len(spans) != 1 || spans[0].Width != 2 {
		t.Fatalf("masked spans: %#v", spans)
	}
}

func TestTreeAcceptsAcyclicSharedPrefix(t *testing.T) {
	nodes := make([]base.Node[string], 2)
	nodes[0].Item = "leaf"
	nodes[1] = base.Node[string]{Item: "branch", Children: nodes[:1]}
	tree := base.NewTree(nodes...)
	tree.Open(1)
	if len(tree.Rows()) != 3 {
		t.Fatalf("rows: %#v", tree.Rows())
	}
}

func TestElementPayloadReachabilityIncludesUndoAndRedo(t *testing.T) {
	var e base.Editor
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
		var e base.Editor
		e.SetText(test.source)
		rows := paintWidget(test.width, e.HeightForWidth(test.width), &e)
		if !strings.Contains(strings.Join(rows, ""), "Z") {
			t.Fatalf("%q: %q", test.source, rows)
		}
	}
}
