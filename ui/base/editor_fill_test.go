package base_test

import (
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
)

func TestFillReplacesAtomicIdentityThroughTheSharedDocument(t *testing.T) {
	session := base.NewEditSession(base.EditSessionConfig{})
	editor := base.NewEditor(base.EditorConfig{Session: session})
	peer := base.NewEditor(base.EditorConfig{Session: session})
	element := editor.InsertElement(fileChip, "@document")
	value, before := editor.Text(), editor.Revision()
	root := semanticOwner(t, editor.Element())

	result, err := root.Invoke(t.Context(), editor.Element().Target(), scene.ActionFill, value)
	if err != nil || result.State != scene.Applied || result.Value != value {
		t.Fatalf("fill = %+v, %v", result, err)
	}
	if session.Revision() != before+1 || len(editor.Elements()) != 0 || len(peer.Elements()) != 0 {
		t.Fatalf("fill retained atomic identity: revision=%d writer=%+v peer=%+v", session.Revision(), editor.Elements(), peer.Elements())
	}
	result, err = root.Invoke(t.Context(), editor.Element().Target(), scene.ActionFill, value)
	if err != nil || result.State != scene.Unchanged || session.Revision() != before+1 {
		t.Fatalf("repeated plain fill = %+v, %v, revision=%d", result, err, session.Revision())
	}
	peer.Undo()
	for _, view := range []*base.Editor{editor, peer} {
		if restored := view.Elements(); len(restored) != 1 || restored[0].ID != element.ID || view.Text() != value {
			t.Fatalf("undo did not restore the shared identity: text=%q elements=%+v", view.Text(), restored)
		}
	}
}

func TestFillProjectsCanonicalDocumentChanges(t *testing.T) {
	field := base.NewText(base.TextConfig{Initial: "a b"})
	root := semanticOwner(t, field.Element())
	for _, step := range []struct {
		input string
		want  string
		state scene.ActionState
	}{{"a\nb", "a b", scene.Unchanged}, {"c\nb", "c b", scene.Applied}, {"c b", "c b", scene.Unchanged}} {
		result, err := root.Invoke(t.Context(), field.Element().Target(), scene.ActionFill, step.input)
		if err != nil || result.State != step.state || result.Value != step.want || field.Text() != step.want {
			t.Fatalf("fill %q = %+v, %v, text=%q", step.input, result, err, field.Text())
		}
	}
}

func TestFillReceiptRetainsCommitAndMaskEvidenceAcrossSubscribers(t *testing.T) {
	for _, test := range []struct {
		name, initialMask, nextMask, replacement, value string
	}{
		{"replacement", "", "", "subscriber", "subscriber"},
		{"restored original", "", "", "before", "before"},
		{"newly masked", "", "*", "subscriber", ""},
		{"unmasked during commit", "*", "", "subscriber", ""},
		{"still masked", "*", "*", "subscriber", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			session := base.NewEditSession(base.EditSessionConfig{Initial: "before"})
			editor := base.NewEditor(base.EditorConfig{Session: session})
			editor.SetMask(test.initialMask)
			before := session.Revision()
			unsubscribe := session.Subscribe(func(change base.ChangeMap) {
				if change.After == before+1 {
					editor.SetMask(test.nextMask)
					session.Reset(test.replacement)
				}
			})
			defer unsubscribe()
			root := semanticOwner(t, editor.Element())
			result, err := root.Invoke(t.Context(), editor.Element().Target(), scene.ActionFill, "input")
			if err != nil || result.State != scene.Applied || result.Value != test.value || editor.Text() != test.replacement {
				t.Fatalf("fill lost commit or masking evidence: result=%+v err=%v text=%q", result, err, editor.Text())
			}
		})
	}
}
