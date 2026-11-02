// Command image shows a PNG and switches between crops of the same transmission.
// Run it from the repository root, optionally passing a different PNG path.
package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"

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
	// Run closes its own terminal. A supplied host survives Run, along with
	// any transmission the viewer did not explicitly retire.
	if images, ok := host.(program.ImageHost); ok {
		defer func() {
			if resource.Live() {
				err = errors.Join(err, images.ReleaseImage(resource))
			}
		}()
	}
	app, err := program.NewScreen(program.ScreenConfig{
		Host: host, Terminal: term.Features{Probe: true, Keyboard: term.KeyboardCompatible},
		Build: func(runtime *program.Runtime) (scene.Element, error) {
			images := runtime.Images()
			picture := kit.Image{Alt: "PNG preview unavailable: " + filepath.Base(path)}
			cell, known := images.CellSize()
			if known && images.Protocol().Supports(graphics.Live) {
				var uploadErr error
				resource, uploadErr = images.Transmit(data)
				if uploadErr != nil {
					return scene.Element{}, uploadErr
				}
				picture.Of, picture.Cell = resource, cell
			}
			return viewer(picture, runtime.Quit), nil
		},
	})
	if err != nil {
		return err
	}
	return app.Run(ctx)
}

func viewer(picture kit.Image, quit func()) scene.Element {
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
			preview := picture
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
