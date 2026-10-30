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
	if err := state.to(source, func([]byte) (uint64, error) { return 1, nil }); err != nil {
		t.Error(err)
	}
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
	queue := func(data []byte) (uint64, error) { //nolint:unparam // The successful callback must implement the admission signature.
		written = append(written, string(data))
		return uint64(len(written)), nil
	}
	var workers sync.WaitGroup
	for i := range 100 {
		workers.Go(func() {
			if err := state.to(strconv.Itoa(i), queue); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	if len(written) != 100 || strings.Count(strings.Join(written, ""), titlePush) != 1 {
		t.Fatal("title stack pushed more than once")
	}
	if written[len(written)-1] != command(titleSet, state.text) {
		t.Fatal("retained title differs from the last published title")
	}
}

func TestRefusedTitleCannotCreateOrReplaceRestorationState(t *testing.T) {
	var state title
	refuse := func([]byte) (uint64, error) { return 0, ErrQueueFull }
	if err := state.to("refused", refuse); err == nil {
		t.Error("refused operation reported success")
	}
	if state.enter() != "" || state.leave() != "" {
		t.Fatal("a refused title created a terminal stack obligation")
	}
	if err := state.to("accepted", func([]byte) (uint64, error) { return 1, nil }); err != nil {
		t.Error(err)
	}
	if err := state.to("refused replacement", refuse); err == nil {
		t.Error("refused operation reported success")
	}
	if got := state.enter(); got != titlePush+command(titleSet, "accepted") {
		t.Fatalf("restored an unaccepted title: %q", got)
	}
}
