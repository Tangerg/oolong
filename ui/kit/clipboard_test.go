package kit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

type inputClipboard struct{ receive func(string, error) }

func (*inputClipboard) Copy(string) bool { return false }

func (c *inputClipboard) Read(_ context.Context, receive func(string, error)) error {
	c.receive = receive
	return nil
}

func TestInputControllerExposesDirectedPasteAndKeyboardFailure(t *testing.T) {
	transport := &inputClipboard{}
	field := kit.NewInput(kit.InputConfig{Clipboard: transport})
	root, err := scene.NewRoot(scene.RootConfig{Element: field.Element()})
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	problem := errors.New("host read failed")
	field.Controller().Do(base.Paste)
	transport.receive("", problem)
	if !errors.Is(field.Controller().PasteError(), problem) {
		t.Fatalf("hidden input failure: %v", field.Controller().PasteError())
	}
	completed := false
	if err := field.Controller().Paste(t.Context(), func(err error) {
		completed = true
		if err != nil {
			t.Error(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	transport.receive("single\nline", nil)
	if !completed || field.Controller().Text() != "single line" {
		t.Fatalf("input paste: text=%q completed=%t", field.Controller().Text(), completed)
	}
}
