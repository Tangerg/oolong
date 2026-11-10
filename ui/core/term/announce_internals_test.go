package term

import (
	"errors"
	"io"
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
	if err := state.to(source, 0, func([]byte) (uint64, error) { return 1, nil }); err != nil {
		t.Error(err)
	}
	if state.set != titleCommand(source) {
		t.Fatalf("retained title = %q, want %q", state.set, source)
	}
	if unsafe.StringData(state.set[len(titleSet):]) == unsafe.StringData(source) { //nolint:gosec // Test compares allocation identity and never dereferences the address.
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
			if err := state.to(strconv.Itoa(i), 0, queue); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	if len(written) != 100 || strings.Count(strings.Join(written, ""), titlePush) != 1 {
		t.Fatal("title stack pushed more than once")
	}
	if written[len(written)-1] != state.set {
		t.Fatal("retained title differs from the last published title")
	}
}

func TestRefusedTitleCannotCreateOrReplaceRestorationState(t *testing.T) {
	var state title
	refuse := func([]byte) (uint64, error) { return 0, ErrQueueFull }
	if err := state.to("refused", 0, refuse); err == nil {
		t.Error("refused operation reported success")
	}
	if state.enter() != "" || state.leave() != "" {
		t.Fatal("a refused title created a terminal stack obligation")
	}
	if err := state.to("accepted", 0, func([]byte) (uint64, error) { return 1, nil }); err != nil {
		t.Error(err)
	}
	if err := state.to("refused replacement", 0, refuse); err == nil {
		t.Error("refused operation reported success")
	}
	if got := state.enter(); got != titlePush+titleCommand("accepted") {
		t.Fatalf("restored an unaccepted title: %q", got)
	}
}

func TestTitleAdmissionReservesEnoughRoomForReacquisition(t *testing.T) {
	w := NewWriter(WriterConfig{Destination: io.Discard})
	defer func() { _ = w.Close() }()
	modes, err := Config{AltScreen: true}.Modes(nil)
	if err != nil {
		t.Fatal(err)
	}
	terminal := &Terminal{writer: w, modes: modes}
	if err := terminal.SetTitle("accepted"); err != nil {
		t.Fatal(err)
	}
	if err := terminal.SetTitle(strings.Repeat("x", 64<<10)); !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("oversize title = %v; must refuse before accepting a restoration obligation", err)
	}
	if got := terminal.title.enter(); got != titlePush+titleCommand("accepted") {
		t.Fatalf("refusal changed restoration state (%d bytes)", len(got))
	}
}

func TestNotificationTextCannotSelectAnOSC9Subcommand(t *testing.T) {
	for _, tc := range []struct{ text, body string }{
		{"ready", "ready"},
		{"", ""},
		{"2026", "• 2026"},
		{"4;0", "• 4;0"},
		{"5 items", "• 5 items"},
		{"\n7;example", "• 7;example"},
		{" 6;example", "•  6;example"},
		{"x\x1b\x07y", "xy"},
	} {
		var output strings.Builder
		w := NewWriter(WriterConfig{Destination: &output})
		terminal := &Terminal{writer: w}
		err := terminal.Notify(tc.text)
		if err = errors.Join(err, w.Close(), w.Err()); err != nil {
			t.Fatal(err)
		}
		if want := notifySend + tc.body + "\a"; output.String() != want {
			t.Errorf("Notify(%q) encoded %q, want %q", tc.text, output.String(), want)
		}
	}
}
