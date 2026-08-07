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
