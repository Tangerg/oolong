package input_test

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/ui/core/input"
)

func TestDCSBodyCannotBecomeKeysAtBell(t *testing.T) {
	for _, size := range []int{8, 1 << 20} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			var parser input.Parser
			parser.Feed([]byte("\x1bP>|" + strings.Repeat("x", size)))
			events := parser.Feed([]byte("\aq\x1b\\z"))
			if got := keysIn(events); !slices.Equal(got, []input.Key{{Rune: 'z'}}) {
				t.Fatalf("DCS body became keys: %+v", events)
			}
		})
	}
}

func TestCanceledDCSDoesNotSwallowFollowingKeys(t *testing.T) {
	for _, cancel := range []byte{0x18, 0x1a} {
		for _, size := range []int{8, 1 << 20} {
			t.Run(fmt.Sprintf("%x/%d", cancel, size), func(t *testing.T) {
				var parser input.Parser
				parser.Feed([]byte("\x1bP>|" + strings.Repeat("x", size)))
				events := parser.Feed([]byte{cancel, 'z'})
				if !slices.Equal(events, []input.Event{input.Key{Rune: 'z'}}) || parser.Pending() {
					t.Fatalf("DCS cancellation = %+v, pending=%v", events, parser.Pending())
				}
			})
		}
	}
}

func TestStringTerminatorKeepsItsIdentityAcrossEscapeTimeout(t *testing.T) {
	for _, family := range []struct{ name, prefix string }{
		{"OSC", "\x1b]52;c;"},
		{"DCS", "\x1bP>|"},
		{"APC", "\x1b_Gi=1;"},
	} {
		for _, size := range []int{32, 1 << 20} {
			for _, gap := range []time.Duration{
				input.DefaultEscapeTimeout - time.Nanosecond,
				input.DefaultEscapeTimeout,
				input.DefaultEscapeTimeout + time.Nanosecond,
			} {
				t.Run(fmt.Sprintf("%s/%d/%s", family.name, size, gap), func(t *testing.T) {
					var stream input.Stream
					at := time.Unix(1, 0)
					stream.Feed([]byte(family.prefix+strings.Repeat("x", size)+"\x1b"), at)
					if _, waiting := stream.DueAt(); waiting {
						t.Fatal("recognized payload acquired a keyboard deadline")
					}
					if events := stream.Expire(at.Add(gap)); len(events) != 0 {
						t.Fatalf("payload expired into input: %+v", events)
					}
					events := stream.Feed([]byte("\\z"), at.Add(gap))
					if got := keysIn(events); !slices.Equal(got, []input.Key{{Rune: 'z', At: at.Add(gap)}}) {
						t.Fatalf("terminator became typing: %+v", events)
					}
				})
			}
		}
	}
}

func TestStringEOFDiscardsTheUnfinishedUnitAndItsOrigin(t *testing.T) {
	for _, family := range []struct{ name, prefix string }{
		{"OSC", "\x1b]52;c;"},
		{"DCS", "\x1bP>|"},
		{"APC", "\x1b_Gi=1;"},
	} {
		for _, size := range []int{32, 1 << 20} {
			for _, tail := range []string{"", "\x1b"} {
				t.Run(fmt.Sprintf("%s/%d/tail_%x", family.name, size, tail), func(t *testing.T) {
					pending := []byte(family.prefix + strings.Repeat("x", size) + tail)
					var parser input.Parser
					parser.Feed(pending)
					if events := parser.Flush(); len(events) != 0 || parser.Pending() {
						t.Fatalf("EOF produced %+v, pending=%v", events, parser.Pending())
					}
					var stream input.Stream
					started := time.Unix(1, 0)
					next := started.Add(time.Second)
					stream.Feed(pending, started)
					if events := stream.Flush(next); len(events) != 0 {
						t.Fatalf("EOF invented keys or a reply: %+v", events)
					}
					if events := stream.Feed([]byte("z"), next); !slices.Equal(events, []input.Event{input.Key{Rune: 'z', At: next}}) {
						t.Fatalf("next unit inherited the unfinished frame: %+v", events)
					}
				})
			}
		}
	}
}

func keysIn(events []input.Event) []input.Key {
	var keys []input.Key
	for _, event := range events {
		if key, ok := event.(input.Key); ok {
			keys = append(keys, key)
		}
	}
	return keys
}
