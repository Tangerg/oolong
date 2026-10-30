// Command mermaid prepares a diagram in a worker and embeds its accepted image in
// Markdown. Install the official mmdc executable before running. Optional browser
// selection uses OOLONG_MERMAID_BROWSER. Press r to replace the source, q to leave.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Tangerg/oolong/ui/core/text"

	"github.com/Tangerg/oolong/examples/internal/markdownlook"
	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/extensions/mermaid"
	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/kit"
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

// The host outlives Run so image erasure and release remain ordered on its writer.
func runScreen(ctx context.Context, host program.Host, prepare func(context.Context, string) (*mermaid.Image, error)) (err error) {
	images, _ := host.(program.ImageHost)
	var screen *diagramScreen
	defer func() {
		if screen != nil {
			err = errors.Join(err, screen.close())
		}
	}()
	app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(runtime *program.Runtime) (scene.Element, error) {
		lifetime, lifetimeErr := runtime.NewScope(program.ScopeConfig{Context: ctx}) //nolint:contextcheck // The explicit config field carries ctx into the lifetime owner.
		if lifetimeErr != nil {
			return scene.Element{}, lifetimeErr
		}
		screen = &diagramScreen{lifetime: lifetime, runtime: runtime, writer: host.Writer(), images: images, prepare: prepare, open: openImage, theme: kit.Suited(runtime.Environment().Ground())}
		screen.start(ctx, flowchart)
		return screen.Element(), nil //nolint:contextcheck // Future input starts work through screen.lifetime, derived from ctx above.
	}})
	if err == nil {
		err = app.Run(ctx)
	}
	return err
}

type diagramScreen struct {
	element          scene.Element
	lifetime         *program.Scope
	scope            *program.Scope
	runtime          *program.Runtime
	writer           program.FrameWriter
	images           program.ImageHost
	prepare          func(context.Context, string) (*mermaid.Image, error)
	theme            kit.Theme
	doc              markdown.Doc
	image            graphics.Image
	err              error
	source           string
	prepared         *mermaid.Image
	exported, notice string
	open             func(context.Context, string) error
	workers          sync.WaitGroup
	pending, closed  bool
}

func (s *diagramScreen) start(ctx context.Context, source string) {
	if s.scope != nil {
		s.scope.Close()
	}
	defer s.element.Invalidate()
	s.source = source
	s.prepared = nil
	s.exported, s.notice = "", ""
	s.pending = true
	s.doc.SetBlocks(nil)
	s.err = s.release()
	s.setDocument(nil)
	if s.err != nil {
		s.pending = false
		return
	}
	scope, err := s.runtime.NewScope(program.ScopeConfig{Context: ctx}) //nolint:contextcheck // Context is passed explicitly through ScopeConfig and then into every Work.Run.
	if err != nil {
		s.err, s.pending = err, false
		return
	}
	s.scope = scope
	prepare := s.prepare
	s.workers.Add(1)
	_, err = s.runtime.Start(scope, program.Work[*mermaid.Image]{
		Run: func(ctx context.Context) (*mermaid.Image, error) {
			defer s.workers.Done()
			return prepare(ctx, source)
		},
		Apply: s.accept,
	})
	if err != nil {
		s.workers.Done()
		scope.Close()
		s.err, s.pending = err, false
	}
}

func (s *diagramScreen) accept(result *mermaid.Image, err error) {
	if s.closed {
		return
	}
	s.pending = false
	s.err = err
	if err != nil {
		return
	}
	if result == nil {
		s.err = errors.New("backend returned no diagram")
		return
	}
	s.prepared = result
	images := s.runtime.Images()
	cell, known := images.CellSize()
	if !images.Protocol().Supports(graphics.Live) || !known || cell.X <= 0 || cell.Y <= 0 {
		s.notice = "Inline images unavailable; source shown. Open Image to view the diagram."
		s.setDocument(nil)
		return
	}
	handle, err := images.Transmit(result.PNG())
	if err != nil {
		s.err = err
		return
	}
	s.image = handle
	s.setDocument(kit.Image{Of: handle, Cell: cell, MaxRows: 16, Alt: "Mermaid diagram", Theme: s.theme})
}

func (s *diagramScreen) setDocument(child grid.Drawable) {
	if child == nil {
		var lines []text.Line
		for line := range strings.SplitSeq(s.source, "\n") {
			lines = append(lines, text.Of(line, s.theme.Text))
		}
		child = text.NewBlock(text.BlockConfig{Lines: lines, Wrap: true})
	}
	source := s.source
	look := markdownlook.New(s.theme, kit.GlyphsFor(s.runtime.Environment().Locale()))
	look.SetRenderer(markdown.FencedCode, func(info, body string) (grid.Drawable, error) {
		if info != "mermaid" {
			return nil, markdown.ErrUnhandled
		}
		if body != source {
			return nil, errors.New("diagram source does not match prepared image")
		}
		return child, nil
	})
	blocks, err := markdown.Render("# Mermaid as embedded content\n\n```mermaid\n"+source+"\n```", look)
	s.doc.SetBlocks(blocks)
	s.err = errors.Join(s.err, err)
}

