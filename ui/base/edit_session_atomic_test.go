package base_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestRepeatedTextMapsPeersThroughTheActualEditAndHistory(t *testing.T) {
	s := base.NewEditSession(base.EditSessionConfig{Initial: "aaa"})
	writer := base.NewEditor(base.EditorConfig{Session: s})
	reader := base.NewEditor(base.EditorConfig{Session: s})
	reader.SetCursor(0, 1)
	reader.Anchor()
	reader.MoveRight()
	check := func(text string, caret, start, oldEnd, newEnd int) {
		t.Helper()
		m := s.LastChange()
		_, c := reader.Cursor()
		if s.Text() != text || c != caret || reader.Selected() != "a" || m.Start != start || m.OldEnd != oldEnd || m.NewEnd != newEnd {
			t.Fatalf("document=%q peer caret=%d selection=%q map=%+v; want %q, %d, a, [%d,%d)→%d", s.Text(), c, reader.Selected(), m, text, caret, start, oldEnd, newEnd)
		}
	}
	writer.InsertRune('a')
	check("aaaa", 3, 0, 0, 1)
	writer.InsertRune('a')
	check("aaaaa", 4, 1, 1, 2)
	writer.Undo()
	check("aaa", 2, 0, 2, 0)
	writer.Redo()
	check("aaaaa", 4, 0, 0, 2)
	writer.SetCursor(0, 0)
	writer.DeleteForward()
	check("aaaa", 3, 0, 1, 0)
}

func TestElementInsertionPublishesIdentityBeforeSubscribersRun(t *testing.T) {
	s := base.NewEditSession(base.EditSessionConfig{})
	editor := base.NewEditor(base.EditorConfig{Session: s})
	var beforeReset []base.Element
	notifications := 0
	unsubscribe := s.Subscribe(func(base.ChangeMap) {
		notifications++
		if notifications == 1 {
			beforeReset = editor.Elements()
			s.Reset("new")
		}
	})
	defer unsubscribe()
	element := editor.InsertElement(1, "tag")
	if len(beforeReset) != 1 || beforeReset[0].ID != element.ID {
		t.Errorf("subscriber saw elements %+v before replacing insertion %+v", beforeReset, element)
	}
	if s.Text() != "new" || len(editor.Elements()) != 0 || notifications != 2 {
		t.Fatalf("older insertion resumed over a newer edit: text=%q marks=%+v notifications=%d", s.Text(), editor.Elements(), notifications)
	}
}

func TestMaskProtectsEveryViewAndWriterOfTheSharedSession(t *testing.T) {
	s := base.NewEditSession(base.EditSessionConfig{Initial: "secret"})
	first, second := base.NewEditor(base.EditorConfig{Session: s}), base.NewEditor(base.EditorConfig{Session: s})
	root, err := scene.NewRoot(scene.RootConfig{Element: second.Element()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(root.Close)
	revision := s.Revision()
	wake := root.Changes()
	first.SetMask("*")
	if s.Revision() != revision {
		t.Fatal("presentation produced a fictitious document edit")
	}
	select {
	case <-wake:
	default:
		t.Error("sensitive display did not invalidate other mounted view")
	}
	observation, err := root.Observe(scene.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(observation.Nodes) != 1 || !observation.Nodes[0].Sensitive || observation.Nodes[0].Value != "" {
		t.Errorf("second view exposed masked value in observation: %+v", observation.Nodes)
	}
	second.SetText("a\nb")
	if s.Text() != "a b" {
		t.Errorf("peer bypassed one-line storage: %q", s.Text())
	}
	s.Reset("c\nd")
	if s.Text() != "c d" {
		t.Errorf("Reset bypassed one-line storage: %q", s.Text())
	}
	second.SetMask("")
	if first.Mask() != "" {
		t.Fatal("views disagree after clearing the session mask")
	}
}

func TestSharedHistoryResetEndsTheTypingGroup(t *testing.T) {
	for _, reset := range []string{"reset unchanged", "forget from peer", "reset from subscriber"} {
		t.Run(reset, func(t *testing.T) {
			s := base.NewEditSession(base.EditSessionConfig{})
			editor := base.NewEditor(base.EditorConfig{Session: s})
			peer := base.NewEditor(base.EditorConfig{Session: s})
			if reset == "reset from subscriber" {
				cancel := s.Subscribe(func(change base.ChangeMap) {
					if change.After == 1 {
						s.Reset("stable")
					}
				})
				defer cancel()
			}
			editor.InsertRune('a')
			switch reset {
			case "reset unchanged":
				s.Reset(s.Text())
			case "forget from peer":
				peer.ForgetHistory()
			}
			before := s.Text()
			editor.InsertRune('b')
			editor.Undo()
			if s.Text() != before {
				t.Fatalf("undo after ending the previous group = %q; want %q", s.Text(), before)
			}
		})
	}
}

func TestYankCannotResumeAcrossASubscribersEdit(t *testing.T) {
	for _, op := range []string{"yank", "cycle"} {
		t.Run(op, func(t *testing.T) {
			var e base.Editor
			for _, v := range []string{"one", "two"} {
				e.SetText(v)
				e.KillToStart()
			}
			if op == "cycle" {
				e.Yank()
			}
			s := e.Session()
			changed := false
			cancel := s.Subscribe(func(base.ChangeMap) {
				if !changed {
					changed = true
					s.Reset("later")
				}
			})
			defer cancel()
			if op == "yank" {
				e.Yank()
			} else {
				e.YankPop()
			}
			e.YankPop()
			if s.Text() != "later" {
				t.Fatalf("superseded yank overwrote the subscriber's edit: %q", s.Text())
			}
		})
	}
}

func TestMaskedFillReceiptCannotExposeTheValue(t *testing.T) {
	var editor base.Editor
	editor.SetMask("*")
	session := editor.Session()
	cancel := session.Subscribe(func(base.ChangeMap) {
		if editor.Text() == "input" {
			session.Reset("replacement secret")
		}
	})
	defer cancel()
	field := new(base.Text)
	field.SetMask("*")
	for _, test := range []struct {
		name    string
		element scene.Element
		want    string
		read    func() string
	}{
		{"editor", editor.Element(), "replacement secret", editor.Text},
		{"text field", field.Element(), "input", field.Text},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := newTestRoot(test.element)
			for _, value := range []string{"input", test.want} {
				result, err := root.root.Invoke(t.Context(), test.element.Target(), scene.ActionFill, value)
				if err != nil || result.Value != "" || test.read() != test.want {
					t.Fatalf("masked fill returned a value or failed to fill (err=%v)", err)
				}
			}
		})
	}
}
