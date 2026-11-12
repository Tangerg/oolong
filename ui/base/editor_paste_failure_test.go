package base_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func TestKeyboardPastePreservesReadFailureAfterItsEditLocationChanges(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*base.Editor)
	}{
		{"document", func(editor *base.Editor) { editor.SetText("replacement") }},
		{"cursor", func(editor *base.Editor) { editor.SetCursor(0, 1) }},
		{"selection", (*base.Editor).SelectAll},
		{"shared session", func(editor *base.Editor) { editor.Session().Reset("shared change") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := &queuedClipboard{}
			editor := base.NewEditor(base.EditorConfig{Initial: "original", Clipboard: transport})
			editor.Do(base.Paste)
			test.change(editor)
			before := editor.Text()
			failure := errors.New("clipboard read failed")
			transport.receivers[0]("unaccepted", failure)
			if !errors.Is(editor.PasteError(), failure) || editor.Text() != before {
				t.Fatalf("read failure = %v, text = %q", editor.PasteError(), editor.Text())
			}
		})
	}
}

func TestKeyboardPastePreservesReadFailuresContainingStaleness(t *testing.T) {
	for _, failure := range []error{
		fmt.Errorf("clipboard read failed: %w", base.ErrPasteStale),
		errors.Join(base.ErrPasteStale, errors.New("clipboard read failed")),
	} {
		t.Run(failure.Error(), func(t *testing.T) {
			transport := &queuedClipboard{}
			editor := base.NewEditor(base.EditorConfig{Initial: "original", Clipboard: transport})
			editor.Do(base.Paste)
			transport.receivers[0]("unaccepted", failure)
			if !errors.Is(editor.PasteError(), failure) || editor.Text() != "original" {
				t.Fatalf("read failure = %v, text = %q", editor.PasteError(), editor.Text())
			}
		})
	}
}

func TestProgrammaticPasteReportsReadFailureWithoutInterpretingItsLocation(t *testing.T) {
	transport := &queuedClipboard{}
	editor := base.NewEditor(base.EditorConfig{Initial: "original", Clipboard: transport})
	var completed bool
	var result error
	if err := editor.Paste(t.Context(), func(err error) { completed, result = true, err }); err != nil {
		t.Fatal(err)
	}
	editor.SetText("replacement")
	failure := errors.New("clipboard read failed")
	transport.receivers[0]("unaccepted", failure)
	if !completed || !errors.Is(result, failure) || errors.Is(result, base.ErrPasteStale) || editor.Text() != "replacement" || editor.PasteError() != nil {
		t.Fatalf("completion = %t, result = %v, text = %q, feedback = %v", completed, result, editor.Text(), editor.PasteError())
	}
}

func TestKeyboardPasteQuietlyDiscardsASuccessfulReadWhoseLocationChanged(t *testing.T) {
	transport := &queuedClipboard{}
	editor := base.NewEditor(base.EditorConfig{Initial: "original", Clipboard: transport})
	editor.Do(base.Paste)
	editor.SetText("replacement")
	transport.receivers[0]("unaccepted", nil)
	if editor.PasteError() != nil || editor.Text() != "replacement" {
		t.Fatalf("stale success = %v, text = %q", editor.PasteError(), editor.Text())
	}
}
