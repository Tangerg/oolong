package text

import (
	"math"
	"strings"
	"testing"
	"unsafe"
)

func TestTextExtentArithmeticSaturates(t *testing.T) {
	if got := advance("\t", math.MaxInt); got != math.MaxInt {
		t.Fatalf("tab advanced the largest column to %d", got)
	}
	w := wrapper{rowWidth: math.MaxInt}
	w.place(unit{width: 1})
	if w.rowWidth != math.MaxInt {
		t.Fatalf("placing past the largest row width wrapped to %d", w.rowWidth)
	}
}

func TestDecoderReleasesDecodedStorageBehindAnIncompleteSequence(t *testing.T) {
	chunk := strings.Repeat("x", 4096) + "\x1b["
	sourceStart := uintptr(unsafe.Pointer(unsafe.StringData(chunk))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	var decoder Decoder
	decoder.Feed(chunk)
	held := decoder.scan.Pending()
	if held != "\x1b[" {
		t.Fatalf("held = %q, want the incomplete sequence", held)
	}
	tailStart := uintptr(unsafe.Pointer(unsafe.StringData(held))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	if tailStart >= sourceStart && tailStart < sourceStart+uintptr(len(chunk)) {
		t.Fatal("the incomplete sequence still shares the decoded chunk allocation")
	}
}

func TestDecoderDetachesSmallResultsFromLargeInputChunks(t *testing.T) {
	chunk := "kept\x1b]8;;https://example.test\x07linked\x1b]8;;\x07\x1b]0;" +
		strings.Repeat("discarded", 1<<12) + "\x07"
	sourceStart := uintptr(unsafe.Pointer(unsafe.StringData(chunk))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	sourceEnd := sourceStart + uintptr(len(chunk))

	var decoder Decoder
	decoder.Feed(chunk)
	line := decoder.Open()
	if got := line.String(); got != "keptlinked" {
		t.Fatalf("open line = %q, want keptlinked", got)
	}
	for i, span := range line {
		for name, value := range map[string]string{"text": span.Text, "link": span.Link} {
			if value == "" {
				continue
			}
			at := uintptr(unsafe.Pointer(unsafe.StringData(value))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
			if at >= sourceStart && at < sourceEnd {
				t.Errorf("span %d %s still shares the input chunk allocation", i, name)
			}
		}
	}
}

func TestDecoderChunkingDoesNotAllocatePerChunk(t *testing.T) {
	const chunks = 4096
	lineAllocs := testing.AllocsPerRun(3, func() {
		var decoder Decoder
		for range chunks {
			decoder.Feed("x")
		}
		_ = decoder.Open()
	})
	if lineAllocs >= 64 {
		t.Fatalf("a %d-chunk line made %.0f allocations; want amortised growth", chunks, lineAllocs)
	}

	sequenceAllocs := testing.AllocsPerRun(3, func() {
		var decoder Decoder
		decoder.Feed("\x1b]")
		for range chunks {
			decoder.Feed("x")
		}
	})
	if sequenceAllocs >= 64 {
		t.Fatalf("a %d-chunk sequence made %.0f allocations; want amortised growth", chunks, sequenceAllocs)
	}
}

func TestLineCloneDetachesRetainedStrings(t *testing.T) {
	source := strings.Repeat("discarded ", 1<<12) + "kept destination"
	textAt := strings.Index(source, "kept")
	linkAt := strings.Index(source, "destination")
	line := Line{{Text: source[textAt : textAt+len("kept")], Link: source[linkAt:]}}
	cloned := line.Clone()

	sourceStart := uintptr(unsafe.Pointer(unsafe.StringData(source))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	sourceEnd := sourceStart + uintptr(len(source))
	for name, value := range map[string]string{
		"text": cloned[0].Text,
		"link": cloned[0].Link,
	} {
		at := uintptr(unsafe.Pointer(unsafe.StringData(value))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
		if at >= sourceStart && at < sourceEnd {
			t.Errorf("cloned %s still shares the source allocation", name)
		}
	}

	line[0].Text = "changed"
	if cloned[0].Text != "kept" {
		t.Fatalf("changing the source span changed the clone to %q", cloned[0].Text)
	}
}

func TestCloneLinesOwnsBothCollectionLevels(t *testing.T) {
	lines := []Line{{{Text: "first"}}, {{Text: "second"}}}
	cloned := CloneLines(lines)
	lines[0][0].Text = "changed"
	lines[1] = nil
	if len(cloned) != 2 || cloned[0].String() != "first" || cloned[1].String() != "second" {
		t.Fatalf("clone changed with its source: %v", cloned)
	}
	if got := CloneLines(nil); got != nil {
		t.Fatalf("nil clone = %v, want nil", got)
	}
}
