package content_test

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"unsafe"
	"weak"

	"github.com/Tangerg/oolong/ui/core/content"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestDocumentRetainsOnlyItsNormalizedFormat(t *testing.T) {
	document, source, err := func() (*content.StreamDocument, weak.Pointer[byte], error) {
		format := "plain" + strings.Repeat(" ", 32<<10)
		reference := weak.Make(unsafe.StringData(format)) //nolint:gosec // Observe retained backing storage without reading its bytes.
		document, err := content.NewStreamDocument(content.StreamConfig{Format: content.Format(format)})
		return document, reference, err
	}()
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	retained := source.Value() != nil
	runtime.KeepAlive(document)
	if retained {
		t.Fatal("document's short format retained discarded normalization input")
	}
}

func TestRegistryRetainsOnlyItsNormalizedBindingNames(t *testing.T) {
	registry, source, err := func() (*content.Registry, weak.Pointer[byte], error) {
		format := "plain" + strings.Repeat(" ", 32<<10)
		reference := weak.Make(unsafe.StringData(format)) //nolint:gosec // Observe retained backing storage without reading its bytes.
		render := func(context.Context, string) (grid.Drawable, error) { return new(text.Block), nil }
		registry, err := content.New(content.Config{Bindings: []content.Binding{{Format: content.Format(format), Render: render}}})
		return registry, reference, err
	}()
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	retained := source.Value() != nil
	runtime.KeepAlive(registry)
	if retained {
		t.Fatal("registry's short binding name retained discarded normalization input")
	}
}
