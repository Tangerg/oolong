package base

import (
	"math"
	"slices"
	"testing"
)

func TestEditSessionResetRegistersOnlyRealViews(t *testing.T) {
	var session EditSession
	for _, value := range []string{"first\r\nsecond", "replacement", "replacement"} {
		session.Reset(value)
		if len(session.views) != 0 {
			t.Fatalf("reset registered %d views without an editor", len(session.views))
		}
	}
	view := NewEditor(EditorConfig{Session: &session})
	session.Reset("next")
	if len(session.views) != 1 || session.views[0].Value() != view {
		t.Fatal("reset changed the set of document views")
	}
	view.Undo()
	if session.Text() != "next" {
		t.Fatalf("reset left undo history: %q", session.Text())
	}
}

func TestEditSessionExhaustionRefusesTheEntireDocumentTransaction(t *testing.T) {
	equalSnapshot := func(a, b editSnapshot) bool {
		return a.view == b.view && a.change == b.change && a.caret == b.caret &&
			slices.Equal(a.lines, b.lines) && slices.Equal(a.marks, b.marks)
	}
	for _, test := range []struct {
		name    string
		prepare func(*Editor)
		apply   func(*Editor)
	}{
		{"reset", nil, func(view *Editor) { view.Session().Reset("replacement") }},
		{"replace", nil, func(view *Editor) { view.SetText("replacement") }},
		{"single line", nil, func(view *Editor) { view.SetSingleLine(true) }},
		{"mask", nil, func(view *Editor) { view.SetMask("*") }},
		{"element", nil, func(view *Editor) { view.InsertElement(2, "new") }},
		{"undo", nil, (*Editor).Undo},
		{"redo", (*Editor).Undo, (*Editor).Redo},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := NewEditSession(EditSessionConfig{Initial: "first\nsecond"})
			view := NewEditor(EditorConfig{Session: session})
			view.InsertElement(1, "kept")
			if test.prepare != nil {
				test.prepare(view)
			}
			session.revision = math.MaxUint64
			before, lastChange := session.Text(), session.LastChange()
			marks, elementIDs := slices.Clone(session.marks), session.elementIDs
			undo, redo := cloneEditSnapshots(session.history.undo), cloneEditSnapshots(session.history.redo)
			notifications := 0
			session.Subscribe(func(ChangeMap) { notifications++ })
			func() {
				defer func() {
					if recover() == nil {
						t.Error("exhausted document accepted a transaction")
					}
				}()
				test.apply(view)
			}()
			if session.Text() != before || session.Revision() != math.MaxUint64 ||
				session.LastChange() != lastChange || session.singleLine || session.mask != "" ||
				session.elementIDs != elementIDs || !slices.Equal(session.marks, marks) ||
				!slices.EqualFunc(session.history.undo, undo, equalSnapshot) || !slices.EqualFunc(session.history.redo, redo, equalSnapshot) ||
				notifications != 0 {
				t.Fatal("refused transaction changed document state or published a change")
			}
		})
	}
}

func TestRefusedEditDoesNotSplitAnExistingTypingGroup(t *testing.T) {
	for _, test := range []struct {
		name     string
		apply    func(*Editor)
		elements bool
	}{
		{name: "set text", apply: func(view *Editor) { view.SetText("replacement") }},
		{name: "clear", apply: (*Editor).Clear},
		{name: "insert", apply: func(view *Editor) { view.Insert("replacement") }},
		{name: "replace", apply: func(view *Editor) { view.Replace(0, 1, "replacement") }},
		{name: "undo", apply: (*Editor).Undo},
		{name: "kill", apply: (*Editor).KillToStart},
		{name: "element revision", apply: func(view *Editor) { view.InsertElement(1, "replacement") }},
		{name: "element identity", apply: func(view *Editor) { view.InsertElement(1, "replacement") }, elements: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			view := NewEditor(EditorConfig{})
			view.InsertRune('a')
			session := view.Session()
			revision, identity := session.revision, session.elementIDs
			if test.elements {
				session.elementIDs.last = math.MaxUint64
			} else {
				session.revision = math.MaxUint64
			}
			func() {
				defer func() {
					if recover() == nil {
						t.Error("exhausted edit did not panic")
					}
				}()
				test.apply(view)
			}()
			session.revision, session.elementIDs = revision, identity
			view.InsertRune('b')
			view.Undo()
			if got := view.Text(); got != "" {
				t.Fatalf("refused edit split the typing group; undo left %q", got)
			}
		})
	}
}
