package headless

import (
	"slices"
	"strconv"
	"testing"
)

func TestHistoryReleasesARegistryHighWaterMark(t *testing.T) {
	var history History[string]
	history.SetLimit(1024)
	for i := range 1024 {
		history.Add(strconv.Itoa(i))
	}
	history.SetLimit(1)

	if len(history.entries) != 1 || history.entries[0] != "1023" {
		t.Fatalf("reduced history = %v, want only the newest entry", history.entries)
	}
	if cap(history.entries) > 2*len(history.entries)+16 {
		t.Fatalf("one entry retains capacity %d from the old limit", cap(history.entries))
	}
}

func TestHistoryReleasesEvictedReferences(t *testing.T) {
	var history History[[]byte]
	history.Add([]byte("one"))
	history.Add([]byte("two"))
	history.Add([]byte("three"))
	retained := history.entries
	history.SetLimit(1)
	if retained[0] != nil || retained[1] != nil {
		t.Fatalf("evicted entries retain references: %v", retained[:2])
	}
	if string(history.entries[0]) != "three" {
		t.Fatalf("retained entry = %q, want three", history.entries[0])
	}
}

func TestHistoryReleasesItsDraftWhenTheWalkEnds(t *testing.T) {
	for _, test := range []struct {
		name string
		end  func(*History[[]byte])
	}{
		{"Forward", func(h *History[[]byte]) { h.Forward() }},
		{"Cancel", func(h *History[[]byte]) { h.Cancel() }},
		{"Add", func(h *History[[]byte]) { h.Add([]byte("submitted")) }},
		{"AddDuplicate", func(h *History[[]byte]) { h.Add([]byte("entry")) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			history := NewHistory(HistoryConfig[[]byte]{Clone: slices.Clone[[]byte], Equal: slices.Equal[[]byte]})
			history.Add([]byte("entry"))
			history.Back([]byte("draft"))
			test.end(history)
			if history.draft != nil {
				t.Fatalf("ended walk retained draft %q", history.draft)
			}
		})
	}
}
