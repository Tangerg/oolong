package base_test

import (
	"errors"
	"image"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestFieldOwnersAdvanceOnlyForSemanticChanges(t *testing.T) {
	text := base.NewText(base.TextConfig{Initial: "hello"})
	before := text.Revision()
	text.SetText("hello")
	text.SetCursor(1)
	if text.Revision() != before {
		t.Fatal("no-op and cursor movement advanced document revision")
	}
	text.SetText("world")
	if text.Revision() != before+1 {
		t.Fatal("replacement did not advance one revision")
	}
	confirm := base.NewConfirm(base.ConfirmConfig{Initial: true})
	revision := confirm.Revision()
	confirm.Say(true)
	if confirm.Revision() != revision {
		t.Fatal("same answer advanced revision")
	}
	confirm.Say(false)
	if confirm.Revision() != revision+1 {
		t.Fatal("changed answer did not advance revision")
	}
	choices := base.NewMultiSelect(base.MultiSelectConfig[string]{Same: base.Equal[string], Options: base.Options("a", "b", "c"), Initial: []string{"b", "b", "missing"}})
	if !slices.Equal(choices.Taken(), []string{"b"}) {
		t.Fatalf("canonical choice=%v", choices.Taken())
	}
	revision = choices.Revision()
	choices.SetTaken([]string{"b", "b"})
	if choices.Revision() != revision {
		t.Fatal("equivalent set advanced revision")
	}
	choices.SetTaken([]string{"a", "b", "c"})
	revision = choices.Revision()
	choices.SetLimit(1)
	if choices.Revision() != revision+1 || !slices.Equal(choices.Taken(), []string{"a"}) {
		t.Fatal("lowering limit did not publish one canonical transition")
	}
}

func TestSharedEditSessionHasOneTextRevisionAndUndoHistory(t *testing.T) {
	session := base.NewEditSession(base.EditSessionConfig{Initial: "你好 world"})
	left := base.NewEditor(base.EditorConfig{Session: session})
	right := base.NewEditor(base.EditorConfig{Session: session})
	left.SetCursor(0, len("你好 "))
	right.SetCursor(0, len("你好 world"))
	right.Anchor()
	right.MoveLeft()
	revision := session.Revision()
	left.Insert("wide ")
	if session.Revision() != revision+1 || right.Text() != left.Text() || right.Text() != "你好 wide world" {
		t.Fatalf("shared revision/text=%d %q %q", session.Revision(), left.Text(), right.Text())
	}
	_, col := right.Cursor()
	if col != len("你好 wide worl") {
		t.Fatalf("other view caret=%d", col)
	}
	if right.Selected() != "d" {
		t.Fatalf("logical selection drifted: %q", right.Selected())
	}
	right.Undo()
	if session.Text() != "你好 world" || left.Text() != session.Text() {
		t.Fatal("undo did not belong to shared session")
	}
	left.Redo()
	if right.Text() != "你好 wide world" {
		t.Fatal("redo was split by view")
	}
	right.Scroll().By(1)
	if left.Scroll() == right.Scroll() {
		t.Fatal("two views share mutable scrolling")
	}
}

func TestSharedSessionRejectsMutationDuringAnyViewProjection(t *testing.T) {
	session := base.NewEditSession(base.EditSessionConfig{Initial: "safe"})
	first := base.NewEditor(base.EditorConfig{Session: session})
	second := base.NewEditor(base.EditorConfig{Session: session})
	element := first.Element()
	if err := element.SetPaint(func(scene.Frame) { second.SetText("polluted") }); err != nil {
		t.Fatal(err)
	}
	root, err := scene.NewRoot(scene.RootConfig{Element: element})
	if err != nil {
		t.Fatal(err)
	}
	before := session.Revision()
	func() {
		defer func() {
			if recover() == nil {
				t.Error("semantic edit was accepted in Paint")
			}
		}()
		_, _ = root.Prepare(grid.NewSurface(10, 1).View())
	}()
	if session.Text() != "safe" || session.Revision() != before {
		t.Fatal("rejected phase changed document before failing")
	}
}

func TestValidationReceiptRejectsAnEditAwayAndBack(t *testing.T) {
	fields := []struct {
		field  base.Field
		change func()
	}{
		{base.NewText(base.TextConfig{Initial: "a"}), nil},
		{base.NewConfirm(base.ConfirmConfig{}), nil},
		{base.NewSelect(base.SelectConfig[string]{Same: base.Equal[string], Options: base.Options("a", "b"), Initial: "a"}), nil},
		{base.NewMultiSelect(base.MultiSelectConfig[string]{Same: base.Equal[string], Options: base.Options("a", "b"), Initial: []string{"a"}}), nil},
	}
	for _, test := range fields {
		if err := test.field.Validate(); err != nil {
			t.Fatal(err)
		}
		receipt := test.field.Validation()
		if receipt == nil || !receipt() {
			t.Fatal("valid revision had no receipt")
		}
		switch field := test.field.(type) {
		case *base.Text:
			field.SetText("b")
			field.SetText("a")
		case *base.Confirm:
			field.Say(true)
			field.Say(false)
		case *base.Select[string]:
			field.Choose("b")
			field.Choose("a")
		case *base.MultiSelect[string]:
			field.SetTaken([]string{"b"})
			field.SetTaken([]string{"a"})
		}
		if receipt() {
			t.Fatalf("%T revived an obsolete validation receipt", test.field)
		}
	}
}

func TestFormStartedIsPendingUntilItsMatchingSettlement(t *testing.T) {
	text := base.NewText(base.TextConfig{Initial: "draft"})
	var started []base.SubmissionID
	cancelled := base.SubmissionID(0)
	form := base.NewForm(base.FormConfig{Fields: []base.Field{text}, Submit: func(id base.SubmissionID) (scene.ActionResult[base.SubmissionID], error) {
		started = append(started, id)
		return scene.ActionResult[base.SubmissionID]{State: scene.Started, Value: id}, nil
	}, Cancel: func(id base.SubmissionID) error { cancelled = id; return nil }})
	first, err := form.Submit()
	if err != nil || first.State != scene.Started || form.Submission().State != base.SubmissionPending {
		t.Fatalf("start=%v err=%v attempt=%v", first, err, form.Submission())
	}
	if cancelErr := form.Cancel(); cancelErr != nil {
		t.Fatal(cancelErr)
	}
	if cancelled != first.Value || form.Submission().State != base.SubmissionPending {
		t.Fatal("cancel request pretended to complete worker")
	}
	text.SetText("new draft")
	if !form.Settle(first.Value, base.SubmissionOutcome{}) {
		t.Fatal("matching outcome was rejected")
	}
	if state := form.Submission(); state.State != base.SubmissionSucceeded || state.Current {
		t.Fatalf("old snapshot success described edited draft: %+v", state)
	}
	second, err := form.Submit()
	if err != nil {
		t.Fatal(err)
	}
	if form.Settle(first.Value, base.SubmissionOutcome{Err: errors.New("late")}) {
		t.Fatal("old task settled new attempt")
	}
	if !form.Settle(second.Value, base.SubmissionOutcome{Cancelled: true}) || form.Submission().State != base.SubmissionCancelled {
		t.Fatal("worker cancellation not settled")
	}
	if len(started) != 2 {
		t.Fatal("unexpected submission count")
	}
}

func TestFormChecksEveryFieldAndRejectsChangesDuringValidation(t *testing.T) {
	invalid := errors.New("required")
	first := base.NewText(base.TextConfig{Check: func(string) error { return invalid }})
	count := 0
	second := base.NewText(base.TextConfig{Check: func(string) error { count++; return nil }})
	form := base.NewForm(base.FormConfig{Fields: []base.Field{first, second}})
	if _, err := form.Submit(); !errors.Is(err, invalid) || count != 1 {
		t.Fatalf("validation=%v all-check-count=%d", err, count)
	}
	first.Check = func(string) error { return nil }
	second.Check = func(string) error { first.SetText("edited after validation"); return nil }
	if _, err := form.Submit(); !errors.Is(err, base.ErrValidationSuperseded) {
		t.Fatalf("stale validation submitted: %v", err)
	}
}

func TestFormReplacementPublishesAtomically(t *testing.T) {
	first := base.NewText(base.TextConfig{Initial: "first"})
	next := base.NewText(base.TextConfig{Initial: "next"})
	form := base.NewForm(base.FormConfig{Fields: []base.Field{first}})
	root := newTestRoot(form)
	root.Draw(grid.NewSurface(20, 3).View())
	func() {
		defer func() {
			if recover() == nil {
				t.Error("duplicate field was accepted")
			}
		}()
		form.Set(next, next)
	}()
	if got := form.Fields(); len(got) != 1 || got[0] != first {
		t.Fatal("failed structure validation changed field collection")
	}
}

func TestSliderUsesTheWrittenTrackUntilNewFrameIsWritten(t *testing.T) {
	slider := base.NewSlider(base.SliderConfig{Minimum: 0, Maximum: 100})
	root := newTestRoot(slider)
	root.Draw(grid.NewSurface(11, 1).View())
	pending, err := root.root.Prepare(grid.NewSurface(21, 1).View())
	if err != nil {
		t.Fatal(err)
	}
	root.sequence++
	pending.Commit(root.sequence)
	if !root.Handle(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(5, 0)}) {
		t.Fatal("written control declined press")
	}
	if slider.Value() != 50 {
		t.Fatalf("press used admitted track rather than written: %d", slider.Value())
	}
	root.root.Wrote(root.sequence)
	root.Handle(input.Mouse{Action: input.MouseUp, Button: input.ButtonLeft, Pos: image.Pt(10, 0)})
}

