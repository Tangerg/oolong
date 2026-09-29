package headless_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Tangerg/oolong/components/headless"
)

type historyAttachment struct {
	ID, Label string
	Data      []byte
}

type historyMessage struct {
	Text        string
	Attachments []historyAttachment
}

func (m historyMessage) clone() historyMessage {
	m.Attachments = slices.Clone(m.Attachments)
	for i := range m.Attachments {
		m.Attachments[i].Data = slices.Clone(m.Attachments[i].Data)
	}
	return m
}

func (m historyMessage) equal(other historyMessage) bool {
	return m.Text == other.Text && slices.EqualFunc(m.Attachments, other.Attachments, func(a, b historyAttachment) bool {
		return a.ID == b.ID && a.Label == b.Label && slices.Equal(a.Data, b.Data)
	})
}

func messageWithAttachment(id string) historyMessage {
	return historyMessage{Text: "explain @file", Attachments: []historyAttachment{
		{ID: id, Label: "@file", Data: []byte("payload")},
	}}
}

func messageHistory() *headless.History[historyMessage] {
	return headless.NewHistory(headless.HistoryConfig[historyMessage]{
		Clone: historyMessage.clone,
		Equal: historyMessage.equal,
	})
}

func TestHistoryKeepsStructuredIdentityAndNonconsecutiveOrder(t *testing.T) {
	h := messageHistory()
	for _, id := range []string{"one", "one", "two", "one"} {
		h.Add(messageWithAttachment(id))
	}
	if got := h.Len(); got != 3 {
		t.Fatalf("retained %d messages, want 3", got)
	}
	for i, id := range []string{"one", "two", "one"} {
		got, ok := h.At(i + 1)
		if !ok || !reflect.DeepEqual(got, messageWithAttachment(id)) {
			t.Fatalf("At(%d) = %+v (%v), want attachment %q", i+1, got, ok, id)
		}
	}

	matches := h.Recall("file", func(m historyMessage) string { return m.Text })
	if len(matches) != 3 {
		t.Fatalf("Recall returned %d entries, want 3", len(matches))
	}
	for i, id := range []string{"one", "two", "one"} {
		got := matches[i]
		if got.Step != i+1 || got.Entry.Attachments[0].ID != id || !slices.Equal(got.At, []int{9, 10, 11, 12}) {
			t.Fatalf("match %d = %+v, want attachment %q at step %d", i, got, id, i+1)
		}
	}
}

func TestHistorySnapshotsStructuredInputs(t *testing.T) {
	h := messageHistory()
	value := messageWithAttachment("original")
	h.Add(value)
	value.Text = "changed"
	value.Attachments[0].ID = "changed"
	value.Attachments[0].Data[0] = 'X'
	value.Attachments = append(value.Attachments, historyAttachment{ID: "extra"})

	got, ok := h.At(1)
	if !ok || !reflect.DeepEqual(got, messageWithAttachment("original")) {
		t.Fatalf("recorded value changed with its input: %+v (%v)", got, ok)
	}
}

func TestHistoryReturnsIndependentStructuredEntries(t *testing.T) {
	for _, test := range []struct {
		name string
		read func(*headless.History[historyMessage]) historyMessage
	}{
		{"At", func(h *headless.History[historyMessage]) historyMessage {
			got, _ := h.At(1)
			return got
		}},
		{"Back", func(h *headless.History[historyMessage]) historyMessage {
			got, _ := h.Back(historyMessage{})
			return got
		}},
		{"Forward", func(h *headless.History[historyMessage]) historyMessage {
			h.Back(historyMessage{})
			h.Back(historyMessage{})
			got, _ := h.Forward()
			return got
		}},
		{"RecallAll", func(h *headless.History[historyMessage]) historyMessage {
			return h.Recall("", nil)[0].Entry
		}},
		{"RecallMatch", func(h *headless.History[historyMessage]) historyMessage {
			return h.Recall("file", func(m historyMessage) string { return m.Text })[0].Entry
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := messageHistory()
			h.Add(messageWithAttachment("older"))
			h.Add(messageWithAttachment("newest"))
			got := test.read(h)
			if !reflect.DeepEqual(got, messageWithAttachment("newest")) {
				t.Fatalf("returned value = %+v, want newest", got)
			}
			got.Attachments[0].ID = "changed"
			got.Attachments[0].Data[0] = 'X'
			again, ok := h.At(1)
			if !ok || !reflect.DeepEqual(again, messageWithAttachment("newest")) {
				t.Fatalf("retained value changed with returned value: %+v (%v)", again, ok)
			}
		})
	}
}

