// Command run publishes completed lines to terminal scrollback so only the open
// line remains revisable by the application.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/publication"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

const (
	defaultEditor                     = "vi"
	spinnerTickInterval               = 120 * time.Millisecond
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
		return newRunner(runtime, command).Element(), nil
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
	out       text.Decoder
	spinner   kit.Spinner
	result    string
	actionErr error
}

func newRunner(runtime *program.InlineRuntime, command []string) *runner {
	theme := kit.Suited(runtime.Environment().Ground())
	glyphs := kit.GlyphsFor(runtime.Environment().Locale())
	r := &runner{runtime: runtime, theme: theme, command: command, keys: runnerKeys()}
	r.out.Base = theme.Text
	r.spinner = kit.Spinner{Theme: theme, Glyphs: glyphs}

	// Terminal metadata is optional; the visible status remains authoritative
	// when a host cannot show window titles or notifications.
	_ = runtime.Session().SetTitle(strings.Join(command, " "))
	scope, err := runtime.NewScope(program.ScopeConfig{})
	if err != nil {
		r.finish(err)
		return r
	}
	if _, err := runtime.Every(scope, spinnerTickInterval, r.spinner.Tick); err != nil {
		scope.Close()
		r.finish(err)
		return r
	}
	if _, err := r.start(scope); err != nil {
		r.finish(err)
	}
	return r
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
	task, err := r.runtime.Start(scope, program.Work[struct{}]{
		Run: func(ctx context.Context) (struct{}, error) { return struct{}{}, r.run(ctx, progress) },
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

type commandWriter struct{ write func([]byte) error }

func (w *commandWriter) Write(data []byte) (int, error) {
	if err := w.write(data); err != nil {
		return 0, err
	}
	return len(data), nil
}

func (r *runner) run(ctx context.Context, progress <-chan struct{}) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	output := &commandWriter{write: func(data []byte) error {
		err := r.publish(ctx, progress, func() []text.Line { return r.out.Feed(string(data)) })
		if err != nil {
			cancel(err)
		}
		return err
	}}
	result := runCommand(ctx, r.command, output)
	if cause := context.Cause(ctx); cause != nil {
		return errors.Join(result, cause)
	}
	return errors.Join(result, r.publish(ctx, progress, r.out.Flush))
}

// The worker retains at most one command read while waiting for publication.
// Only the UI owner advances the decoder and offers lines. An accepted prefix is
// never replayed; the worker waits for a capacity notification before retrying.
func (r *runner) publish(ctx context.Context, progress <-chan struct{}, decode func() []text.Line) error {
	var lines []text.Line
	for {
		err := r.runtime.Dispatcher().Invoke(ctx, func() error {
			if decode != nil {
				lines = decode()
				decode = nil
			}
			for len(lines) != 0 {
				paragraph := &kit.Paragraph{}
				paragraph.SetText(lines[:1])
				if _, err := r.runtime.Publisher().TryPublish(paragraph); err != nil {
					return err
				}
				lines = lines[1:]
			}
			return nil
		})
		if !errors.Is(err, publication.ErrCapacity) {
			return err
		}
		select {
		case <-progress:
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
}

func (r *runner) finish(err error) {
	r.result = "done"
	if err != nil {
		r.result = "failed: " + err.Error()
	}
	_ = r.runtime.Session().SetTitle("")
	_ = r.runtime.Session().Bell()
	_ = r.runtime.Session().Notify(strings.Join(r.command, " ") + ": " + r.result)
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

	if r.result != "" {
		kit.Label{Text: r.result + "   " + r.hint(leaveRunner), Style: r.theme.Muted}.Draw(rows[1])
		return
	}
	spinner := r.spinner
	var hints []string
	for _, action := range []keymap.Action{openEditor, suspendRunner} {
		if hint := r.hint(action); hint != "" {
			hints = append(hints, hint)
		}
	}
	spinner.Label = strings.Join(r.command, " ")
	if len(hints) != 0 {
		spinner.Label += " — " + strings.Join(hints, ", ")
	}
	spinner.Draw(rows[1])
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
		// The terminal is given away and taken back: the modes this session turned on
		// go off in the opposite order, the child gets a terminal with no idea a
		// program was using it, and the whole of that happens again in reverse.
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

func (r *runner) Element() scene.Element {
	if r.element.IsZero() {
		r.element = scene.NewElement(scene.ElementConfig{
			Key:       "run",
			Focusable: func() bool { return true },
			Paint:     func(frame scene.Frame) { r.Draw(frame.View) },
			Handle: func(event scene.Event) scene.EventResult {
				return scene.EventResult{Consumed: r.Handle(event.Input())}
			},
		})
	}
	return r.element
}
