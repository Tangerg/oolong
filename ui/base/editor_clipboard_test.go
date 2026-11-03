package base_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/clipboard"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
)

type directedClipboard struct {
	wire    clipboard.Channel
	stream  *input.Stream
	receive func(string, error)
	result  <-chan directedReadResult
}

type directedReadResult struct {
	value string
	err   error
}

func (*directedClipboard) Copy(string) bool { return false }

func (c *directedClipboard) Read(ctx context.Context, receive func(string, error)) error {
	admitted := make(chan struct{})
	result := make(chan directedReadResult, 1)
	go func() {
		value, err := c.wire.Read(ctx, clipboard.System, func(string) error { close(admitted); return nil })
		result <- directedReadResult{value, err}
	}()
	select {
	case <-admitted:
		c.receive, c.result = receive, result
		return nil
	case result := <-result:
		return result.err
	}
}

func (c *directedClipboard) answer(t *testing.T, value string) {
	t.Helper()
	if c.stream == nil {
		c.stream = input.NewStream(input.StreamConfig{Clipboard: &c.wire})
	}
	sequence, ok := c.wire.Copy(clipboard.System, value)
	if !ok {
		t.Fatal("answer exceeds protocol limit")
	}
	if events := c.stream.Feed([]byte(sequence), time.Now()); len(events) != 0 {
		t.Fatalf("requested clipboard answer became input: %+v", events)
	}
	result := <-c.result
	c.receive(result.value, result.err)
}

