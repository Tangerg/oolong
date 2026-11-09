package input_test

import (
	"image"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
)

func TestX10MouseBodyDoesNotExpireAsTyping(t *testing.T) {
	report := x10(0, 3, 4)
	const introducerBytes = len("\x1b[M")
	for split := introducerBytes; split < len(report); split++ {
		t.Run(strconv.Itoa(split), func(t *testing.T) {
			started := time.Unix(1, 0)
			at := started
			var stream input.Stream
			if events := stream.Feed([]byte(report[:split]), at); len(events) != 0 {
				t.Fatalf("incomplete mouse report produced %+v", events)
			}
			if due, waiting := stream.DueAt(); waiting {
				t.Fatalf("recognized mouse framing armed an Escape deadline at %v", due)
			}
			at = at.Add(2 * input.DefaultEscapeTimeout)
			if events := stream.Expire(at); len(events) != 0 {
				t.Fatalf("mouse report expired into %+v", events)
			}
			want := []input.Event{input.Mouse{Pos: image.Pt(2, 3), Action: input.MouseDown, Button: input.ButtonLeft, At: started}}
			if got := stream.Feed([]byte(report[split:]), at); !slices.Equal(got, want) {
				t.Fatalf("delayed mouse body produced %+v, want %+v", got, want)
			}
		})
	}
}

func TestX10MouseBodyAtEOFDoesNotBecomeTyping(t *testing.T) {
	report := x10(0, 3, 4)
	const introducerBytes = len("\x1b[M")
	for end := introducerBytes; end < len(report); end++ {
		t.Run(strconv.Itoa(end), func(t *testing.T) {
			var parser input.Parser
			if events := parser.Feed([]byte(report[:end])); len(events) != 0 {
				t.Fatalf("incomplete mouse report produced %+v", events)
			}
			if !parser.Pending() {
				t.Fatal("mouse payload was forgotten before EOF")
			}
			if events := parser.Flush(); len(events) != 0 {
				t.Fatalf("truncated mouse report became %+v at EOF", events)
			}
			if parser.Pending() {
				t.Fatal("EOF retained an incomplete mouse report")
			}
			want := []input.Event{input.Key{Rune: 'z'}}
			if got := parser.Feed([]byte("z")); !slices.Equal(got, want) {
				t.Fatalf("input after EOF = %+v, want %+v", got, want)
			}
		})
	}
}
