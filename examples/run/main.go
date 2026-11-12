// Command run publishes completed lines to terminal scrollback so only the open
// line remains revisable by the application.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Tangerg/oolong/examples/internal/drawn"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

const (
	defaultEditor       = "vi"
	spinnerTickInterval = 120 * time.Millisecond
	// An unfinished line has no publication receipt to bound its retained strings
	// or style metadata; command admission supplies both budgets.
	maxCommandLineBytes               = 64 << 10
	maxCommandLineSpans               = 1024
	leaveRunner         keymap.Action = "leave"
	openEditor          keymap.Action = "editor"
	suspendRunner       keymap.Action = "suspend"
)

func main() {
	command := os.Args[1:]
	if len(command) > 0 && command[0] == "--" {
		command = command[1:]
	}
	if len(command) == 0 {
		command = []string{"go", "version"}
	}
	app, err := program.NewInline(program.InlineConfig{Terminal: term.Features{Probe: true}, Build: func(runtime *program.InlineRuntime) (scene.Element, error) {
		runner, err := newRunner(runtime, command)
		if err != nil {
			return scene.Element{}, err
		}
		return runner.Element(), nil
	}})
	if err == nil {
		err = app.Run(context.Background())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "run:", err)
		os.Exit(1)
	}
}

type runner struct {
	element scene.Element
	runtime *program.InlineRuntime
	theme   kit.Theme
	command []string
	keys    *keymap.Map

	// Escape sequences may span command chunks, so one decoder must retain colour
	// and partial-sequence state for the whole output stream.
	out     *text.Decoder
	spinner kit.Spinner
	// outcome is nil while the command runs and set once when it settles.
	outcome   *runOutcome
	actionErr error
}

type runOutcome struct{ err error }

func (o runOutcome) String() string {
	if o.err != nil {
		return "failed: " + strings.ReplaceAll(o.err.Error(), "\n", "; ")
	}
	return "done"
}

func newRunner(runtime *program.InlineRuntime, command []string) (*runner, error) {
	theme := kit.Suited(runtime.Environment().Ground())
	out, err := text.NewDecoder(text.DecoderConfig{
		Base: theme.Text, MaxLineBytes: maxCommandLineBytes, MaxLineSpans: maxCommandLineSpans,
	})
	if err != nil {
		return nil, err
	}
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())
	r := &runner{runtime: runtime, theme: theme, command: command, keys: runnerKeys(), out: out}
	r.element = drawn.Element("run", r)
	r.spinner = kit.Spinner{Theme: theme, Glyphs: glyphs}

	// Terminal metadata is optional; the visible status remains authoritative
	// when a host cannot show window titles or notifications.
	_ = runtime.Session().SetTitle(strings.Join(command, " "))
	scope, err := runtime.NewScope(program.ScopeConfig{})
	if err != nil {
		r.finish(err)
		return r, nil
	}
	if _, err := runtime.Every(scope, spinnerTickInterval, r.spinner.Tick); err != nil {
		scope.Close()
		r.finish(err)
		return r, nil
	}
	if _, err := r.start(scope); err != nil {
		r.finish(err)
	}
	return r, nil
}

func runnerKeys() *keymap.Map {
	keys := new(keymap.Map)
	keys.Bind(leaveRunner, input.Ctrl.Rune('c'))
	keys.Bind(openEditor, input.Ctrl.Rune('e'))
	keys.Bind(suspendRunner, input.Ctrl.Rune('z'))
	return keys
}

func (r *runner) hint(action keymap.Action) string {
	bound := r.keys.Keys(action)
	if len(bound) == 0 {
		return ""
	}
	return bound[0].String() + ": " + action.String()
}

