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
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
)

func main() {
	if err := program.Run(context.Background(), program.Config{
		Inline: func(loop program.InlineLoop) program.Component { return newChat(loop) },
	}); err != nil {
		fmt.Fprintln(os.Stderr, "streaming:", err)
		os.Exit(1)
	}
}

// chat is the whole interface: a composer, and a status line while a reply is
// arriving.
type chat struct {
	loop  program.InlineLoop
	theme kit.Theme

	composer kit.Composer
	status   kit.Status

	// answering is the reply still being streamed, and stop ends the timer driving
	// it. Both are nil when nothing is happening, which is what the status line and
	// the frame schedule are both keyed off.
	answering []string
	said      int
	stop      func()

	// width is what the last frame was drawn at. Printing needs it before there is a
	// view to ask, because a printed block is measured before it is drawn.
	width int
}

var quit = headless.Binding{Key: input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl}, Does: "quit"}

func newChat(loop program.InlineLoop) *chat {
	theme := kit.Dark()
	c := &chat{loop: loop, theme: theme}
	c.theme = theme
	c.composer = kit.Composer{
		Theme:       theme,
		Prompt:      "› ",
		Placeholder: "Ask something, or press ctrl+c to leave",
		Hints: []headless.Binding{
			{Key: input.Key{Code: input.Enter}, Does: "send"},
			{Key: input.Key{Code: input.Enter, Mods: input.Alt}, Does: "newline"},
			quit,
		},
	}
	c.status = kit.Status{Theme: theme}
	return c
}

// Draw stacks the status line over the composer, and remembers the width so that
// printing can measure against it.
func (c *chat) Draw(v grid.View) {
	c.width, _ = v.Size()
	rows := layout.Rows(v,
		layout.Slot{Size: layout.Fixed(c.statusRows())},
		layout.Slot{Size: layout.Measured(1, 0), Of: &c.composer},
	)
	if c.statusRows() > 0 {
		c.status.Draw(rows[0])
	}
	c.composer.Draw(rows[1])
}

func (c *chat) statusRows() int {
	if c.answering == nil {
		return 0
	}
	return 1
}

// Handle sends on Enter and leaves on ctrl+c. Everything else is the composer's.
func (c *chat) Handle(ev input.Event) bool {
	if quit.Matches(ev) {
		c.loop.Quit()
		return true
	}
	if key, ok := ev.(input.Key); ok && key.Down() && key.Is(input.Enter, 0) {
		c.send()
		return true
	}
	return c.composer.Handle(ev)
}

// send prints what was typed and starts a reply arriving.
func (c *chat) send() {
	asked := strings.TrimSpace(c.composer.Text())
	if asked == "" || c.answering != nil {
		return
	}
	c.composer.Reset()
	c.print(kit.Message{Theme: c.theme, Speaker: "you", Body: asked, Own: true})

	c.answering = strings.Fields(reply(asked))
	c.said = 0
	c.status.Doing = "thinking"
	// The clock exists only while something is animating. An interface with nothing
	// happening asks for no frames at all.
	c.stop = c.loop.Every(90*time.Millisecond, c.advance)
}

// advance adds a word to the reply, and prints the whole thing once it is done.
func (c *chat) advance() {
	c.status.Tick()
	c.said++
	c.status.Doing = strings.Join(c.answering[:min(c.said, len(c.answering))], " ")
	if c.said < len(c.answering) {
		return
	}
	c.print(kit.Message{Theme: c.theme, Speaker: "assistant", Body: strings.Join(c.answering, " ")})
	c.stop()
	c.answering, c.stop = nil, nil
}

// print puts a finished message into the terminal's scrollback, above the block.
func (c *chat) print(m kit.Message) {
	c.loop.Print(m.Measure(c.width), m.Draw)
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
