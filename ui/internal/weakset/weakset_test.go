package weakset_test

import (
	"runtime"
	"testing"

	"github.com/Tangerg/oolong/ui/internal/weakset"
)

type member struct{ name string }

func TestLiveMembersKeepRegistrationOrder(t *testing.T) {
	var s weakset.Set[member]
	first, second := &member{"first"}, &member{"second"}
	s.Add(first)
	s.Add(second)
	live := s.Live()
	if len(live) != 2 || live[0] != first || live[1] != second {
		t.Fatalf("live = %v, want first then second", live)
	}
	s.Prune(func(m *member) bool { return m != first })
	if live := s.Live(); len(live) != 1 || live[0] != second {
		t.Fatalf("after pruning first, live = %v", live)
	}
	runtime.KeepAlive(first)
	runtime.KeepAlive(second)
}

func TestCollectedMembersLeaveTheRegistry(t *testing.T) {
	var s weakset.Set[member]
	s.Add(&member{"transient"})
	runtime.GC()
	if live := s.Live(); len(live) != 0 {
		t.Fatalf("a collected member is still live: %v", live)
	}
}
