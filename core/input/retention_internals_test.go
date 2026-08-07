package input

import (
	"bytes"
	"testing"
	"unsafe"
)

func TestParserReleasesTheConsumedReadBufferBehindAPendingTail(t *testing.T) {
	var parser Parser
	parser.buf = append(bytes.Repeat([]byte{'x'}, 4096), esc)
	sourceStart := uintptr(unsafe.Pointer(unsafe.SliceData(parser.buf))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	sourceEnd := sourceStart + uintptr(len(parser.buf))
	parser.drain(false)
	if len(parser.buf) != 1 || parser.buf[0] != esc {
		t.Fatalf("pending bytes = %q, want one escape", parser.buf)
	}
	tailStart := uintptr(unsafe.Pointer(unsafe.SliceData(parser.buf))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	if tailStart >= sourceStart && tailStart < sourceEnd {
		t.Fatal("the pending escape still shares the consumed read allocation")
	}
}
