package term

import (
	"strings"
	"sync"

	"github.com/Tangerg/oolong/core/ansi"
)

// The things a program says to the terminal that are not a frame.
//
// Each is one sequence, and each is ignored by a terminal that does not implement
// it — which is the whole reason they can be sent without asking first.
const (
	// titleSet names the window. Command 0 sets the icon name and the window title
	// together: a terminal that keeps them apart shows the same thing in both
	// places, and a program that had to choose between them would be choosing on
	// behalf of every terminal.
	titleSet = "\x1b]0;"
	// titlePush and titlePop are the title stack. What a session set has to be put
	// back for the same reason a mode does — a shell whose window is still called
	// "building oolong" an hour after the build is a program that left something
	// behind.
	titlePush = "\x1b[22;0t"
	titlePop  = "\x1b[23;0t"

	// notifySend is the desktop notification iTerm2 defined and others followed. It
	// is the one with any reach; the alternatives are one terminal apiece.
	notifySend = "\x1b]9;"
)

// title is the window title a session set, and what has to be put back after it.
//
// It is guarded because it can be set from anywhere — a download reporting its
// progress is not on the interface's goroutine — and read by the session's own
// unwinding.
type title struct {
	mu sync.Mutex
	// pushed says the terminal is holding the title this session found, so it owes a
	// pop.
	pushed bool
	text   string
}

// to advances the retained title and its output in the same order.
func (t *title) to(s string, queue func([]byte) uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := ""
	if !t.pushed {
		// Whatever the terminal was called is kept before it is replaced, which is
		// what makes putting it back possible at all: there is no way to ask a
		// terminal what its title is.
		out = titlePush
		t.pushed = true
	}
	t.text = strings.Clone(s)
	queue([]byte(out + command(titleSet, s)))
}

// enter is what to write to show the title again after the terminal has been given
// away and taken back, and nothing when this session never set one.
func (t *title) enter() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.pushed {
		return ""
	}
	return titlePush + command(titleSet, t.text)
}

func (t *title) leave() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.pushed {
		return ""
	}
	return titlePop
}

// command is a string command: an introducer, a body that cannot end it early, and
// the terminator every terminal understands.
func command(intro, body string) string {
	var b strings.Builder
	b.Grow(len(intro) + len(body) + 1)
	b.WriteString(intro)
	b.WriteString(printable(body))
	b.WriteByte(ansi.Bell)
	return b.String()
}

// printable drops what cannot go inside a sequence.
//
// The text comes from a program's own output, a file name, or a model's answer, and
// any of those may hold an escape byte or a bell. Either would end the sequence
// early and leave the rest of the text to be read as commands by the terminal, which
// is the oldest trick there is. This is the same trust boundary a cell keeps and it
// is kept here for the same reason.
func printable(s string) string {
	s = strings.ToValidUTF8(s, "�")
	if !strings.ContainsFunc(s, unprintable) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !unprintable(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func unprintable(r rune) bool { return r < 0x20 || r >= 0x7f && r <= 0x9f }
