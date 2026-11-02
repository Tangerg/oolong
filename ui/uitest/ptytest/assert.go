package ptytest

import (
	"strconv"
	"strings"
)

// TestingT lets assertion tests capture failures without failing the enclosing test.
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

type Mode struct {
	Name    string
	On, Off string
}

// RequireSymmetricModes checks reverse teardown because terminal modes interact:
// leaving the alternate screen before releasing its mouse mode changes which
// screen receives the release. Each mode must be acquired and released once.
func RequireSymmetricModes(t TestingT, transcript []byte, modes ...Mode) {
	t.Helper()
	text := string(transcript)

	ordered := make([]string, 2*len(modes))
	for i, mode := range modes {
		if mode.On == "" || mode.Off == "" {
			t.Errorf("%s needs non-empty on and off sequences", mode.Name)
			return
		}
		if n := strings.Count(text, mode.On); n != 1 {
			t.Errorf("%s was turned on %d times, want once", mode.Name, n)
			return
		}
		if n := strings.Count(text, mode.Off); n != 1 {
			t.Errorf("%s was turned off %d times, want once", mode.Name, n)
			return
		}
		ordered[i], ordered[len(ordered)-1-i] = mode.On, mode.Off
	}
	RequireOrdered(t, transcript, ordered...)
}