func TestListRejectsAnOldHitAfterCollectionReplacement(t *testing.T) {
	list := &base.List[string]{}
	list.SetItems([]string{"old a", "old b"})
	root := newTestRoot(list)
	root.Draw(grid.NewSurface(10, 2).View())
	old := root.root.CaptureInput(input.Mouse{Action: input.MouseDown, Button: input.ButtonLeft, Pos: image.Pt(0, 1)})
	list.SetItems([]string{"new a", "new b"})
	root.Draw(grid.NewSurface(10, 2).View())
	if root.root.HandleInput(old) || list.Selected() != 0 {
		t.Fatal("old dataset hit selected a new record")
	}
}

func TestEditSessionResetPublishesOnlyAfterHistoryIsSettled(t *testing.T) {
	session := base.NewEditSession(base.EditSessionConfig{Initial: "old"})
	view := base.NewEditor(base.EditorConfig{Session: session})
	view.Insert("before")
	edited := false
	unsubscribe := session.Subscribe(func(base.ChangeMap) {
		if edited {
			return
		}
		edited = true
		view.SetCursor(0, len(view.Text()))
		view.Insert(" newer")
	})
	defer unsubscribe()
	session.Reset("fresh")
	if view.Text() != "fresh newer" {
		t.Fatalf("reentrant edit lost: %q", view.Text())
	}
	view.Undo()
	if view.Text() != "fresh" {
		t.Fatal("Reset cleared the undo entry of a newer subscriber edit")
	}
	view.Undo()
	if view.Text() != "fresh" {
		t.Fatal("reset revived the previous document's history")
	}
}

