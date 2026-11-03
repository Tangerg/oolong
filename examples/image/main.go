// Command image retains its host until the final image is released so terminal
// teardown cannot orphan a transmission.
package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sync"

	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/keymap"
	"github.com/Tangerg/oolong/ui/core/program"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/kit"
)

const samplePNG = "docs/public/brand/oolong-icon.png"

func main() {
	path := samplePNG
	switch len(os.Args) {
	case 1:
	case 2:
		path = os.Args[1]
	default:
		fmt.Fprintln(os.Stderr, "usage: image [file.png]")
		os.Exit(2)
	}
	if err := run(context.Background(), path, nil); err != nil {
		fmt.Fprintln(os.Stderr, "image:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, path string, host program.Host) (err error) {
	data, err := os.ReadFile(path) //nolint:gosec // The user selects the PNG to display.
	if err != nil {
		return err
	}
	var resource graphics.Resource
	var scope *program.Scope
	var workers sync.WaitGroup
	defer func() {
		if scope != nil {
			scope.Close()
		}
		workers.Wait()
		if images, ok := host.(program.ImageHost); ok && resource.Live() {
			err = errors.Join(err, images.ReleaseImage(resource))
		}
	}()
	app, err := program.NewScreen(program.ScreenConfig{
		Host: host, Terminal: term.Features{Probe: true, Keyboard: term.KeyboardCompatible},
		Build: func(runtime *program.Runtime) (scene.Element, error) {
			images := runtime.Images()
			picture := &kit.Image{Alt: "PNG preview unavailable: " + filepath.Base(path)}
			element := viewer(picture, runtime.Quit)
			cell, known := images.CellSize()
			if !known || !images.Protocol().Supports(graphics.Live) {
				return element, nil
			}
			var scopeErr error
			scope, scopeErr = runtime.NewScope(program.ScopeConfig{Context: ctx}) //nolint:contextcheck // The scope owns the upload's explicit parent lifetime.
			if scopeErr != nil {
				return scene.Element{}, scopeErr
			}
			result, uploadErr := images.Transmit(scope.Context(), data) //nolint:contextcheck // ScopeConfig above derives this request lifetime from ctx.
			if uploadErr != nil {
				return scene.Element{}, uploadErr
			}
			receive := func() (graphics.Resource, error) {
				defer workers.Done()
				outcome := <-result
				resource = outcome.Resource
				return outcome.Resource, outcome.Err
			}
			workers.Add(1)
			_, startErr := runtime.Start(scope, program.Work[graphics.Resource]{
				Run: func(context.Context) (graphics.Resource, error) { return receive() },
				Apply: func(image graphics.Resource, uploadErr error) {
					if uploadErr != nil {
						picture.Alt = "PNG upload failed: " + uploadErr.Error()
					} else {
						picture.Of, picture.Cell = image, cell
					}
					element.Invalidate()
				},
			})
			if startErr != nil {
				scope.Close()
				go func() { _, _ = receive() }()
				return scene.Element{}, startErr
			}
			return element, nil
		},
	})
	if err != nil {
		return err
	}
	return app.Run(ctx)
}

func viewer(picture *kit.Image, quit func()) scene.Element {
	const (
		full  keymap.Action = "full"
		left  keymap.Action = "left"
		right keymap.Action = "right"
		exit  keymap.Action = "quit"
	)
	var keys keymap.Map
	keys.Bind(full, input.Chord{Rune: '1'})
	keys.Bind(left, input.Chord{Rune: '2'})
	keys.Bind(right, input.Chord{Rune: '3'})
	keys.Bind(exit, input.Chord{Rune: 'q'})
	keys.Bind(exit, input.Ctrl.Rune('c'))

	var element scene.Element
	element = scene.NewElement(scene.ElementConfig{
		Key:       "png",
		Focusable: func() bool { return true },
		Paint: func(frame scene.Frame) {
			width, height := frame.Size()
			frame.Text(0, 0, "1 full   2 left   3 right   q quit", grid.Style{})
			preview := *picture
			preview.MaxRows = max(0, height-1)
			preview.Draw(frame.View.Sub(grid.Area(0, 1, width, preview.MaxRows)))
		},
		Handle: func(event scene.Event) scene.EventResult {
			key, ok := event.Input().(input.Key)
			if !ok {
				return scene.EventResult{}
			}
			action, bound := keys.ActionFor(key)
			if !bound {
				return scene.EventResult{}
			}
			size := picture.Of.Size()
			var source image.Rectangle
			switch action {
			case exit:
				quit()
				return scene.EventResult{Consumed: true}
			case left:
				source = image.Rect(0, 0, max(1, size.X/2), size.Y)
			case right:
				source = image.Rect(size.X/2, 0, size.X, size.Y)
			}
			if picture.Of.Live() && picture.Source != source {
				picture.Source = source
				element.Invalidate()
			}
			return scene.EventResult{Consumed: true}
		},
	})
	return element
}
