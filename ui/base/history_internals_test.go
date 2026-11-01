package base

import (
	"runtime"
	"slices"
	"strconv"
	"testing"
	"weak"
)

func TestHistoryReleasesARegistryHighWaterMark(t *testing.T) {
	var history History[string]
	history.SetLimit(1024)
	for i := range 1024 {
		history.Add(strconv.Itoa(i))
	}
	history.SetLimit(1)

	if history.Len() != 1 || history.entries.At(0) != "1023" {
		t.Fatalf("reduced history = %v, want only the newest entry", slices.Collect(history.entries.Values()))
	}
	if history.entries.Capacity() > 2*history.Len()+16 {
		t.Fatalf("one entry retains capacity %d from the old limit", history.entries.Capacity())
	}
}

func TestHistoryReleasesEvictedReferences(t *testing.T) {
	var history History[[]byte]
	evicted := func() weak.Pointer[byte] {
		value := make([]byte, 256)
		history.Add(value)
		return weak.Make(&value[0])
	}
	one, two := evicted(), evicted()
	history.Add([]byte("three"))
	history.SetLimit(1)
	runtime.GC()
	if one.Value() != nil || two.Value() != nil {
		t.Fatal("evicted entries retain their payloads")
	}
	if string(history.entries.At(0)) != "three" {
		t.Fatalf("retained entry = %q, want three", history.entries.At(0))
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
