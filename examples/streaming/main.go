// Command streaming is a chat interface that streams, in about a hundred lines.
//
// It is the shape the library is for: what has been said is printed into the
// terminal's own scrollback, where it stays after the program exits, and what is
// still happening is a live block at the bottom. Type something and press Enter;
// a canned reply arrives a word at a time. Ctrl+C leaves.
//
// There is no model behind it and no network. What is being demonstrated is the
// interface, and the reply is a timer.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/keymap"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
)

func main() {
	if err := program.Run(context.Background(), program.Config{
		Inline: func(runtime *program.InlineRuntime) program.Component {
			return headless.NewRoot(newChat(runtime))
		},
		// Asking costs one round trip and is the only way to learn two things a
		// program cannot work out for itself: what colour the terminal draws on, and
		// what it will do with an image.
		Terminal: term.Options{Probe: true},
	}); err != nil {
		fmt.Fprintln(os.Stderr, "streaming:", err)
		os.Exit(1)
	}
}

// chat is the whole interface: a composer, and a status line while a reply is
// arriving.
type chat struct {
	runtime *program.InlineRuntime
	theme   kit.Theme

	// keys is one table for the whole interface: what the field does with a keystroke,
	// and what this program does with one. A widget reading through it answers the
	// actions it knows and lets the rest past, which is why there is one and not three.
	keys *keymap.Map

	// body is the two of them arranged, and the thing that decides which of them an
	// event is for. Laying them out by hand would mean translating a click into the
	// composer's own box by hand too — and getting it wrong on exactly the frames
	// where the status line is there and everything below it has moved down a row.
	body     *headless.Container
	composer kit.Composer
	status   kit.Status

	// answering is the reply still being streamed, and stop ends the timer driving
	// it. Both are nil when nothing is happening, which is what the status line and
	// the frame schedule are both keyed off.
	answering []string
	said      int
	stop      func()
}

// What this program can be asked to do, on top of what a text field can. A name and
// not a keystroke: what produces one is the keymap's business, and the hint row reads
// the answer back out of the same table rather than being told it again here.
const (
	send keymap.Action = "send"
	quit keymap.Action = "quit"
)

// Clipboard is what an editor wants, which is worth saying out loud: the interface
// is declared by the consumer and satisfied by a concrete runtime capability.
var _ headless.Clipboard = program.Clipboard{}

func newChat(runtime *program.InlineRuntime) *chat {
	// The look follows what the terminal said. A theme that has to be told whether the
	// terminal is light is a theme that is wrong for half the people who run it.
	theme := kit.Suited(runtime.Environment().Ground())
	// The furniture follows the locale, because a terminal that is not in UTF-8 draws
	// a box character as mojibake and there is no way to ask it.
	glyphs := kit.GlyphsFor(os.Getenv)
	// The field's own keys, and this program's two on top of them.
	keys := headless.DefaultEditorKeys()
	keys.Bind(send, input.Chord{Code: input.Enter})
	keys.Bind(quit, input.Ctrl.Rune('c'))

	c := &chat{runtime: runtime, theme: theme, keys: keys}
	c.composer = kit.Composer{
		Theme:       theme,
		Prompt:      glyphs.Marker + " ",
		Placeholder: "Ask something, or press ctrl+c to leave",
		Keys:        keys,
		Hints:       []keymap.Action{send, headless.InsertNewline, quit},
	}
	c.status = kit.Status{Theme: theme}
	// Copy and cut go to the terminal, which over ssh or through a multiplexer is the
	// only end of the connection the user is at.
	c.composer.Editor().Clipboard = runtime.Clipboard()
	c.body = headless.Rows(
		headless.Item{Size: layout.Fixed(0), Of: headless.Static{Of: &c.status}},
		headless.Item{Size: layout.Measured(1, 0), Of: &c.composer},
	)
	return c
}

// Draw stacks the status line over the composer.
//
// The status row is there only while something is happening, which is one number
// rather than a branch around the drawing: a slot of no rows is a slot, and the
// container works out what that leaves for everything else.
func (c *chat) Draw(v headless.Frame) {
	c.body.Items[0].Size = layout.Fixed(c.statusRows())
	c.body.Draw(v)
}

func (c *chat) statusRows() int {
	if c.answering == nil {
		return 0
	}
	return 1
}

// Handle runs this program's own two actions. Everything else belongs to whichever
// part of the interface it is for, which is the container's to work out.
//
// The map is asked what a chord names rather than read through, because these two
// keys are one chord each and the reading is done further down by the field. Asking is
// a question about the table; reading is a thing with a memory, and two of those over
// one table would each know half of what was typed.
func (c *chat) Handle(ev input.Event) bool {
	if key, ok := ev.(input.Key); ok && key.Down() {
		action, _ := c.keys.Action(key.Chord())
		switch action {
		case send:
			c.send()
			return true
		case quit:
			c.runtime.Quit()
			return true
		}
	}
	return c.body.Handle(ev)
}

// send prints what was typed and starts a reply arriving.
func (c *chat) send() {
	asked := strings.TrimSpace(c.composer.Text())
	if asked == "" || c.answering != nil {
		return
	}
	c.composer.Reset()
	c.runtime.Print(kit.Message{Theme: c.theme, Speaker: "you", Body: asked, Own: true})

	c.answering = strings.Fields(reply(asked))
	c.said = 0
	c.status.Doing = "thinking"
	// The clock exists only while something is animating. An interface with nothing
	// happening asks for no frames at all.
	c.stop = c.runtime.Every(90*time.Millisecond, c.advance)
}

// advance adds a word to the reply, and prints the whole thing once it is done.
func (c *chat) advance() {
	c.status.Tick()
	c.said++
	c.status.Doing = strings.Join(c.answering[:min(c.said, len(c.answering))], " ")
	if c.said < len(c.answering) {
		return
	}
	c.runtime.Print(kit.Message{Theme: c.theme, Speaker: "assistant", Body: strings.Join(c.answering, " ")})
	c.stop()
	c.answering, c.stop = nil, nil
}

// reply is the canned answer. A real one would arrive over a network; what this
// program is showing is what happens to it when it does.
func reply(asked string) string {
	return "You said " + clip(asked) + ". This reply is a timer, not a model — " +
		"what is worth looking at is where it ends up: this paragraph is being " +
		"printed into your terminal's own scrollback, so you can scroll back to it, " +
		"select it, and still find it after this program exits. The block below is " +
		"the only part the program still owns."
}

func clip(s string) string {
	if len(s) > 40 {
		return s[:40] + "…"
	}
	return s
}
