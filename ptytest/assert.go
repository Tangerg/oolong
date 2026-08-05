package ptytest

import (
	"strconv"
	"strings"
)

// TestingT is the part of [testing.TB] these assertions use.
//
// An interface rather than *testing.T so that the assertions can be tested: a
// test that checks an assertion fails when it should has to be able to catch the
// failure, and it cannot do that with the real thing.
type TestingT interface {
	Helper()
	Errorf(format string, args ...any)
}

// quoteAll renders tokens the way a failure should read them: quoted, so an
// escape sequence is visible rather than obeyed by the terminal reading the test
// output.
func quoteAll(tokens []string) []string {
	out := make([]string, len(tokens))
	for i, token := range tokens {
		out[i] = strconv.Quote(token)
	}
	return out
}

// RequireContains asserts that every token appears somewhere.
func RequireContains(t TestingT, transcript []byte, tokens ...string) {
	t.Helper()
	text := string(transcript)
	for _, token := range tokens {
		if !strings.Contains(text, token) {
			t.Errorf("the transcript never contains %s\ngot:\n%s",
				strconv.Quote(token), strconv.Quote(text))
		}
	}
}

// RequireNotContains asserts that no token appears.
//
// It is how the interesting half of a renderer's promises are stated: what an
// idle frame must not write, what a shrinking block must not leave behind.
func RequireNotContains(t TestingT, transcript []byte, tokens ...string) {
	t.Helper()
	text := string(transcript)
	for _, token := range tokens {
		if strings.Contains(text, token) {
			t.Errorf("the transcript contains %s and should not\ngot:\n%s",
				strconv.Quote(token), strconv.Quote(text))
		}
	}
}

// RequireOrdered asserts that the tokens appear in the order given. Anything may
// appear between them.
func RequireOrdered(t TestingT, transcript []byte, tokens ...string) {
	t.Helper()
	text := string(transcript)
	at := 0
	for i, token := range tokens {
		found := strings.Index(text[at:], token)
		if found < 0 {
			t.Errorf("the transcript has no %s after the %d token(s) before it\ngot:\n%s",
				strconv.Quote(token), i, strconv.Quote(text))
			return
		}
		at += found + len(token)
	}
}

// Mode pairs the sequence that turns a terminal mode on with the one that turns
// it off.
type Mode struct {
	Name    string
	On, Off string
}

// RequireSymmetricModes asserts that a session gave the terminal back exactly as
// it found it.
//
// Every mode is turned on once and off once, the on-sequences appear in the order
// given, and the off-sequences appear in the reverse of it. That last part is the
// one worth checking and the one nothing else would catch: modes interact, and a
// session that leaves the alternate screen before it releases the mouse has
// released the mouse of a screen that is already gone.
//
// A terminal left in a mode nobody turned off is a terminal the user has to
// close, which is why this is worth a dedicated assertion rather than three
// RequireOrdered calls.
func RequireSymmetricModes(t TestingT, transcript []byte, modes ...Mode) {
	t.Helper()
	text := string(transcript)

	ons := make([]int, len(modes))
	offs := make([]int, len(modes))
	for i, mode := range modes {
		if n := strings.Count(text, mode.On); n != 1 {
			t.Errorf("%s was turned on %d times, want once", mode.Name, n)
			return
		}
		if n := strings.Count(text, mode.Off); n != 1 {
			t.Errorf("%s was turned off %d times, want once", mode.Name, n)
			return
		}
		ons[i] = strings.Index(text, mode.On)
		offs[i] = strings.Index(text, mode.Off)
		if offs[i] < ons[i] {
			t.Errorf("%s was turned off before it was turned on", mode.Name)
			return
		}
	}

	for i := 1; i < len(modes); i++ {
		if ons[i] < ons[i-1] {
			t.Errorf("%s was turned on before %s, which is out of order",
				modes[i].Name, modes[i-1].Name)
		}
		// The reverse order on the way out: what was turned on last comes off first.
		if offs[i] > offs[i-1] {
			t.Errorf("%s was turned off after %s; a session has to be unwound in the "+
				"order it was set up, backwards", modes[i].Name, modes[i-1].Name)
		}
	}
}