func (r *runner) start(scope *program.Scope) (*program.Task, error) {
	progress := make(chan struct{}, 1)
	unsubscribe := r.runtime.Publisher().Subscribe(func() {
		select {
		case progress <- struct{}{}:
		default:
		}
	})
	pump := r.pump(progress)
	task, err := r.runtime.Start(scope, program.Work[struct{}]{
		Run: func(ctx context.Context) (struct{}, error) { return struct{}{}, pump.run(ctx) },
		Apply: func(_ struct{}, err error) {
			unsubscribe()
			scope.Close()
			r.finish(err)
		},
	})
	if err != nil {
		unsubscribe()
		scope.Close()
	}
	return task, err
}

func (r *runner) pump(progress <-chan struct{}) *outputPump {
	return &outputPump{
		command:  r.command,
		owner:    r.runtime.Dispatcher(),
		progress: progress,
		feed:     r.out.Feed,
		flush:    r.out.Flush,
		offer:    r.offer,
	}
}

// offer publishes lines into scrollback until publication runs out of room and
// returns the lines it could not take yet.
func (r *runner) offer(lines []text.Line) ([]text.Line, error) {
	for len(lines) != 0 {
		paragraph := &kit.Paragraph{}
		paragraph.SetText(lines[:1])
		if _, err := r.runtime.Publisher().TryPublish(paragraph); err != nil {
			return lines, err
		}
		lines = lines[1:]
	}
	return nil, nil
}

func (r *runner) finish(err error) {
	r.outcome = &runOutcome{err: err}
	_ = r.runtime.Session().SetTitle("")
	_ = r.runtime.Session().Bell()
	_ = r.runtime.Session().Notify(strings.Join(r.command, " ") + ": " + r.outcome.String())
}

func (r *runner) Draw(v grid.View) {
	slots := []layout.Slot{
		{Size: layout.Fixed(1)},
		{Size: layout.Fixed(1)},
	}
	if r.actionErr != nil {
		slots = append(slots, layout.Slot{Size: layout.Fixed(1)})
	}
	rows := v.Subs((layout.Flow{Axis: layout.Down}).Rects(v.Bounds().Size(), slots))
	if r.actionErr != nil {
		kit.Label{Text: r.actionErr.Error(), Style: r.theme.Danger}.Draw(rows[2])
	}
	// The open line is drawn rather than printed: it is not finished, and what is
	// printed is finished for good.
	r.out.Open().Draw(rows[0], 0, 0)

	if r.outcome != nil {
		// The editor and suspension stay available after the command settles, so
		// their hints stay too: what is shown is what the keys do.
		label := r.outcome.String() + "   " + r.hints(leaveRunner, openEditor, suspendRunner)
		kit.Label{Text: label, Style: r.theme.Muted}.Draw(rows[1])
		return
	}
	spinner := r.spinner
	spinner.Label = strings.Join(r.command, " ")
	if hints := r.hints(openEditor, suspendRunner); hints != "" {
		spinner.Label += " — " + hints
	}
	spinner.Draw(rows[1])
}

func (r *runner) hints(actions ...keymap.Action) string {
	var hints []string
	for _, action := range actions {
		if hint := r.hint(action); hint != "" {
			hints = append(hints, hint)
		}
	}
	return strings.Join(hints, ", ")
}

func (r *runner) Handle(ev input.Event) bool {
	key, ok := ev.(input.Key)
	if !ok {
		return false
	}
	action, bound := r.keys.ActionFor(key)
	if !bound {
		return false
	}
	switch action {
	case leaveRunner:
		r.runtime.Quit()
	case openEditor:
		r.actionErr = r.runtime.Session().Hand(edit)
	case suspendRunner:
		r.actionErr = r.runtime.Session().Suspend()
	default:
		return false
	}
	return true
}

func edit() error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = defaultEditor
	}
	file, err := os.CreateTemp("", "oolong-*.txt")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if err = file.Close(); err != nil {
		return err
	}

	cmd, err := editorCommand(context.Background(), editor, file.Name())
	if err != nil {
		return err
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func (r *runner) Element() scene.Element { return r.element }
