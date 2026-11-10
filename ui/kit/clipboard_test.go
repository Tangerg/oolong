package kit_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

type inputClipboard struct{ receive func(string, error) }

func (*inputClipboard) Copy(string) bool { return false }

func (c *inputClipboard) Read(_ context.Context, receive func(string, error)) error {
	c.receive = receive
	return nil
}

func TestPasteFailureIsVisibleThroughTheDefaultInputAndComposerSkins(t *testing.T) {
	for _, kind := range []string{"input", "composer"} {
		t.Run(kind, func(t *testing.T) {
			transport := &inputClipboard{}
			var element scene.Element
			var paste func(keymap.Action) bool
			var measure func(int) int
			if kind == "input" {
				field := kit.NewInput(kit.InputConfig{Initial: "draft", Clipboard: transport})
				element, paste, measure = field.Element(), field.Controller().Do, field.HeightForWidth
			} else {
				composer := kit.NewComposer(kit.ComposerConfig{Initial: "draft", Clipboard: transport, MaxRows: 1})
				element, paste, measure = composer.Element(), composer.Controller().Do, composer.HeightForWidth
			}
			root, err := scene.NewRoot(scene.RootConfig{Element: element})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(root.Close)
			problem := errors.New("clipboard unavailable")
			paste(base.Paste)
			transport.receive("", problem)
			if height := measure(40); height != 2 {
				t.Fatalf("measured rows = %d, want value and feedback", height)
			}
			surface := grid.NewSurface(40, 2)
			frame, err := root.Prepare(surface.View())
			if err != nil {
				t.Fatal(err)
			}
			frame.Commit(1)
			if !strings.Contains(strings.Join(surface.Rows(), "\n"), problem.Error()) {
				t.Fatalf("default skin lost feedback: %q", surface.Rows())
			}
			observation, err := root.Observe(scene.Query{})
			if err != nil {
				t.Fatal(err)
			}
			for _, node := range observation.Nodes {
				if node.Role == scene.RoleTextBox && node.State.Has(scene.StateInvalid) && node.Description == problem.Error() {
					return
				}
			}
			t.Fatalf("default skin lost semantic feedback: %+v", observation.Nodes)
		})
	}
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
