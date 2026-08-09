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

func TestWrappingReleasesOversizedRowStorage(t *testing.T) {
	blocks := make([]Block, 1024)
	for i := range blocks {
		blocks[i].lines = []text.Line{text.Of("row", grid.Style{})}
	}
	var doc Doc
	doc.SetBlocks(blocks)
	doc.wrap(10)
	doc.SetBlocks(blocks[:1])
	doc.wrap(10)
	if cap(doc.rows) > 2*len(doc.rows)+16 {
		t.Fatalf("document retains capacity %d for %d row", cap(doc.rows), len(doc.rows))
	}
}

func TestStreamCutReleasesThePublishedSourceStorage(t *testing.T) {
	prefix := strings.Repeat("published ", 1<<16)
	source := prefix + "\n\nopen tail\n"
	stream := &Stream{}
	if done := stream.Feed(source); len(done) == 0 {
		t.Fatal("the blank line did not publish the finished prefix")
	}
	held := stream.held.String()
	if held != "open tail\n" {
		t.Fatalf("open tail = %q", held)
	}

	sourceStart := uintptr(unsafe.Pointer(unsafe.StringData(source))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	tailStart := uintptr(unsafe.Pointer(unsafe.StringData(held)))     //nolint:gosec // Test compares allocation identity and never dereferences the address.
	if tailStart >= sourceStart && tailStart < sourceStart+uintptr(len(source)) {
		t.Fatal("the short open tail still shares the published source allocation")
	}
}

func TestRenderOwnsLookGlyphsAndHighlighterResults(t *testing.T) {
	source := strings.Repeat("discarded", 1<<12) + "kept"
	glyph := source[len(source)-len("kept"):]
	provided := []text.Line{text.Of(glyph, grid.Style{})}
	look := Look{
		Glyphs: Glyphs{Divider: glyph},
		Highlight: func(string, string) []text.Line {
			return provided
		},
	}

	rule := Render("---", look)
	code := Render("```go\nx\n```", look)
	provided[0][0].Text = "changed"
	if got := rule[0].lines[0].String(); got != "kept" {
		t.Fatalf("rule glyph = %q, want owned kept", got)
	}
	if got := code[0].lines[0].String(); got != "kept" {
		t.Fatalf("highlighted code = %q after callback storage changed, want owned kept", got)
	}

	sourceStart := uintptr(unsafe.Pointer(unsafe.StringData(source))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	sourceEnd := sourceStart + uintptr(len(source))
	for name, value := range map[string]string{
		"rule": rule[0].lines[0][0].Text,
		"code": code[0].lines[0][0].Text,
	} {
		at := uintptr(unsafe.Pointer(unsafe.StringData(value))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
		if at >= sourceStart && at < sourceEnd {
			t.Errorf("%s still shares the caller's source allocation", name)
		}
	}
}

func TestStreamAppendAllocationsDoNotFollowChunkCount(t *testing.T) {
	allocations := func(chunks int) float64 {
		return testing.AllocsPerRun(5, func() {
			var stream Stream
			for range chunks {
				stream.Feed("x")
			}
		})
	}
	small := allocations(1 << 10)
	large := allocations(2 << 10)
	if large > small+4 {
		t.Fatalf("doubling chunks grew allocations from %.0f to %.0f", small, large)
	}
}

func TestStreamSearchesEachByteOfAnIncompleteLineOnce(t *testing.T) {
	var stream Stream
	for length := 1; length <= 1<<12; length++ {
		stream.Feed("x")
		if stream.scanned != 0 || stream.searched != length {
			t.Fatalf("after %d bytes, line starts at %d and search ends at %d",
				length, stream.scanned, stream.searched)
		}
	}
	stream.Feed("\n")
	if stream.scanned != 1<<12+1 || stream.searched != stream.scanned {
		t.Fatalf("completed line starts at %d and search ends at %d", stream.scanned, stream.searched)
	}
}
