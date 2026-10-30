package clipboard

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestARefusedReadExpiresAtItsDeadline(t *testing.T) {
	for _, elapsed := range []time.Duration{responseTimeout, responseTimeout + time.Nanosecond} {
		t.Run(elapsed.String(), func(t *testing.T) {
			now := time.Unix(1, 0)
			channel := &Channel{now: func() time.Time { return now }}
			if !channel.Request(System, func(string) bool { return true }) {
				t.Fatal("initial request was refused")
			}
			now = now.Add(elapsed)
			params := "c;" + base64.StdEncoding.EncodeToString([]byte("unrelated"))
			if text, ok := channel.Answer(params, channel.clock()); ok {
				t.Fatalf("expired request accepted %q", text)
			}
			if !channel.Request(System, func(string) bool { return true }) {
				t.Fatal("an expired request still owned the channel")
			}
		})
	}
}

func TestAReadRemainsLiveBeforeItsDeadline(t *testing.T) {
	now := time.Unix(1, 0)
	channel := &Channel{now: func() time.Time { return now }}
	if !channel.Request(System, func(string) bool { return true }) {
		t.Fatal("initial request was refused")
	}
	now = now.Add(responseTimeout - time.Nanosecond)
	if channel.Request(System, func(string) bool { return true }) {
		t.Fatal("a live request was replaced")
	}
	params := "c;" + base64.StdEncoding.EncodeToString([]byte("answer"))
	if text, ok := channel.Answer(params, channel.clock()); !ok || text != "answer" {
		t.Fatalf("live request answer = %q, %t", text, ok)
	}
}

func TestAnUnreadableMatchingAnswerSettlesTheRequest(t *testing.T) {
	channel := &Channel{}
	if !channel.Request(System, func(string) bool { return true }) {
		t.Fatal("request was refused")
	}
	if text, ok := channel.Answer("c;not-base64", channel.clock()); ok {
		t.Fatalf("unreadable answer was accepted as %q", text)
	}
	if !channel.Request(System, func(string) bool { return true }) {
		t.Fatal("the unreadable answer did not settle its request")
	}
}

func TestAnswerEligibilityUsesArrivalTime(t *testing.T) {
	for _, beforeRequest := range []bool{false, true} {
		t.Run(map[bool]string{false: "delayed decoding", true: "before request"}[beforeRequest], func(t *testing.T) {
			now := time.Unix(1, 0)
			channel := &Channel{now: func() time.Time { return now }}
			arrived := now
			if beforeRequest {
				now = now.Add(time.Second)
			}
			channel.Request(System, func(string) bool { return true })
			if !beforeRequest {
				now = now.Add(responseTimeout + time.Second)
			}
			params := "c;" + base64.StdEncoding.EncodeToString([]byte("answer"))
			text, ok := channel.Answer(params, arrived)
			if ok == beforeRequest {
				t.Fatalf("arrival eligibility = %q, %t", text, ok)
			}
			if beforeRequest {
				if text, ok := channel.Answer(params, now); !ok || text != "answer" {
					t.Fatal("old arrival settled a newer request")
				}
			}
		})
	}
}
