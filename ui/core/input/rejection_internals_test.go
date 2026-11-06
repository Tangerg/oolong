package input

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestOSCRejectionIsIndependentOfPayloadAndTerminatorPartitions(t *testing.T) {
	source := "\x1b]52;c;" + strings.Repeat("A", maxStringBody) + "\x1b\\x"
	want := []Event{OSCRejection{Command: 52, Prefix: "c;"}, Key{Code: Character, Rune: 'x'}}
	for _, split := range []int{1, 2, 5, 6, 7, maxStringBody - 1, maxStringBody, maxStringBody + 1, len(source) - 2} {
		var parser Parser
		events := parser.Feed([]byte(source[:split]))
		events = append(events, parser.Feed([]byte(source[split:]))...)
		if !slices.Equal(events, want) || parser.Pending() {
			t.Fatalf("split at %d = %+v, pending %v", split, events, parser.Pending())
		}
	}
}

func TestOSCRejectionRetainsOnlyTheCompleteBoundedHeader(t *testing.T) {
	for _, tc := range []struct {
		header, want string
	}{
		{header: "c;", want: "c;"},
		{header: "c;payload;", want: "c;"},
		{header: "c"},
		{header: strings.Repeat("c", maxParameterBytes-1) + ";", want: strings.Repeat("c", maxParameterBytes-1) + ";"},
		{header: strings.Repeat("c", maxParameterBytes) + ";"},
		{header: "\xff;", want: "�;"},
	} {
		var parser Parser
		parser.Feed([]byte("\x1b]52;" + tc.header + strings.Repeat("A", maxStringBody)))
		if len(parser.strBody) > maxParameterBytes || cap(parser.strBody) > maxParameterBytes {
			t.Fatalf("discard retained %d bytes, capacity %d", len(parser.strBody), cap(parser.strBody))
		}
		events := parser.Feed([]byte("\x07"))
		want := OSCRejection{Command: 52, Prefix: tc.want}
		if len(events) != 1 || events[0] != want {
			t.Fatalf("header of %d bytes produced %T, want bounded rejection", len(tc.header), events)
		}
		if !utf8.ValidString(events[0].(OSCRejection).Prefix) || ByteSize(events[0]) != eventOverhead+len(tc.want) || ByteSize(events[0]) > MaxDecodedEventBytes {
			t.Fatal("rejection escaped the immutable decoded-event contract")
		}
		if parser.strBody != nil || parser.Pending() {
			t.Fatal("settled rejection retained framing")
		}
	}
}

func TestOSCBodyBudgetRejectsAtTheSameLimit(t *testing.T) {
	for _, size := range []int{maxStringBody - 1, maxStringBody, maxStringBody + 1} {
		var parser Parser
		body := "c;" + strings.Repeat("A", size-len("c;"))
		events := parser.Feed([]byte("\x1b]52;" + body + "\x1b\\"))
		if len(events) != 1 {
			t.Fatalf("body size %d produced %d events", size, len(events))
		}
		if size < maxStringBody {
			if events[0] != (OSC{Command: 52, Params: body}) {
				t.Fatalf("body within budget produced %T", events[0])
			}
		} else if events[0] != (OSCRejection{Command: 52, Prefix: "c;"}) {
			t.Fatalf("body at or above budget produced %T", events[0])
		}
	}
}
