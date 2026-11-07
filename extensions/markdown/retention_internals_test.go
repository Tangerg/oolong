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
	doc := &Snapshot{}
	doc.layout.Store(&documentLayout{width: 8, places: []placedBlock{{layout: blockLayout{rows: []text.Wrapped{{Line: text.Of("old", grid.Style{})}}}}}})
	if len(doc.arrange(10).places) != 0 {
		t.Fatal("removed block still retained")
	}
}

func TestReplacingADocumentReleasesItsCachedRowsBeforeAnotherLayout(t *testing.T) {
	doc := &Doc{}
	doc.SetBlocks(mustRender(t, strings.Repeat("old content ", 128), Look{}))
	if len(doc.view.arrange(20).places) == 0 {
		t.Fatal("fixture has no cached geometry")
	}
	doc.SetBlocks(nil)
	if current := doc.view.layout.Load(); current != nil {
		t.Fatal("replacement retained the discarded placement cache")
	}
}

func TestDocCopyDetachesItsWrap(t *testing.T) {
	var original Doc
	original.SetBlocks(mustRender(t, "original words", Look{}))
	want := append([]placedBlock(nil), original.view.arrange(8).places...)

	copied := original
	copied.SetBlocks(mustRender(t, "replacement", Look{}))
	copied.view.arrange(8)

	if got := original.view.arrange(8).places; !reflect.DeepEqual(got, want) {
		t.Fatalf("original wrap after changing copy = %+v, want %+v", got, want)
	}
}

func TestDocCopyOwnsSubsequentAppends(t *testing.T) {
	blocks := mustRender(t, "first", Look{})
	original := Doc{view: &Snapshot{blocks: make([]Block, len(blocks), len(blocks)+2)}}
	copy(original.view.blocks, blocks)

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
	doc.view.arrange(10)
	doc.SetBlocks(blocks[:1])
	places := doc.view.arrange(10).places
	if cap(places) > 2*len(places)+16 {
		t.Fatalf("document retains capacity %d for %d row", cap(places), len(places))
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
	if got := rule[0].rule.Text; got != "kept" {
		t.Fatalf("rule glyph = %q, want owned kept", got)
	}
	if got := code[0].Rows(20)[0].Text; got != "kept" {
		t.Fatalf("highlighted code = %q after callback storage changed, want owned kept", got)
	}

	sourceStart := uintptr(unsafe.Pointer(unsafe.StringData(source))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	sourceEnd := sourceStart + uintptr(len(source))
	for name, value := range map[string]string{
		"rule": rule[0].rule.Text,
		"code": code[0].Rows(20)[0].Text,
	} {
		at := uintptr(unsafe.Pointer(unsafe.StringData(value))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
		if at >= sourceStart && at < sourceEnd {
			t.Errorf("%s still shares the caller's source allocation", name)
		}
	}
}

func TestNestedListItemsOwnMarkersAndShareQuotationRails(t *testing.T) {
	const depth = 32
	indent := strings.Repeat("  ", depth)
	source := strings.Repeat("- ", depth) + "> first\n" + indent + ">\n" +
		indent + "> again\n" + indent + ">\n" + indent + "> - child\n" + indent + "> - next"
	blocks := mustRender(t, source, Look{Glyphs: Glyphs{Bullet: "-", Bar: "|"}})
	for level := range depth {
		if len(blocks) != 1 {
			t.Fatalf("level %d has %d blocks, want one nested item", level, len(blocks))
		}
		item, ok := blocks[0].content.(*listItem)
		if !ok || item.marker == nil || item.marker.next != nil {
			t.Fatalf("level %d does not own exactly its marker", level)
		}
		blocks = item.body.blocks
	}
	if len(blocks) != 4 || blocks[0].prefix.rails == nil {
		t.Fatalf("expected four quoted blocks, got %d", len(blocks))
	}
	for i, block := range blocks {
		prefix := block.prefix
		if item, ok := block.content.(*listItem); ok {
			prefix = item.prefix
			if item.marker == nil || item.marker.next != nil {
				t.Fatalf("quoted item %d retained another item's marker", i)
			}
		}
		if prefix.rails != blocks[0].prefix.rails {
			t.Fatalf("block %d copied the enclosing quotation", i)
		}
	}
}