func (s *diagramScreen) release() error {
	if s.image.ID == 0 {
		return nil
	}
	var removal bytes.Buffer
	if err := s.image.Placement(1).Erase(&removal); err != nil {
		return err
	}
	if _, err := s.writer.Queue(removal.Bytes()); err != nil {
		return err
	}
	if err := s.images.ReleaseImage(s.image); err != nil {
		return err
	}
	s.image = graphics.Image{}
	return nil
}

func (s *diagramScreen) close() error {
	s.closed = true
	if s.lifetime != nil {
		s.lifetime.Close()
	}
	if s.scope != nil {
		s.scope.Close()
	}
	s.workers.Wait()
	s.doc.SetBlocks(nil)
	return s.release()
}

func (s *diagramScreen) Draw(view grid.View) {
	width, height := view.Size()
	if height < 1 {
		return
	}
	s.drawActions(view.Sub(grid.Area(0, 0, width, 1)))
	s.doc.Draw(view.Sub(grid.Area(0, 1, width, max(0, height-2))))
	status, style := "r replaces source · q quits", s.theme.Subtle
	switch {
	case s.err != nil:
		status, style = s.err.Error(), s.theme.Danger
	case s.pending:
		status = "Preparing Mermaid…"
	case s.notice != "":
		status = s.notice
	}
	kit.Label{Text: status, Style: style}.Draw(view.Sub(grid.Area(0, height-1, width, 1)))
}

func (s *diagramScreen) Handle(event input.Event) bool {
	if mouse, ok := event.(input.Mouse); ok {
		return mouse.Pos.Y == 0 && mouse.Action == input.MouseDown && mouse.Button == input.ButtonLeft && s.clickAction(mouse.Pos.X)
	}
	key, ok := event.(input.Key)
	if !ok || !key.Down() {
		return false
	}
	if key.Rune == 'q' || key.Rune == 'c' && key.Mods.Has(input.Ctrl) {
		s.runtime.Quit()
		return true
	}
	if s.act(key.Rune) {
		s.element.Invalidate()
		return true
	}
	if key.Rune == 'r' {
		source := sequence
		if s.source == sequence {
			source = flowchart
		}
		s.start(s.lifetime.Context(), source)
		return true
	}
	return false
}

const (
	flowchart = "flowchart TD\n Owner --> Worker --> PNG --> Owner --> Terminal"
	sequence  = "sequenceDiagram\n Owner->>Worker: Prepare source\n Worker-->>Owner: Neutral PNG\n Owner->>Terminal: Upload accepted image"
)

func (s *diagramScreen) drawActions(view grid.View) {
	x := 0
	for _, action := range diagramActions() {
		kit.Label{Text: action.label, Style: s.theme.Accent}.Draw(view.Sub(grid.Area(x, 0, text.Width(action.label), 1)))
		x += text.Width(action.label) + 2
	}
}

func (s *diagramScreen) clickAction(x int) bool {
	offset := 0
	for _, action := range diagramActions() {
		width := text.Width(action.label)
		if x >= offset && x < offset+width {
			consumed := s.act(action.key)
			s.element.Invalidate()
			return consumed
		}
		offset += width + 2
	}
	return false
}

func (s *diagramScreen) act(key rune) bool {
	switch key {
	case 'c':
		if !s.runtime.Clipboard().Copy(s.source) {
			s.notice = "Clipboard unavailable"
		} else {
			s.notice = "Source copied"
		}
	case 'o', 'p':
		path, ok := s.exportedPath()
		if !ok {
			return true
		}
		if key == 'p' {
			if !s.runtime.Clipboard().Copy(path) {
				s.notice = "Clipboard unavailable: " + path
			} else {
				s.notice = "Image path copied: " + path
			}
			return true
		}
		s.openExported(path)
	default:
		return false
	}
	return true
}

func (s *diagramScreen) exportedPath() (string, bool) {
	if s.prepared == nil {
		s.notice = "Image is not ready"
		return "", false
	}
	if s.exported == "" {
		path, err := exportPNG(s.prepared.PNG())
		if err != nil {
			s.notice = err.Error()
			return "", false
		}
		s.exported = path
	}
	return s.exported, true
}

// openExported hands the file to the system viewer off the interface goroutine, so
// drawing is not blocked and the terminal is not held open while the viewer runs.
func (s *diagramScreen) openExported(path string) {
	open := s.open
	s.workers.Add(1)
	_, err := s.runtime.Start(s.scope, program.Work[struct{}]{
		Run: func(ctx context.Context) (struct{}, error) {
			defer s.workers.Done()
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			return struct{}{}, open(ctx, path)
		},
		Apply: func(_ struct{}, err error) {
			if err != nil {
				s.notice = err.Error()
			} else {
				s.notice = "Opened image: " + path
			}
		},
	})
	if err != nil {
		s.workers.Done()
		s.notice = err.Error()
	}
}

func (s *diagramScreen) Element() scene.Element {
	if s.element.IsZero() {
		s.element = scene.NewElement(scene.ElementConfig{
			Key:       "mermaid",
			Focusable: func() bool { return true },
			Paint:     func(frame scene.Frame) { s.Draw(frame.View) },
			Handle: func(event scene.Event) scene.EventResult {
				return scene.EventResult{Consumed: s.Handle(event.Input)}
			},
		})
	}
	return s.element
}
