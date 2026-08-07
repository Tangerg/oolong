package markdown

import (
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/text"
)

func TestWrappingReleasesRowsRemovedFromTheDocument(t *testing.T) {
	doc := &Doc{rows: []row{
		{Wrapped: text.Wrapped{Line: text.Of("one", grid.Style{})}, gap: "one"},
		{Wrapped: text.Wrapped{Line: text.Of("two", grid.Style{})}, gap: "two"},
		{Wrapped: text.Wrapped{Line: text.Of("three", grid.Style{})}, gap: "three"},
	}}
	doc.wrap(10)
	for i, cached := range doc.rows[:cap(doc.rows)] {
		if !reflect.DeepEqual(cached, row{}) {
			t.Fatalf("cached row %d retained removed content %+v", i, cached)
		}
	}
}

func TestStreamCutReleasesThePublishedSourceStorage(t *testing.T) {
	prefix := strings.Repeat("published ", 1<<16)
	source := prefix + "\n\nopen tail\n"
	stream := &Stream{}
	if done := stream.Feed(source); len(done) == 0 {
		t.Fatal("the blank line did not publish the finished prefix")
	}
	if stream.held != "open tail\n" {
		t.Fatalf("open tail = %q", stream.held)
	}

	sourceStart := uintptr(unsafe.Pointer(unsafe.StringData(source)))    //nolint:gosec // Test compares allocation identity and never dereferences the address.
	tailStart := uintptr(unsafe.Pointer(unsafe.StringData(stream.held))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	if tailStart >= sourceStart && tailStart < sourceStart+uintptr(len(source)) {
		t.Fatal("the short open tail still shares the published source allocation")
	}
}
