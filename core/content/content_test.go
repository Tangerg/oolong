package content_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/oolong/core/content"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

func TestRegistryOwnsBindingsAndDispatchesExactlyOnce(t *testing.T) {
	calls := 0
	renderer := func(_ context.Context, source string) (grid.Drawable, error) {
		calls++
		return text.NewBlock(text.BlockConfig{Lines: []text.Line{{{Text: source}}}}), nil
	}
	bindings := []content.Binding{{Format: " GO ", Render: renderer}}
	registry, err := content.New(content.Config{Bindings: bindings})
	if err != nil {
		t.Fatal(err)
	}
	bindings[0].Render = nil
	result, err := registry.Render(t.Context(), "go", "hello")
	if err != nil || result.HeightForWidth(10) != 1 || calls != 1 {
		t.Fatalf("result=%v error=%v calls=%d", result, err, calls)
	}
	if _, err := registry.Render(t.Context(), "missing", ""); !errors.Is(err, content.ErrUnknownFormat) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := registry.Render(ctx, "go", ""); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("error=%v calls=%d", err, calls)
	}
}

func TestRegistryRejectsAmbiguousAndInvalidBindings(t *testing.T) {
	render := func(context.Context, string) (grid.Drawable, error) { return new(text.Block), nil }
	for _, bindings := range [][]content.Binding{
		{{Format: "go", Render: render}, {Format: " GO ", Render: render}},
		{{Format: "bad/format", Render: render}},
		{{Format: "K", Render: render}},
		{{Format: "", Render: render}},
		{{Format: "go"}},
	} {
		if _, err := content.New(content.Config{Bindings: bindings}); err == nil {
			t.Fatalf("accepted %v", bindings)
		}
	}
}

func TestRegistryPreservesPartialResultsAndRejectsTypedNil(t *testing.T) {
	failure := errors.New("render failed")
	for _, tt := range []struct {
		name           string
		value          grid.Drawable
		err, errorWant error
	}{
		{name: "nil", errorWant: content.ErrInvalidResult},
		{name: "typed nil", value: (*text.Block)(nil), errorWant: content.ErrInvalidResult},
		{name: "error", err: failure, errorWant: failure},
		{name: "partial", value: new(text.Block), err: failure, errorWant: failure},
	} {
		t.Run(tt.name, func(t *testing.T) {
			registry, err := content.New(content.Config{Bindings: []content.Binding{{Format: "test", Render: func(context.Context, string) (grid.Drawable, error) { return tt.value, tt.err }}}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := registry.Render(t.Context(), "test", "")
			if !errors.Is(err, tt.errorWant) {
				t.Fatal(err)
			}
			if tt.name == "partial" && result != tt.value {
				t.Fatal("discarded readable content")
			}
			if tt.name != "partial" && result != nil {
				t.Fatal("returned nil interface payload")
			}
		})
	}
}
