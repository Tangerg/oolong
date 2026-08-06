// Command run runs something and shows what it said, in colour.
//
// This is the shape of a program that drives other programs. Three things are worth
// looking at, and none of them is the running:
//
// The output arrives coloured, and stays coloured. A command writes escape sequences
// to say what it means — a failing test in red, a path underlined — and a decoder
// reads them back into styled text, a chunk at a time, because output does not
// arrive on line boundaries and a sequence can be split down the middle.
//
// What is finished belongs to the terminal. Every line the decoder completes is
// printed into the terminal's own scrollback, where it can be scrolled back to,
// selected, and found after this program exits. The live block at the bottom is the
// only part this program still owns.
//
// The terminal can be given away. Ctrl+E opens $EDITOR and takes the terminal back
// when it exits; Ctrl+Z stops this process the way a shell does. Both put the
// terminal back exactly as it was found, and both take it back the same way.
//
//	go run ./run -- go version
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Tangerg/oolong/components/kit"
	"github.com/Tangerg/oolong/core/grid"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
	"github.com/Tangerg/oolong/core/program"
	"github.com/Tangerg/oolong/core/term"
	"github.com/Tangerg/oolong/core/text"
)

func main() {
	command := os.Args[1:]
	if len(command) == 0 {
		command = []string{"go", "version"}
	}
	if err := program.Run(context.Background(), program.Config{
		Inline: func(runtime *program.InlineRuntime) program.Component {
			return newRunner(runtime, command)
		},
		Terminal: term.Options{Probe: true},
	}); err != nil {
		fmt.Fprintln(os.Stderr, "run:", err)
		os.Exit(1)
	}
}

// runner is the live block: what is still being written, and what is happening.
type runner struct {
	runtime  *program.InlineRuntime
	dispatch program.Dispatcher
	theme    kit.Theme
	command  []string

	// out reads the escape sequences a command wrote back into styled text. It holds
	// the state between chunks — the colour in force, half a sequence — which is why
	// it is a decoder and not a function.
	out     text.Decoder
	spinner kit.Spinner
	status  string
	done    bool
}

func newRunner(runtime *program.InlineRuntime, command []string) *runner {
	theme := kit.Suited(runtime.Environment().Ground())
	r := &runner{runtime: runtime, dispatch: runtime.Dispatcher(), theme: theme, command: command}
	r.out.Base = theme.Text
	r.spinner = kit.Spinner{Theme: theme, Label: strings.Join(command, " ")}
	r.status = "starting"

	// The window says what is happening to somebody looking at another window, which
	// is the one thing an interface cannot say by drawing.
	runtime.Session().SetTitle(strings.Join(command, " "))
	runtime.Every(120*time.Millisecond, r.spinner.Tick)
	go r.start()
	return r
}

// start runs the command and posts what it says.
//
// The reading happens here, on a goroutine of its own, and every chunk is posted to
// the one goroutine that owns the interface. That is the whole concurrency model:
// nothing else may touch what is on screen.
func (r *runner) start() {
	//nolint:gosec // G204: running what the user asked for on their own command line
	// is the whole of what this program is.
	cmd := exec.CommandContext(context.Background(), r.command[0], r.command[1:]...)
	pipe, opened := cmd.StdoutPipe()
	if opened != nil {
		r.dispatch.Post(func() { r.finish(opened) })
		return
	}
	cmd.Stderr = cmd.Stdout
	if started := cmd.Start(); started != nil {
		r.dispatch.Post(func() { r.finish(started) })
		return
	}

	reader := bufio.NewReader(pipe)
	buf := make([]byte, 4096)
	for {
		n, read := reader.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			r.dispatch.Post(func() { r.output(chunk) })
		}
		if read != nil {
			break
		}
	}
	ended := cmd.Wait()
	r.dispatch.Post(func() { r.finish(ended) })
}

// output takes a piece of what the command said. Everything a newline finished
// belongs to the terminal from here on; what is left is still being written.
func (r *runner) output(chunk string) {
	for _, line := range r.out.Feed(chunk) {
		r.runtime.Print(&kit.Paragraph{Lines: []text.Line{line}})
	}
}

// finish prints whatever was left, says how it went, and asks for attention.
func (r *runner) finish(err error) {
	for _, line := range r.out.Flush() {
		r.runtime.Print(&kit.Paragraph{Lines: []text.Line{line}})
	}
	r.done = true
	r.status = "done"
	if err != nil {
		r.status = err.Error()
	}
	r.runtime.Session().SetTitle("")
	// The two things that reach somebody who is looking at another window.
	r.runtime.Session().Bell()
	r.runtime.Session().Notify(strings.Join(r.command, " ") + ": " + r.status)
}

// Draw is the line still being written, and a row saying what is happening.
func (r *runner) Draw(v grid.View) {
	rows := v.Subs(layout.Down.Rects(v.Bounds().Size(),
		layout.Slot{Size: layout.Fixed(1)},
		layout.Slot{Size: layout.Fixed(1)},
	))
	// The open line is drawn rather than printed: it is not finished, and what is
	// printed is finished for good.
	r.out.Open().Draw(rows[0], 0, 0)

	if r.done {
		kit.Label{Text: r.status + "   ctrl+c: leave", Style: r.theme.Muted}.Draw(rows[1])
		return
	}
	r.spinner.Label = strings.Join(r.command, " ") + " — ctrl+e: editor, ctrl+z: suspend"
	r.spinner.Draw(rows[1])
}

func (r *runner) Handle(ev input.Event) bool {
	key, ok := ev.(input.Key)
	if !ok || !key.Down() || !key.Mods.Has(input.Ctrl) {
		return false
	}
	switch key.Rune {
	case 'c':
		r.runtime.Quit()
	case 'e':
		// The terminal is given away and taken back: the modes this session turned on
		// go off in the opposite order, the child gets a terminal with no idea a
		// program was using it, and the whole of that happens again in reverse.
		if err := r.runtime.Session().Hand(edit); err != nil {
			r.status = err.Error()
		}
	case 'z':
		if err := r.runtime.Session().Suspend(); err != nil {
			r.status = err.Error()
		}
	default:
		return false
	}
	return true
}

// edit opens whatever the user's editor is, on a scratch file.
func edit() error {
	name := os.Getenv("EDITOR")
	if name == "" {
		name = "vi"
	}
	file, err := os.CreateTemp("", "oolong-*.txt")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if err := file.Close(); err != nil {
		return err
	}

	//nolint:gosec // G204: the editor is the user's own, out of their environment.
	cmd := exec.CommandContext(context.Background(), name, file.Name())
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
