package term

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/oolong/ui/core/ansi"
)

// Session metadata is separate from frames because its restoration obligation
// survives ordinary redraws.
var (
	titleSet   = ansi.OSCPrefix(ansi.OSCTitle)
	notifySend = ansi.OSCPrefix(ansi.OSCNotify)
)

const commandEnd = string(rune(ansi.Bell))

// Title changes can originate outside the UI goroutine, so admission and the
// restoration obligation share one lock.
type title struct {
	mu sync.Mutex
	// An accepted command also owns the obligation to pop the saved title.
	set string
}

func (t *title) to(s string, modeBytes int, queue func([]byte) (uint64, error)) error {
	set := titleCommand(s)
	// Reacquisition restores modes and the title stack in one reserved frame.
	if len(set) > controlByteLimit-modeBytes-len(ansi.PushTitle) {
		return ErrPayloadTooLarge
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := set
	if t.set == "" {
		out = ansi.PushTitle + set
	}
	if _, err := queue([]byte(out)); err != nil {
		return err
	}
	t.set = set
	return nil
}

func (t *title) enter() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.set == "" {
		return ""
	}
	return ansi.PushTitle + t.set
}

func (t *title) leave() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.set == "" {
		return ""
	}
	return ansi.PopTitle
}

func titleCommand(text string) string { return titleSet + printable(text) + commandEnd }

func notification(text string) string {
	body := printable(text)
	// OSC 9 also selects ConEmu commands by the first digit. Frame numeric
	// text as a bullet so even a notification from an untrusted source stays text.
	if first := strings.TrimSpace(body); first != "" && first[0] >= '0' && first[0] <= '9' {
		body = "• " + body
	}
	return notifySend + body + commandEnd
}

// Strip control bytes before framing metadata: ESC and BEL can end an OSC
// early and turn the remaining caller text into terminal commands.
func printable(s string) string {
	return strings.Map(dropControl, strings.ToValidUTF8(s, string(utf8.RuneError)))
}

func dropControl(r rune) rune {
	if unicode.IsControl(r) {
		return -1
	}
	return r
}

// ReportDirectory lets the terminal resolve relative paths in output after
// the program changes directory. The path becomes absolute; empty uses the
// process working directory. A nil error means queued with the frames, while
// Writer reports delivery.
func (t *Terminal) ReportDirectory(path string) error {
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("term: report the directory: %w", err)
		}
		path = cwd
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("term: report the directory: %w", err)
	}
	// The host is part of the form and is what tells a terminal on the other end of an
	// ssh connection that the path is not one of its own. A machine that cannot name
	// itself reports none, which is the form's own way of saying "wherever this is".
	host, _ := os.Hostname()
	location := (&url.URL{
		Scheme: "file",
		Host:   host,
		Path:   filepath.ToSlash(abs),
	}).String()
	_, err = t.writer.Queue([]byte(ansi.OSCPrefix(ansi.OSCDirectory) + location + ansi.ST))
	return err
}

// SetTitle retains the title-stack restoration obligation alongside writer
// admission. Peers may omit title-stack support, so restoration is best-effort.
// Titles too large to restate after handover return ErrPayloadTooLarge.
func (t *Terminal) SetTitle(s string) error {
	return t.title.to(s, len(t.modes.Enter()), t.writer.Queue)
}

// Bell delegates attention policy to the user's terminal configuration.
func (t *Terminal) Bell() error { _, err := t.writer.Queue([]byte{ansi.Bell}); return err }

// Notify is supplementary: unsupported peers may ignore it without a reply.
// Numeric-leading text receives a bullet so OSC 9 peers cannot interpret it as
// a control command in their shared notification namespace.
func (t *Terminal) Notify(text string) error {
	_, err := t.writer.Queue([]byte(notification(text)))
	return err
}
