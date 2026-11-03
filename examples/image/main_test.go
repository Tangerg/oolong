package main

import (
	"context"
	"image"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/uitest"
)

type imageHost struct {
	*uitest.Host
	resources graphics.ImageOwner
	image     graphics.Resource
}

func (h *imageHost) NativeTarget() grid.NativeTarget { return h.resources.NativeTarget() }
func (*imageHost) Graphics() graphics.Protocol       { return graphics.Kitty }
func (*imageHost) CellSize() (image.Point, bool)     { return image.Pt(8, 16), true }

func (h *imageHost) Transmit(_ context.Context, data []byte) (<-chan graphics.UploadResult, error) {
	wire, err := graphics.Transmit(io.Discard, 1, data)
	if err != nil {
		return nil, err
	}
	h.image = h.resources.Bind(wire)
	result := make(chan graphics.UploadResult, 1)
	result <- graphics.UploadResult{Resource: h.image}
	close(result)
	return result, nil
}

func (h *imageHost) ReleaseImage(resource graphics.Resource) error {
	return h.resources.Release(resource, func(wire graphics.Image) error { return wire.Release(io.Discard) })
}

func TestViewerCropsTheSamePNGAndQuits(t *testing.T) {
	host := &imageHost{Host: uitest.New(t, uitest.Config{Width: 40, Height: 12})}
	defer host.resources.RevokeAll()
	done := make(chan error, 1)
	go func() { done <- run(t.Context(), filepath.Join("../..", samplePNG), host) }()
	until := func(description, sequence string) {
		t.Helper()
		host.Until(t, description, func() bool { return strings.Contains(host.Frames(), sequence) })
	}
	until("whole image", "a=p,i=1,p=1,c=")
	host.Press(input.Keypad2)
	until("left half", "a=p,i=1,p=1,x=0,y=0,w=256,h=512,")
	host.Type("3")
	until("right half", "a=p,i=1,p=1,x=256,y=0,w=256,h=512,")
	host.Type("1")
	host.Until(t, "restored whole image", func() bool { return strings.Count(host.Frames(), "a=p,i=1,p=1,c=") >= 2 })
	host.Send(input.Key{Code: input.Character, Rune: 'с', BaseRune: 'c', Mods: input.Ctrl})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if host.image.Live() {
		t.Fatal("viewer left an image allocated on its caller-owned host")
	}
}

func TestViewerExplainsUnavailableGraphics(t *testing.T) {
	host := uitest.New(t, uitest.Config{Width: 48, Height: 5})
	done := make(chan error, 1)
	go func() { done <- run(t.Context(), filepath.Join("../..", samplePNG), host) }()
	host.Shows(t, "PNG preview unavailable: oolong-icon.png")
	host.Type("q")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(host.Frames(), "\x1b_G") {
		t.Fatal("plain host received graphics commands")
	}
}
