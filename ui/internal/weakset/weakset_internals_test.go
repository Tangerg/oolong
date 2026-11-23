package weakset

import (
	"runtime"
	"testing"
)

func TestStorageFollowsTheLiveSetRatherThanEveryMember(t *testing.T) {
	var s Set[[16]byte]
	for range 16 {
		for range 32 {
			s.Add(new([16]byte))
		}
		runtime.GC()
	}
	live := new([16]byte)
	s.Add(live)
	if len(s.members) > 64 || cap(s.members) > 128 {
		t.Fatalf("retained weak handles grew with registry age: len=%d cap=%d", len(s.members), cap(s.members))
	}
	runtime.KeepAlive(live)
}
