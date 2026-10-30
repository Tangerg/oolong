package base_test

import (
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/ui/base"
)

func TestHistoryFailedSnapshotsPreserveTheWalk(t *testing.T) {
	for _, test := range []struct {
		name, failed string
		steps        int
		step         func(*base.History[string]) (string, bool)
		want         string
	}{
		{"first draft", "draft", 0, func(h *base.History[string]) (string, bool) { return h.Back("draft") }, "newest"},
		{"first entry", "newest", 0, func(h *base.History[string]) (string, bool) { return h.Back("draft") }, "newest"},
		{"older entry", "older", 1, func(h *base.History[string]) (string, bool) { return h.Back("ignored") }, "older"},
		{"newer entry", "newest", 2, (*base.History[string]).Forward, "newest"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fail := false
			h := base.NewHistory(base.HistoryConfig[string]{Clone: func(value string) string {
				if fail && value == test.failed {
					panic("snapshot failed")
				}
				return value
			}})
			h.Add("older")
			h.Add("newest")
			for range test.steps {
				h.Back("draft")
			}
			fail = true
			func() {
				defer func() {
					if got := recover(); got != "snapshot failed" {
						t.Fatalf("panic = %v, want snapshot failed", got)
					}
				}()
				test.step(h)
			}()
			if h.Walking() != (test.steps > 0) || h.Len() != 2 {
				t.Fatalf("failed snapshot changed history: walking=%v, entries=%d", h.Walking(), h.Len())
			}
			fail = false
			if got, ok := test.step(h); !ok || got != test.want {
				t.Fatalf("next step = %q (%v), want %q", got, ok, test.want)
			}
			if got, ok := h.Cancel(); !ok || got != "draft" {
				t.Fatalf("saved draft = %q (%v), want draft", got, ok)
			}
		})
	}
}

func TestHistoryGoexitPreservesTheSavedDraft(t *testing.T) {
	for _, callback := range []string{"clone", "equal"} {
		t.Run(callback, func(t *testing.T) {
			fail := false
			h := base.NewHistory(base.HistoryConfig[string]{
				Clone: func(value string) string {
					if fail && callback == "clone" {
						runtime.Goexit()
					}
					return value
				},
				Equal: func(a, b string) bool {
					if fail && callback == "equal" {
						runtime.Goexit()
					}
					return a == b
				},
			})
			h.Add("entry")
			h.Back("draft")
			fail = true
			done := make(chan struct{})
			go func() {
				defer close(done)
				h.Add("another")
			}()
			<-done
			fail = false
			if h.Len() != 1 || !h.Walking() {
				t.Fatalf("abnormal Add changed history: entries=%d, walking=%v", h.Len(), h.Walking())
			}
			if got, ok := h.Forward(); !ok || got != "draft" {
				t.Fatalf("draft after Goexit = %q (%v), want draft", got, ok)
			}
		})
	}
}

func TestHistoryFailedAddPreservesEntriesAndDraft(t *testing.T) {
	for _, callback := range []string{"clone", "equal"} {
		t.Run(callback, func(t *testing.T) {
			fail := false
			h := base.NewHistory(base.HistoryConfig[string]{
				Clone: func(value string) string {
					if fail && callback == "clone" {
						panic("record failed")
					}
					return value
				},
				Equal: func(a, b string) bool {
					if fail && callback == "equal" {
						panic("record failed")
					}
					return a == b
				},
			})
			h.Add("older")
			h.Add("newest")
			h.Back("draft")
			fail = true
			func() {
				defer func() {
					if got := recover(); got != "record failed" {
						t.Fatalf("panic = %v, want record failed", got)
					}
				}()
				h.Add("another")
			}()
			fail = false
			if h.Len() != 2 || !h.Walking() {
				t.Fatalf("failed Add changed history: entries=%d, walking=%v", h.Len(), h.Walking())
			}
			if got, ok := h.Forward(); !ok || got != "draft" {
				t.Fatalf("draft after failed Add = %q (%v), want draft", got, ok)
			}
			if got, ok := h.At(1); !ok || got != "newest" {
				t.Fatalf("newest after failed Add = %q (%v), want newest", got, ok)
			}
		})
	}
}
