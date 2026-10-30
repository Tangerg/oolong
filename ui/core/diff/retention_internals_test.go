package diff

import (
	"strings"
	"testing"
	"unsafe"
)

func TestHunksDoNotRetainTheirSourceDocuments(t *testing.T) {
	prefix := strings.Repeat("unchanged context\n", 1<<12)
	beforeSource := prefix + "before"
	afterSource := prefix + "after"
	hunks := Between(strings.Split(beforeSource, "\n"), strings.Split(afterSource, "\n")).Hunks(0)
	if len(hunks) != 1 || len(hunks[0].Lines) != 2 {
		t.Fatalf("hunks = %+v, want one replacement", hunks)
	}

	for i, line := range hunks[0].Lines {
		at := uintptr(unsafe.Pointer(unsafe.StringData(line.Text))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
		for name, source := range map[string]string{"before": beforeSource, "after": afterSource} {
			start := uintptr(unsafe.Pointer(unsafe.StringData(source))) //nolint:gosec // Test compares allocation identity and never dereferences the address.
			if at >= start && at < start+uintptr(len(source)) {
				t.Errorf("hunk line %d still shares the %s document allocation", i, name)
			}
		}
	}
}
