package uitest

import (
	"strings"
	"testing"
	"testing/synctest"
)

func TestVisibleRunsJoinAppearanceBoundaries(t *testing.T) {
	frame := "\x1b[1;1H\x1b[31mhello \x1b]8;;https://example.test\x1b\\world\x1b]8;;\x1b\\\x1b[0m"
	got, err := visibleRuns(frame)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "hello world") {
		t.Fatalf("repaint text = %q, want text joined across appearance", got)
	}
}

func TestVisibleRunsKeepGeometryBoundaries(t *testing.T) {
	for _, seq := range []string{"\x1b[2;1H", "\x1b[?6h", "\x1b[8;10;20t", "\x1b]1337;File=inline=1:AAAA\x07", "\x1bPgraphics\x1b\\"} {
		got, err := visibleRuns("hello " + seq + "world")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(got, "hello world") {
			t.Errorf("%q joined text across geometry operation %q", got, seq)
		}
	}
}

func TestTextAssertionCannotInterpretCustomCharacterSetsAsASCII(t *testing.T) {
	if got, err := visibleRuns("\x1b(0qqq"); err == nil {
		t.Fatalf("reported %q for line-drawing glyphs", got)
	}
}

func TestVisibilityWaitsForOutputInsteadOfRequestingIdleFrames(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		host := New(t, Config{Width: 20, Height: 2})
		done := make(chan struct{}, 2)
		wait := func() {
			defer func() { done <- struct{}{} }()
			host.Shows(t, "ready")
		}
		go wait()
		reply := <-host.Repaints()
		_, _ = host.out.Write([]byte("pending"))
		reply(20, 2, nil)
		synctest.Wait()
		go wait()
		reply = <-host.Repaints()
		_, _ = host.out.Write([]byte("pending"))
		reply(20, 2, nil)
		synctest.Wait()
		select {
		case reply = <-host.Repaints():
			t.Error("visibility assertion itself requested another idle frame")
		default:
			_, _ = host.out.Write([]byte("changed"))
			reply = <-host.Repaints()
		}
		_, _ = host.out.Write([]byte("ready"))
		reply(20, 2, nil)
		<-done
		<-done
	})
}