func TestHistoryRestoresAnIndependentStructuredDraft(t *testing.T) {
	for _, test := range []struct {
		name   string
		finish func(*headless.History[historyMessage]) (historyMessage, bool)
	}{
		{"Forward", (*headless.History[historyMessage]).Forward},
		{"Cancel", (*headless.History[historyMessage]).Cancel},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := messageHistory()
			h.Add(messageWithAttachment("entry"))
			draft := messageWithAttachment("draft")
			h.Back(draft)
			draft.Attachments[0].ID = "changed"
			draft.Attachments[0].Data[0] = 'X'
			if _, ok := h.Back(messageWithAttachment("replacement")); ok {
				t.Fatal("moved past the oldest entry")
			}
			got, ok := test.finish(h)
			if !ok || !reflect.DeepEqual(got, messageWithAttachment("draft")) || h.Walking() {
				t.Fatalf("restored draft = %+v (%v), walking=%v", got, ok, h.Walking())
			}
			got.Attachments[0].Data[0] = 'Y'
			if _, ok := test.finish(h); ok {
				t.Fatal("returned the draft twice")
			}
			entry, _ := h.Back(messageWithAttachment("next draft"))
			if !reflect.DeepEqual(entry, messageWithAttachment("entry")) {
				t.Fatalf("draft mutation changed history entry: %+v", entry)
			}
			next, _ := test.finish(h)
			if !reflect.DeepEqual(next, messageWithAttachment("next draft")) {
				t.Fatalf("next walk retained the previous draft: %+v", next)
			}
		})
	}
}

func TestShrinkingStructuredHistoryKeepsTheDraftAndRetainedOrder(t *testing.T) {
	h := messageHistory()
	for _, id := range []string{"one", "two", "three", "four"} {
		h.Add(messageWithAttachment(id))
	}
	h.Back(messageWithAttachment("draft"))
	h.Back(historyMessage{})
	h.Back(historyMessage{})
	h.SetLimit(2)
	if _, ok := h.Back(historyMessage{}); ok {
		t.Fatal("moved into an evicted entry")
	}
	for _, id := range []string{"three", "four", "draft"} {
		got, ok := h.Forward()
		if !ok || !reflect.DeepEqual(got, messageWithAttachment(id)) {
			t.Fatalf("forward after shrinking = %+v (%v), want %q", got, ok, id)
		}
	}
}

func TestHistoryLeavesEmptyValuePolicyToTheCaller(t *testing.T) {
	h := historyOf("", "", " ", "  ", "")
	if h.Len() != 4 {
		t.Fatalf("kept %d entries, want 4", h.Len())
	}
	for i, want := range []string{"", "  ", " ", ""} {
		if got, ok := h.At(i + 1); !ok || got != want {
			t.Fatalf("At(%d) = %q (%v), want %q", i+1, got, ok, want)
		}
	}
}

func TestHistoryZeroConfigurationKeepsEveryValue(t *testing.T) {
	var zero headless.History[int]
	for _, h := range []*headless.History[int]{&zero, headless.NewHistory(headless.HistoryConfig[int]{})} {
		h.Add(0)
		h.Add(0)
		if h.Len() != 2 || h.Limit() != 1000 {
			t.Fatalf("unconfigured history kept %d entries with limit %d", h.Len(), h.Limit())
		}
		if got, ok := h.Back(42); !ok || got != 0 {
			t.Fatalf("zero entry = %d (%v)", got, ok)
		}
		if got, ok := h.Forward(); !ok || got != 42 {
			t.Fatalf("draft = %d (%v), want 42", got, ok)
		}
	}
}

func TestDuplicateSubmissionEndsTheHistoryWalk(t *testing.T) {
	h := historyOf("same")
	h.Back("draft")
	h.Add("same")
	if h.Walking() || h.Len() != 1 {
		t.Fatalf("duplicate submission: walking=%v, entries=%d", h.Walking(), h.Len())
	}
	if _, ok := h.Cancel(); ok {
		t.Fatal("submission retained an abandoned draft")
	}
}

func TestEmptyHistoryDoesNotCaptureADraft(t *testing.T) {
	h := messageHistory()
	if _, ok := h.Back(messageWithAttachment("draft")); ok || h.Walking() {
		t.Fatal("empty history began a walk")
	}
	if _, ok := h.Cancel(); ok {
		t.Fatal("empty history retained a draft")
	}
	if _, ok := h.Forward(); ok {
		t.Fatal("empty history moved forward")
	}
}

func TestHistoryRecallOnlyObservesTheWalk(t *testing.T) {
	h := historyOf("older", "newest")
	h.Back("draft")
	if got := h.Recall("needle", nil); len(got) != 0 {
		t.Fatalf("query without text matched %+v", got)
	}
	matches := h.Recall("", func(string) string { panic("empty query must not project") })
	if len(matches) != 2 || matches[0].Step != 1 || matches[1].Step != 2 {
		t.Fatalf("all entries = %+v", matches)
	}
	if got, ok := h.Forward(); !ok || got != "draft" {
		t.Fatalf("Recall moved the walk: Forward = %q (%v)", got, ok)
	}
}

func TestNewHistoryRejectsNegativeCapacity(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("NewHistory accepted a negative limit")
		}
	}()
	headless.NewHistory(headless.HistoryConfig[int]{Limit: -1})
}
