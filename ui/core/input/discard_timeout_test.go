package input_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
)

func TestRejectedCSIStaysFramedAcrossEscapeTimeout(t *testing.T) {
	for _, gap := range []time.Duration{
		input.DefaultEscapeTimeout - time.Nanosecond,
		input.DefaultEscapeTimeout,
		input.DefaultEscapeTimeout + time.Nanosecond,
	} {
		t.Run(gap.String(), func(t *testing.T) {
			var stream input.Stream
			at := time.Unix(1, 0)
			if events := stream.Feed([]byte("\x1b["+strings.Repeat("0", (64<<10)+1)), at); len(events) != 0 {
				t.Fatalf("oversized CSI = %#v", events)
			}
			if _, waiting := stream.DueAt(); waiting {
				t.Fatal("rejected CSI acquired an Escape deadline")
			}
			if events := stream.Expire(at.Add(gap)); len(events) != 0 {
				t.Fatalf("discard expired into %#v", events)
			}
			stream.Feed([]byte(strings.Repeat(";1", 1<<16)), at.Add(gap))
			got := stream.Feed([]byte("qz"), at.Add(time.Second))
			want := []input.Event{input.Key{Rune: 'z', At: at.Add(time.Second)}}
			if !slices.Equal(got, want) {
				t.Fatalf("final byte escaped framing: %#v; want %#v", got, want)
			}
		})
	}
}

func TestRejectedCSIEndsOnlyAtItsBoundaryOrTransportReset(t *testing.T) {
	for _, end := range []string{"final", "canceled", "EOF", "reset"} {
		t.Run(end, func(t *testing.T) {
			var stream input.Stream
			at := time.Unix(1, 0)
			stream.Feed([]byte("\x1b["+strings.Repeat("0", (64<<10)+1)), at)
			var expected []input.Event
			switch end {
			case "final":
				stream.Feed([]byte("q"), at)
			case "canceled":
				expected = stream.Feed([]byte{0x18}, at)
				if !slices.Equal(expected, []input.Event{input.Key{Rune: 'x', Mods: input.Ctrl, At: at}}) {
					t.Fatalf("cancellation key = %#v", expected)
				}
			case "EOF":
				stream.Flush(at)
			case "reset":
				stream.Reset()
			default:
				t.Fatalf("unhandled end %q", end)
			}
			got := stream.Feed([]byte("0"), at.Add(time.Second))
			if !slices.Equal(got, []input.Event{input.Key{Rune: '0', At: at.Add(time.Second)}}) {
				t.Fatalf("completed discard retained authority: %#v", got)
			}
		})
	}
}
