// Command mermaid prepares browser output away from the UI owner so rendering
// cannot stall terminal input.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/extensions/mermaid"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/kit"
)

const (
	flowchart = "flowchart TD\n Owner --> Worker --> PNG --> Owner --> Terminal"
	sequence  = "sequenceDiagram\n Owner->>Worker: Prepare source\n Worker-->>Owner: Neutral PNG\n Owner->>Terminal: Upload accepted image"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "mermaid:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) (err error) {
	backend, err := mermaid.New(mermaid.Config{Browser: os.Getenv("OOLONG_MERMAID_BROWSER")})
	if err != nil {
		return err
	}
	terminal, err := term.Open(term.Config{AltScreen: true, Features: term.Features{Probe: true, Mouse: true}})
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, terminal.Close()) }()
	return runScreen(ctx, program.TerminalHost(terminal), backend.Render)
}

// The host outlives Run so its image owner can release the final resource.
func runScreen(ctx context.Context, host program.Host, prepare func(context.Context, string) (*mermaid.Image, error)) (err error) {
	var screen *diagramScreen
	defer func() {
		if screen != nil {
			err = errors.Join(err, screen.close())
		}
	}()
	images, _ := host.(program.ImageHost)
	//nolint:contextcheck // Every scope the screen opens derives from the session context Run(ctx) creates.
	app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) {
		screen = newDiagramScreen(runtime, images, prepare)
		screen.show(flowchart)
		return screen.Element(), nil
	}})
	if err == nil {
		err = app.Run(ctx)
	}
	return err
}

type diagramScreen struct {
	element scene.Element
	runtime *program.Runtime
	prepare func(context.Context, string) (*mermaid.Image, error)
	open    func(context.Context, string) error
	theme   kit.Theme
	keys    *keymap.Map
	images  *imageLedger
	toolbar scene.Snapshot[[]toolbarButton]
	doc     markdown.Doc
	current diagram
	// workers counts every goroutine the screen started, so close returns only
	// after each has finished and every accepted upload is in the ledger.
	workers sync.WaitGroup
}

func newDiagramScreen(runtime *program.Runtime, images program.ImageHost, prepare func(context.Context, string) (*mermaid.Image, error)) *diagramScreen {
	s := &diagramScreen{
		runtime: runtime,
		prepare: prepare,
		open:    openImage,
		theme:   kit.Suited(runtime.Environment().Ground()),
		keys:    diagramKeys(),
		images:  &imageLedger{host: images},
	}
	s.element = scene.NewElement(scene.ElementConfig{
		Key:       "mermaid",
		Focusable: func() bool { return true },
		Paint:     s.paint,
		Handle:    s.handle,
	})
	return s
}

func (s *diagramScreen) Element() scene.Element { return s.element }

func (s *diagramScreen) paint(frame scene.Frame) {
	view := frame.View
	width, height := view.Size()
	if height < 1 {
		return
	}
	s.toolbar.Stage(frame, drawToolbar(view, s.keys, s.theme.Accent))
	s.doc.Draw(view.Sub(grid.Area(0, 1, width, max(0, height-2))))
	footer := view.Sub(grid.Area(0, height-1, width, 1))
	message, failed := s.current.status()
	switch {
	case failed:
		kit.Label{Text: message, Style: s.theme.Danger}.Draw(footer)
	case message != "":
		kit.Label{Text: message, Style: s.theme.Subtle}.Draw(footer)
	default:
		kit.Help{Theme: s.theme, Keys: s.keys, Show: []keymap.Action{replaceDiagramSource, quitDiagram}}.Draw(footer)
	}
}

func (s *diagramScreen) handle(event scene.Event) scene.EventResult {
	action, ok := s.actionFor(event)
	if !ok || !s.act(action) {
		return scene.EventResult{}
	}
	s.element.Invalidate()
	return scene.EventResult{Consumed: true}
}

// actionFor resolves a click against the toolbar of the frame it arrived in,
// which is the toolbar the user aimed at even if the bindings changed since.
func (s *diagramScreen) actionFor(event scene.Event) (keymap.Action, bool) {
	switch in := event.Input().(type) {
	case input.Mouse:
		if in.Action != input.MouseDown || in.Button != input.ButtonLeft {
			return "", false
		}
		buttons, _ := event.Read(&s.toolbar)
		return buttonAt(buttons, in.Pos)
	case input.Key:
		return s.keys.ActionFor(in)
	default:
		return "", false
	}
}

func (s *diagramScreen) act(action keymap.Action) bool {
	switch action {
	case quitDiagram:
		s.runtime.Quit()
	case replaceDiagramSource:
		if s.current.source == sequence {
			s.show(flowchart)
		} else {
			s.show(sequence)
		}
	case copyDiagramSource:
		s.copy(s.current.source, "Source copied", "Clipboard unavailable")
	case copyDiagramPath:
		if path, ok := s.export(); ok {
			s.copy(path, "Image path copied: "+path, "Clipboard unavailable: "+path)
		}
	case openDiagram:
		if path, ok := s.export(); ok {
			s.openExported(path)
		}
	default:
		return false
	}
	return true
}

func (s *diagramScreen) copy(value, copied, unavailable string) {
	if s.runtime.Clipboard().Copy(value) {
		s.current.notice = copied
	} else {
		s.current.notice = unavailable
	}
}

// close runs after Run has returned, when the program has already revoked every
// pending delivery, so no result can still reach the screen.
func (s *diagramScreen) close() error {
	if s.current.work != nil {
		s.current.work.Close()
	}
	s.workers.Wait()
	s.doc.SetBlocks(nil)
	return s.images.releaseAll()
}
