package text

import (
	"strings"
	"testing"
	"unsafe"
)

func TestDecoderReleasesDecodedStorageBehindAnIncompleteSequence(t *testing.T) {
	chunk := strings.Repeat("x", 4096) + "\x1b["
	sourceStart := uintptr(unsafe.Pointer(unsafe.StringData(chunk))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	var decoder Decoder
	decoder.Feed(chunk)
	if decoder.held != "\x1b[" {
		t.Fatalf("held = %q, want the incomplete sequence", decoder.held)
	}
	tailStart := uintptr(unsafe.Pointer(unsafe.StringData(decoder.held))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	if tailStart >= sourceStart && tailStart < sourceStart+uintptr(len(chunk)) {
		t.Fatal("the incomplete sequence still shares the decoded chunk allocation")
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
