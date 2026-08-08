package headless

import (
	"strconv"
	"testing"
)

func TestHistoryReleasesARegistryHighWaterMark(t *testing.T) {
	var history History
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