func TestEditSessionNotificationsKeepCommitOrderUnderReentry(t *testing.T) {
	session := base.NewEditSession(base.EditSessionConfig{Initial: "a"})
	editor := base.NewEditor(base.EditorConfig{Session: session})
	before := session.Revision()
	edited := false
	first := session.Subscribe(func(base.ChangeMap) {
		if !edited {
			edited = true
			editor.Insert("c")
		}
	})
	defer first()
	var revisions []uint64
	second := session.Subscribe(func(change base.ChangeMap) { revisions = append(revisions, change.After) })
	defer second()
	editor.Insert("b")
	if !slices.Equal(revisions, []uint64{before + 1, before + 2}) {
		t.Fatalf("observer received out-of-order revisions: %v", revisions)
	}
}

func TestEditSessionPublishesRevisionBeforeInvalidatingScenePredicates(t *testing.T) {
	session := base.NewEditSession(base.EditSessionConfig{Initial: "old"})
	editor := base.NewEditor(base.EditorConfig{Session: session})
	before := session.Revision()
	observedPartial := false
	if err := editor.Element().SetFocusable(func() bool {
		if session.Text() == "new" && session.Revision() == before {
			observedPartial = true
		}
		return true
	}); err != nil {
		t.Fatal(err)
	}
	root := newTestRoot(editor)
	root.Draw(grid.NewSurface(8, 1).View())
	editor.SetText("new")
	if observedPartial {
		t.Fatal("scene predicate observed new document bytes with the previous revision")
	}
}

func TestControllerInputRejectsPurePhaseBeforeAdvancingMatchers(t *testing.T) {
	type inputOwner interface {
		Element() scene.Element
		Handle(event input.Event) bool
	}
	owners := map[string]inputOwner{
		"confirm":    base.NewConfirm(base.ConfirmConfig{}),
		"text":       base.NewText(base.TextConfig{}),
		"select":     base.NewSelect(base.SelectConfig[string]{Same: base.Equal[string], Options: base.Options("a", "b")}),
		"multi":      base.NewMultiSelect(base.MultiSelectConfig[string]{Same: base.Equal[string], Options: base.Options("a", "b")}),
		"list":       &base.List[string]{},
		"filter":     &base.Filter[string]{},
		"table":      &base.Table[string]{},
		"tree":       base.NewTree(base.Node[string]{Item: "root"}),
		"settings":   &base.Settings[string]{},
		"completion": &base.Completion{},
		"slider":     base.NewSlider(base.SliderConfig{Maximum: 5}),
	}
	for name, owner := range owners {
		t.Run(name, func(t *testing.T) {
			if err := owner.Element().SetPaint(func(scene.Frame) { owner.Handle(input.Key{Rune: 'g'}) }); err != nil {
				t.Fatal(err)
			}
			root := newTestRoot(owner)
			func() {
				defer func() {
					if recover() == nil {
						t.Error("input advanced during pure presentation")
					}
				}()
				root.Draw(grid.NewSurface(8, 2).View())
			}()
		})
	}
}

func TestFormCancellationRejectsPurePhaseBeforeCallingWorker(t *testing.T) {
	requests := 0
	form := base.NewForm(base.FormConfig{Submit: func(id base.SubmissionID) (scene.ActionResult[base.SubmissionID], error) {
		return scene.ActionResult[base.SubmissionID]{State: scene.Started, Value: id}, nil
	}, Cancel: func(base.SubmissionID) error { requests++; return nil }})
	if _, err := form.Submit(); err != nil {
		t.Fatal(err)
	}
	if err := form.Element().SetPaint(func(scene.Frame) { _ = form.Cancel() }); err != nil {
		t.Fatal(err)
	}
	root := newTestRoot(form)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("paint requested worker cancellation")
			}
		}()
		root.Draw(grid.NewSurface(8, 2).View())
	}()
	if requests != 0 {
		t.Fatal("phase rejection ran after cancellation callback")
	}
}
