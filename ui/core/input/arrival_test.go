package input_test

import (
	"image"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
)

func TestSplitInputKeepsItsFirstObservation(t *testing.T) {
	started := time.Unix(1, 0)
	completed := started.Add(time.Millisecond)
	for _, test := range []struct {
		name, prefix, tail string
		want               input.Event
	}{
		{"UTF-8", "\xe4", "\xb8\xad", input.Key{Rune: '中', At: started}},
		{"CSI", "\x1b[1;", "5A", input.Key{Code: input.Up, Mods: input.Ctrl, At: started}},
		{"SS3", "\x1bO", "P", input.Key{Code: input.F1, At: started}},
		{"Alt", "\x1b\xe4", "\xb8\xad", input.Key{Rune: '中', Mods: input.Alt, At: started}},
		{"SGR mouse", "\x1b[<0;3;4", "M", input.Mouse{Pos: image.Pt(2, 3), Button: input.ButtonLeft, At: started}},
		{"X10 mouse", "\x1b[M", "\x20\x23\x24", input.Mouse{Pos: image.Pt(2, 3), Button: input.ButtonLeft, At: started}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stream input.Stream
			if events := stream.Feed([]byte(test.prefix), started); len(events) != 0 {
				t.Fatalf("partial unit produced %+v", events)
			}
			got := stream.Feed([]byte(test.tail+"z"), completed)
			want := []input.Event{test.want, input.Key{Rune: 'z', At: completed}}
			if !slices.Equal(got, want) {
				t.Fatalf("received %+v, want %+v", got, want)
			}
		})
	}
}

func TestOneReadMayCompleteOneUnitAndStartAnother(t *testing.T) {
	var stream input.Stream
	first := time.Unix(1, 0)
	second := first.Add(time.Millisecond)
	third := second.Add(time.Millisecond)
	stream.Feed([]byte("\xe4"), first)
	got := stream.Feed([]byte("\xb8\xad\x1b["), second)
	got = append(got, stream.Feed([]byte("A"), third)...)
	want := []input.Event{input.Key{Rune: '中', At: first}, input.Key{Code: input.Up, At: second}}
	if !slices.Equal(got, want) {
		t.Fatalf("one read conflated two origins: %+v", got)
	}
}

func TestResetAndCancellationReleaseThePreviousOrigin(t *testing.T) {
	for _, reset := range []bool{false, true} {
		var stream input.Stream
		first := time.Unix(1, 0)
		second := first.Add(time.Second)
		stream.Feed([]byte("\x1bP>|body"), first)
		var got []input.Event
		if reset {
			stream.Reset()
			got = stream.Feed([]byte("z"), second)
		} else {
			got = stream.Feed([]byte("\x18z"), second)
		}
		if !slices.Equal(got, []input.Event{input.Key{Rune: 'z', At: second}}) {
			t.Fatalf("reset=%v retained the old origin: %+v", reset, got)
		}
	}
}
