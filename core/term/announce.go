package term

import (
	"fmt"
	"os"
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

// to is what to write to show s, remembering it.
func (t *title) to(s string) string {
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
	return out + command(titleSet, s)
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

// leave is what to write to put back the title the session found.
func (t *title) leave() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.pushed {
		return ""
	}
	return titlePop
}

// SetTitle names the terminal's window, and remembers to put back whatever it was
// called before.
//
// The title is where a program says what it is doing to somebody who is not looking
// at it: a tab in a window behind this one, a taskbar entry, a window list. That is
// the whole of its value, and it is why the text should be the task and not the
// program's name — which the user can already see.
//
// It is queued beside the frames, so it lands between two of them and never inside
// one. A terminal that does not implement it ignores it, and one that does not
// implement the title stack ignores the putting back — which is why a program that
// cares should set something sensible on the way out rather than rely on it.
func (t *Terminal) SetTitle(s string) { t.writer.Queue([]byte(t.title.to(s))) }

// Bell asks the terminal for its attention.
//
// What that is, is the user's to decide and not this program's: a sound, a flash of
// the window, a mark on the tab, or nothing at all. That is the reason to send this
// rather than to invent an attention-getting animation — the user has already told
// their terminal what they want to happen.
func (t *Terminal) Bell() { t.writer.Queue([]byte{ansi.Bell}) }

// Notify asks for a desktop notification.
//
// It is for the thing that finished while the user was looking at something else,
// which is the case a terminal interface cannot answer on its own: the window is
// not on screen, so nothing drawn in it is seen.
//
// Terminals that do not implement it ignore it, and there is no way to find out
// which did — so a program that has something to say should say it in the interface
// as well, and treat this as the extra it is.
func (t *Terminal) Notify(text string) {
	t.writer.Queue([]byte(command(notifySend, text)))
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

func unprintable(r rune) bool { return r < 0x20 || r == 0x7f }

// LogTo opens a file for a program's diagnostics while the terminal is taken.
//
// An interface owns the screen: a line printed to standard output lands in the
// middle of a frame, moves everything below it, and is gone by the next repaint. So
// a program being debugged needs somewhere else to write, and the usual somewhere is
// a file being followed in another window.
//
// It is opened for appending, so a run does not lose the run before it, and it is
// the caller's to close. Pointing the standard library's logger at it is one line
// and is deliberately not done here — a library that reached into a program's global
// logging would be deciding something that is not its to decide:
//
//	f, err := term.LogTo("debug.log")
//	if err != nil {
//		return err
//	}
//	defer f.Close()
//	log.SetOutput(f)
func LogTo(path string) (*os.File, error) {
	//nolint:gosec // G304: the path is the caller's, and opening the file they named
	// is the whole of what this does.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("term: open a log: %w", err)
	}
	return f, nil
}
