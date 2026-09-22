package term

import (
	"strconv"
	"strings"
	"sync"
	"testing"
	"unsafe"
)

func TestTitleOwnsTheTextItRetainsAcrossHandover(t *testing.T) {
	backing := strings.Repeat("x", 4096) + "kept"
	source := backing[len(backing)-len("kept"):]
	var state title
	state.to(source, func([]byte) uint64 { return 1 })
	if state.text != source {
		t.Fatalf("retained title = %q, want %q", state.text, source)
	}
	if unsafe.StringData(state.text) == unsafe.StringData(source) { //nolint:gosec // Test compares allocation identity and never dereferences the address.
		t.Fatal("a short title retained its caller's larger backing string")
	}
}

func TestConcurrentTitlesPublishInTheirRetainedOrder(t *testing.T) {
	var state title
	var written []string
	queue := func(data []byte) uint64 { written = append(written, string(data)); return uint64(len(written)) }
	var workers sync.WaitGroup
	for i := range 100 {
		workers.Go(func() { state.to(strconv.Itoa(i), queue) })
	}
	workers.Wait()
	if len(written) != 100 || strings.Count(strings.Join(written, ""), titlePush) != 1 {
		t.Fatal("title stack pushed more than once")
	}
	if written[len(written)-1] != command(titleSet, state.text) {
		t.Fatal("retained title differs from the last published title")
	}
}
