package main

import (
	"context"
	"errors"
	"image"
	"strings"

	"github.com/Tangerg/oolong/examples/internal/markdownlook"
	"github.com/Tangerg/oolong/extensions/markdown"
	"github.com/Tangerg/oolong/extensions/mermaid"
	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

const diagramRows = 16

// diagramPhase is how far the current source has come toward the screen.
type diagramPhase uint8

const (
	diagramRendering diagramPhase = iota
	diagramUploading
	diagramShown // the image, or the source standing in for it, is final
	diagramFailed
)

// diagram is everything that belongs to one source. Replacing the source
// replaces the whole value, so nothing of a superseded diagram survives it.
type diagram struct {
	source  string
	phase   diagramPhase
	failure error
	image   *mermaid.Image
	// exported is the file every open and copy of this image names once one
	// of them has written it.
	exported string
	notice   string
	// work owns every task of this diagram; closing it revokes their results.
	work *program.Scope
}

func (d *diagram) status() (message string, failed bool) {
	switch d.phase {
	case diagramFailed:
		return d.failure.Error(), true
	case diagramRendering, diagramUploading:
		return "Preparing Mermaid…", false
	default:
		return d.notice, false
	}
}

func (s *diagramScreen) show(source string) {
	if s.current.work != nil {
		s.current.work.Close()
	}
	s.current = diagram{source: source, phase: diagramRendering}
	// The document must stop drawing the previous image before it is released.
	s.present(nil)
	if err := s.images.releaseAll(); err != nil {
		s.fail(err)
		return
	}
	work, err := s.runtime.NewScope(program.ScopeConfig{})
	if err != nil {
		s.fail(err)
		return
	}
	s.current.work = work
	prepare := s.prepare
	err = s.start(func(ctx context.Context) (*mermaid.Image, error) { return prepare(ctx, source) }, s.rendered)
	if err != nil {
		work.Close()
		s.fail(err)
	}
}

func (s *diagramScreen) rendered(result *mermaid.Image, err error) {
	if err != nil {
		s.fail(err)
		return
	}
	s.current.image = result
	images := s.runtime.Images()
	cell, known := images.CellSize()
	if !images.Protocol().Supports(graphics.Live) || !known || cell.X <= 0 || cell.Y <= 0 {
		s.current.phase = diagramShown
		s.current.notice = "Inline images unavailable; source shown. Open Image to view the diagram."
		return
	}
	owner, ledger, png := s.runtime.Dispatcher(), s.images, result.PNG()
	err = s.start(func(ctx context.Context) (graphics.Resource, error) {
		var upload <-chan graphics.UploadResult
		admissionErr := owner.Invoke(ctx, func() error {
			var transmitErr error
			upload, transmitErr = images.Transmit(ctx, png)
			return transmitErr
		})
		if admissionErr != nil {
			return graphics.Resource{}, admissionErr
		}
		return ledger.receive(ctx, upload)
	}, func(resource graphics.Resource, err error) { s.uploaded(resource, cell, err) })
	if err != nil {
		s.fail(err)
		return
	}
	s.current.phase = diagramUploading
}

func (s *diagramScreen) uploaded(resource graphics.Resource, cell image.Point, err error) {
	if err != nil {
		s.fail(err)
		return
	}
	s.current.phase = diagramShown
	s.present(kit.Image{Of: resource, Cell: cell, MaxRows: diagramRows, Alt: "Mermaid diagram", Theme: s.theme})
}

func (s *diagramScreen) fail(err error) {
	s.current.phase, s.current.failure = diagramFailed, err
}

// start runs work in the current diagram's scope and counts its worker.
func (s *diagramScreen) start[T any](run func(context.Context) (T, error), apply func(T, error)) error {
	s.workers.Add(1)
	_, err := s.runtime.Start(s.current.work, program.Work[T]{
		Run: func(ctx context.Context) (T, error) {
			defer s.workers.Done()
			return run(ctx)
		},
		Apply: apply,
	})
	if err != nil {
		s.workers.Done()
	}
	return err
}

// present embeds the diagram in its document: child is the image, or nil to
// let the source stand in for it.
func (s *diagramScreen) present(child grid.Drawable) {
	source := s.current.source
	if child == nil {
		var lines []text.Line
		for line := range strings.SplitSeq(source, "\n") {
			lines = append(lines, text.Of(line, s.theme.Text))
		}
		child = text.NewBlock(text.BlockConfig{Lines: lines, Wrap: true})
	}
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
	if err != nil {
		s.fail(err)
	}
}
