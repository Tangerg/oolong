package clipboard

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestARefusedReadExpiresBeforeAnUnrelatedAnswer(t *testing.T) {
	now := time.Unix(1, 0)
	channel := &Channel{now: func() time.Time { return now }}
	if _, ok := channel.Request(System); !ok {
		t.Fatal("initial request was refused")
	}
	now = now.Add(responseTimeout + time.Nanosecond)
	params := "c;" + base64.StdEncoding.EncodeToString([]byte("unrelated"))
	if text, ok := channel.Answer(params); ok {
		t.Fatalf("expired request accepted %q", text)
	}
	if _, ok := channel.Request(System); !ok {
		t.Fatal("an expired request still owned the channel")
	}
}

func TestAnUnreadableMatchingAnswerSettlesTheRequest(t *testing.T) {
	channel := &Channel{}
	if _, ok := channel.Request(System); !ok {
		t.Fatal("request was refused")
	}
	if text, ok := channel.Answer("c;not-base64"); ok {
		t.Fatalf("unreadable answer was accepted as %q", text)
	}
	if _, ok := channel.Request(System); !ok {
		t.Fatal("the unreadable answer did not settle its request")
	}
}
