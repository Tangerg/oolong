package term

import (
	"strings"
	"sync"

	"github.com/Tangerg/oolong/ui/core/ansi"
)

// Session metadata is separate from frames because its restoration obligation
// survives ordinary redraws.
const (
	// OSC 0 updates icon and window titles together so destinations that
	// distinguish them do not retain inconsistent names.
	titleSet = "\x1b]0;"
	// Balance the title stack so a session does not leave its title on the shell.
	titlePush = "\x1b[22;0t"
	titlePop  = "\x1b[23;0t"

	notifySend = "\x1b]9;"
	commandEnd = string(rune(ansi.Bell))
)

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
	if len(set) > controlByteLimit-modeBytes-len(titlePush) {
		return ErrPayloadTooLarge
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := set
	if t.set == "" {
		out = titlePush + set
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
	return titlePush + t.set
}

func (t *title) leave() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.set == "" {
		return ""
	}
	return titlePop
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
