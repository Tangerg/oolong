package highlight

import (
	"strings"
	"testing"
	"unsafe"

	"github.com/Tangerg/oolong/core/text"
)

func TestHighlightedTextDoesNotRetainItsSource(t *testing.T) {
	source := strings.Repeat("// discarded\n", 1<<12) + "var kept = 1\n"
	lines := highlight("go", source, schemeOf(""))
	assertDetachedText(t, source, lines)
}

func TestPlainFallbackDoesNotRetainItsSource(t *testing.T) {
	source := strings.Repeat("discarded\n", 1<<12) + "kept"
	assertDetachedText(t, source, plain(source))
}

func assertDetachedText(t *testing.T, source string, lines []text.Line) {
	t.Helper()
	sourceStart := uintptr(unsafe.Pointer(unsafe.StringData(source))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
	sourceEnd := sourceStart + uintptr(len(source))
	for row, line := range lines {
		for column, span := range line {
			if span.Text == "" {
				continue
			}
			at := uintptr(unsafe.Pointer(unsafe.StringData(span.Text))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
			if at >= sourceStart && at < sourceEnd {
				t.Fatalf("line %d span %d still shares the source allocation", row, column)
			}
		}
	}
}
