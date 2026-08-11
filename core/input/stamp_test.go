package input_test

import (
	"image"
	"testing"
	"time"

	"github.com/Tangerg/oolong/core/input"
)

func TestStampGivesTimedEventsOneTransportArrival(t *testing.T) {
	at := time.Unix(42, 17)
	kept := at.Add(-time.Second)
	events := []input.Event{
		input.Key{Code: input.Enter},
		input.Mouse{Pos: image.Pt(3, 4)},
		input.Paste{Text: "untimed"},
		input.Key{Code: input.Esc, At: kept},
	}

	got := input.Stamp(events, at)
	if got[0].(input.Key).At != at || got[1].(input.Mouse).At != at {
		t.Fatalf("stamped events = %#v", got)
	}
	if events[0].(input.Key).At != at {
		t.Fatal("Stamp returned a detached batch instead of replacing parser-owned events")
	}
	if got[2] != (input.Paste{Text: "untimed"}) {
		t.Fatalf("untimed event changed to %#v", got[2])
	}
	if got[3].(input.Key).At != kept {
		t.Fatalf("existing arrival changed from %v to %v", kept, got[3].(input.Key).At)
	}
}