func TestEditorClipboardReplyKeepsItsOriginalRecipientAfterFocusMoves(t *testing.T) {
	transport := &directedClipboard{}
	first, second := &base.Editor{Clipboard: transport}, &base.Editor{}
	group := scene.NewGroup(scene.GroupConfig{Items: []scene.Item{
		{Key: "first", Element: first.Element()}, {Key: "second", Element: second.Element()},
	}})
	root, err := scene.NewRoot(scene.RootConfig{Element: group.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := first.Element().RequestFocus(); err != nil {
		t.Fatal(err)
	}
	completed := false
	if err := first.Paste(t.Context(), func(err error) {
		completed = true
		if err != nil {
			t.Error(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := second.Element().RequestFocus(); err != nil {
		t.Fatal(err)
	}
	transport.answer(t, "for first")
	if !completed || first.Text() != "for first" || second.Text() != "" {
		t.Fatalf("directed paste: first=%q second=%q completed=%t", first.Text(), second.Text(), completed)
	}
	for _, event := range transport.stream.Feed([]byte("\x1b[200~for second\x1b[201~"), time.Now()) {
		root.Handle(event)
	}
	if second.Text() != "for second" || first.Text() != "for first" {
		t.Fatalf("native paste: first=%q second=%q", first.Text(), second.Text())
	}
}

func TestEditorClipboardReplyCannotCrossUnmountAndRemount(t *testing.T) {
	transport := &directedClipboard{}
	editor := &base.Editor{Clipboard: transport}
	root, err := scene.NewRoot(scene.RootConfig{Element: editor.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	var result error
	if err := editor.Paste(t.Context(), func(err error) { result = err }); err != nil {
		t.Fatal(err)
	}
	other := (&base.Editor{}).Element()
	if err := root.SetContent(other); err != nil {
		t.Fatal(err)
	}
	root.FlushNotifications()
	if err := root.SetContent(editor.Element()); err != nil {
		t.Fatal(err)
	}
	transport.answer(t, "obsolete")
	if !errors.Is(result, base.ErrPasteStale) || editor.Text() != "" {
		t.Fatalf("remount revived paste: text=%q error=%v", editor.Text(), result)
	}
}

func TestStandalonePasteCannotSurviveAnInterveningSceneLifetime(t *testing.T) {
	transport := &clip{}
	editor := &base.Editor{Clipboard: transport}
	var result error
	if err := editor.Paste(t.Context(), func(err error) { result = err }); err != nil {
		t.Fatal(err)
	}
	root, err := scene.NewRoot(scene.RootConfig{Element: editor.Element()})
	if err != nil {
		t.Fatal(err)
	}
	root.Close()
	transport.receive("obsolete", nil)
	if !errors.Is(result, base.ErrPasteStale) || editor.Text() != "" {
		t.Fatalf("standalone request revived: text=%q error=%v", editor.Text(), result)
	}
	if err := editor.Paste(t.Context(), func(error) { t.Error("detached request completed") }); !errors.Is(err, scene.ErrDetached) {
		t.Fatalf("detached admission = %v", err)
	}
}

func TestEditorClipboardReplyRequiresItsOriginalEditLocation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*base.Editor)
	}{
		{"document", func(e *base.Editor) { e.SetText("replacement") }},
		{"cursor", func(e *base.Editor) { e.SetCursor(0, 1) }},
		{"selection", func(e *base.Editor) { e.SelectAll() }},
		{"shared session", func(e *base.Editor) { e.Session().Reset("shared change") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := &clip{}
			editor := base.NewEditor(base.EditorConfig{Initial: "original"})
			editor.Clipboard = transport
			var result error
			if err := editor.Paste(t.Context(), func(err error) { result = err }); err != nil {
				t.Fatal(err)
			}
			test.change(editor)
			before := editor.Text()
			transport.receive("obsolete", nil)
			if !errors.Is(result, base.ErrPasteStale) || editor.Text() != before {
				t.Fatalf("stale edit location: text=%q error=%v", editor.Text(), result)
			}
		})
	}
}

func TestStandaloneEditorPasteAndEmptyClipboardHaveExplicitResults(t *testing.T) {
	transport := &clip{}
	editor := base.NewEditor(base.EditorConfig{Initial: "original"})
	editor.Clipboard = transport
	editor.SelectAll()
	completed := 0
	complete := func(err error) {
		completed++
		if err != nil {
			t.Error(err)
		}
	}
	if err := editor.Paste(t.Context(), complete); err != nil {
		t.Fatal(err)
	}
	transport.receive("", nil)
	if editor.Text() != "original" || editor.Selected() != "original" || completed != 1 {
		t.Fatalf("empty read changed selection: text=%q selection=%q completed=%d", editor.Text(), editor.Selected(), completed)
	}
	if err := editor.Paste(t.Context(), complete); err != nil {
		t.Fatal(err)
	}
	transport.receive("replacement", nil)
	if editor.Text() != "replacement" || completed != 2 {
		t.Fatalf("standalone completion: text=%q completed=%d", editor.Text(), completed)
	}
}

func TestEditorClipboardCancellationAndFailuresAreExplicit(t *testing.T) {
	transport := &clip{}
	editor := &base.Editor{Clipboard: transport}
	ctx, cancel := context.WithCancel(t.Context())
	if err := editor.Paste(ctx, func(error) { t.Error("cancelled callback ran") }); err != nil {
		t.Fatal(err)
	}
	cancel()
	transport.receive("cancelled", nil)
	if editor.Text() != "" {
		t.Fatalf("cancelled read inserted %q", editor.Text())
	}
	problem := errors.New("read failed")
	var result error
	if err := editor.Paste(t.Context(), func(err error) { result = err }); err != nil {
		t.Fatal(err)
	}
	transport.receive("", problem)
	if !errors.Is(result, problem) || editor.PasteError() != nil {
		t.Fatalf("programmatic failure = %v, keyboard feedback = %v", result, editor.PasteError())
	}
	editor.Do(base.Paste)
	transport.receive("", problem)
	if !errors.Is(editor.PasteError(), problem) {
		t.Fatalf("keyboard failure = %v", editor.PasteError())
	}
	transport.refuse = true
	editor.Do(base.Paste)
	if !errors.Is(editor.PasteError(), errors.ErrUnsupported) {
		t.Fatalf("keyboard admission refusal = %v", editor.PasteError())
	}
	if err := editor.Paste(t.Context(), nil); err == nil {
		t.Fatal("missing completion receiver accepted")
	}
}

func TestTextExposesItsEditorsPasteResultAndKeyboardFailure(t *testing.T) {
	transport := &clip{}
	field := base.NewText(base.TextConfig{Clipboard: transport})
	problem := errors.New("clipboard denied")
	field.Do(base.Paste)
	transport.receive("", problem)
	if !errors.Is(field.PasteError(), problem) {
		t.Fatalf("field paste failure = %v", field.PasteError())
	}
	completed := false
	if err := field.Paste(t.Context(), func(err error) {
		completed = true
		if err != nil {
			t.Error(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	transport.receive("one\ntwo", nil)
	if !completed || field.Text() != "one two" || !errors.Is(field.PasteError(), problem) {
		t.Fatalf("field completion: text=%q completed=%t keyboard=%v", field.Text(), completed, field.PasteError())
	}
}
