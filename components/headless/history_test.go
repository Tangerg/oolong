package headless_test

import (
	"strconv"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
)

func BenchmarkHistoryAtCapacity(b *testing.B) {
	const limit = 1000
	lines := make([]string, limit+1)
	for i := range lines {
		lines[i] = "command " + strconv.Itoa(i)
	}
	var history headless.History
	history.SetLimit(limit)
	for _, line := range lines[:limit] {
		history.Add(line)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		history.Add(lines[i%len(lines)])
	}
	if history.Len() != limit {
		b.Fatalf("history grew to %d entries", history.Len())
	}
}

func historyOf(lines ...string) *headless.History {
	var h headless.History
	for _, line := range lines {
		h.Add(line)
	}
	return &h
}

func TestHistoryWalksBackwardsAndForwards(t *testing.T) {
	h := historyOf("first", "second", "third")

	for _, want := range []string{"third", "second", "first"} {
		got, ok := h.Back("")
		if !ok || got != want {
			t.Fatalf("back gave %q (%v), want %q", got, ok, want)
		}
	}
	if _, ok := h.Back(""); ok {
		t.Error("stepping past the oldest entry reported something")
	}
	for _, want := range []string{"second", "third"} {
		got, ok := h.Forward()
		if !ok || got != want {
			t.Fatalf("forward gave %q (%v), want %q", got, ok, want)
		}
	}
}

// TestHistoryGivesTheDraftBack is the thing that is annoying to lose: the user is half
// way through a line, presses up to check something, and presses down expecting their
// line back. It is the only text in a prompt they cannot get again by scrolling.
func TestHistoryGivesTheDraftBack(t *testing.T) {
	h := historyOf("older")

	if got, _ := h.Back("half-typed"); got != "older" {
		t.Fatalf("back gave %q", got)
	}
	got, ok := h.Forward()
	if !ok || got != "half-typed" {
		t.Errorf("forward gave %q (%v), want the draft back", got, ok)
	}
	if h.Walking() {
		t.Error("still walking after coming back to the draft")
	}
	// And it is not handed out twice.
	if _, ok := h.Forward(); ok {
		t.Error("forward past the draft reported something")
	}
}

func TestHistoryCancelGivesTheDraftBack(t *testing.T) {
	h := historyOf("older")
	h.Back("half-typed")
	got, ok := h.Cancel()
	if !ok || got != "half-typed" {
		t.Errorf("cancel gave %q (%v)", got, ok)
	}
	if _, ok := h.Cancel(); ok {
		t.Error("cancelling when not walking reported a draft")
	}
}

func TestSubmittingEndsTheWalk(t *testing.T) {
	// Whatever the user was stepping through, they have now said something.
	h := historyOf("first", "second")
	h.Back("")
	h.Back("")
	h.Add("third")

	if h.Walking() {
		t.Error("still walking after a line was submitted")
	}
	if got, _ := h.Back(""); got != "third" {
		t.Errorf("the next step back gave %q, want the newest entry", got)
	}
}

func TestHistoryKeepsNothingNotWorthKeeping(t *testing.T) {
	h := historyOf("same", "same", "  ", "", "other", "same")
	if got := h.Len(); got != 3 {
		t.Errorf("kept %d entries, want 3", got)
	}
	// Duplicates that are not consecutive are kept: the order tells the truth about
	// what happened.
	for i, want := range []string{"same", "other", "same"} {
		got, _ := h.At(h.Len() - i)
		if got != want {
			t.Errorf("entry %d from the start is %q, want %q", i, got, want)
		}
	}
}

func TestHistoryDropsTheOldest(t *testing.T) {
	var h headless.History
	h.SetLimit(3)
	for _, line := range []string{"a", "b", "c", "d", "e"} {
		h.Add(line)
	}
	if got := h.Len(); got != 3 {
		t.Fatalf("kept %d entries, want 3", got)
	}
	if got, _ := h.At(3); got != "c" {
		t.Errorf("the oldest kept entry is %q, want %q", got, "c")
	}
}

func TestHistoryLimitHasOneValidatedConfigurationPath(t *testing.T) {
	var h headless.History
	if got := h.Limit(); got != headless.DefaultHistoryLimit {
		t.Fatalf("zero history limit = %d, want %d", got, headless.DefaultHistoryLimit)
	}
	h.SetLimit(3)
	if got := h.Limit(); got != 3 {
		t.Fatalf("configured history limit = %d, want 3", got)
	}
	h.SetLimit(0)
	if got := h.Limit(); got != headless.DefaultHistoryLimit {
		t.Fatalf("restored history limit = %d, want %d", got, headless.DefaultHistoryLimit)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("SetLimit accepted a negative limit")
		}
	}()
	h.SetLimit(-1)
}

func TestHistoryAtOutOfRange(t *testing.T) {
	h := historyOf("only")
	for _, n := range []int{0, -1, 2} {
		if got, ok := h.At(n); ok {
			t.Errorf("At(%d) = %q", n, got)
		}
	}
}

// TestRecallPutsTheRecentFirst. Scoring alone buries the line from a minute ago behind
// six from last week, and "the one I ran recently" is what somebody searching their
// own history is nearly always after.
func TestRecallPutsTheRecentFirst(t *testing.T) {
	h := historyOf("git status", "git commit", "make test", "git status --short")

	got := h.Recall("git")
	if len(got) != 3 {
		t.Fatalf("found %d entries, want 3: %+v", len(got), got)
	}
	if got[0].Entry != "git status --short" {
		t.Errorf("the first result is %q, want the most recent", got[0].Entry)
	}
	if got[0].Step != 1 {
		t.Errorf("its step is %d, want 1", got[0].Step)
	}
	// And the step is what jumps to it.
	entry, ok := h.At(got[1].Step)
	if !ok || entry != got[1].Entry {
		t.Errorf("step %d led to %q, want %q", got[1].Step, entry, got[1].Entry)
	}
	if len(got[0].At) == 0 {
		t.Error("nothing to underline in a match")
	}
}

func TestRecallWithNoQueryIsEverythingNewestFirst(t *testing.T) {
	h := historyOf("a", "b", "c")
	got := h.Recall("")
	if len(got) != 3 || got[0].Entry != "c" || got[2].Entry != "a" {
		t.Errorf("recall gave %+v", got)
	}
}

func TestWalkingIsNotDisturbedByWhatArrivesDuringIt(t *testing.T) {
	// The place is counted from the end, so an entry added mid-walk cannot move it.
	h := historyOf("first", "second")
	if got, _ := h.Back(""); got != "second" {
		t.Fatalf("back gave %q", got)
	}
	h.Add("third")
	if h.Walking() {
		t.Error("adding an entry left the walk in progress")
	}
}
