package markdown

import (
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/text"
)

func TestWrappingReleasesRowsRemovedFromTheDocument(t *testing.T) {
	doc := &Doc{places: []placedBlock{{layout: blockLayout{rows: []row{{Line: text.Of("old", grid.Style{})}}}}}}
	doc.arrange(10)
	if len(doc.places) != 0 {
		t.Fatal("removed block still retained")
	}
}

func TestReplacingADocumentReleasesItsCachedRowsBeforeAnotherLayout(t *testing.T) {
	doc := &Doc{}
	doc.SetBlocks(mustRender(t, strings.Repeat("old content ", 128), Look{}))
	doc.arrange(20)
	doc.SetBlocks(nil)
	if doc.fresh || len(doc.places) != 0 {
		t.Fatalf("invalidated document retained %d row(s)", len(doc.places))
	}
	for i, cached := range doc.places[:cap(doc.places)] {
		if !reflect.DeepEqual(cached, placedBlock{}) {
			t.Fatalf("cached row %d retained replaced content %+v", i, cached)
		}
	}
}

func TestDocCopyDetachesItsWrap(t *testing.T) {
	var original Doc
	original.SetBlocks(mustRender(t, "original words", Look{}))
	original.arrange(8)
	want := append([]placedBlock(nil), original.places...)

	copied := original
	copied.SetBlocks(mustRender(t, "replacement", Look{}))
	copied.arrange(8)

	original.arrange(8)
	if got := original.places; !reflect.DeepEqual(got, want) {
		t.Fatalf("original wrap after changing copy = %+v, want %+v", got, want)
	}
}

func TestDocCopyOwnsSubsequentAppends(t *testing.T) {
	blocks := mustRender(t, "first", Look{})
	original := Doc{blocks: make([]Block, len(blocks), len(blocks)+2)}
	copy(original.blocks, blocks)
	original.blocksOwner = &original

	copied := original
	copied.Append(mustRender(t, "copied", Look{})...)
	original.Append(mustRender(t, "original", Look{})...)

	copyBlocks := copied.Blocks()
	originalBlocks := original.Blocks()
	if got := copyBlocks[len(copyBlocks)-1].lines[0].String(); got != "copied" {
		t.Fatalf("copy's last block after original append = %q, want copied", got)
	}
	if got := originalBlocks[len(originalBlocks)-1].lines[0].String(); got != "original" {
		t.Fatalf("original's last block after copy append = %q, want original", got)
	}
}

func TestWrappingReleasesOversizedRowStorage(t *testing.T) {
	blocks := make([]Block, 1024)
	for i := range blocks {
		blocks[i].lines = []text.Line{text.Of("row", grid.Style{})}
	}
	var doc Doc
	doc.SetBlocks(blocks)
	doc.arrange(10)
	doc.SetBlocks(blocks[:1])
	doc.arrange(10)
	if cap(doc.places) > 2*len(doc.places)+16 {
		t.Fatalf("document retains capacity %d for %d row", cap(doc.places), len(doc.places))
	}
}

func TestRenderOwnsLookGlyphsAndHighlighterResults(t *testing.T) {
	source := strings.Repeat("discarded", 1<<12) + "kept"
	glyph := source[len(source)-len("kept"):]
	provided := []text.Line{text.Of(glyph, grid.Style{})}
	look := Look{Glyphs: Glyphs{Divider: glyph}}
	look.SetRenderer(FencedCode, func(string, string) (grid.Drawable, error) {
		return text.NewBlock(text.BlockConfig{Lines: provided}), nil
	})

	rule := mustRender(t, "---", look)
	code := mustRender(t, "```go\nx\n```", look)
	provided[0][0].Text = "changed"
	if got := rule[0].lines[0].String(); got != "kept" {
		t.Fatalf("rule glyph = %q, want owned kept", got)
	}
	if got := code[0].Rows(20)[0].Text; got != "kept" {
		t.Fatalf("highlighted code = %q after callback storage changed, want owned kept", got)
	}

	sourceStart := uintptr(unsafe.Pointer(unsafe.StringData(source))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	sourceEnd := sourceStart + uintptr(len(source))
	for name, value := range map[string]string{
		"rule": rule[0].lines[0][0].Text,
		"code": code[0].Rows(20)[0].Text,
	} {
		at := uintptr(unsafe.Pointer(unsafe.StringData(value))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
		if at >= sourceStart && at < sourceEnd {
			t.Errorf("%s still shares the caller's source allocation", name)
		}
	}
}
