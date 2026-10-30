package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	"os"
	"sync"
	"testing"

	"github.com/Tangerg/oolong/ui/core/program"

	"github.com/Tangerg/oolong/extensions/mermaid"
	"github.com/Tangerg/oolong/ui/core/graphics"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/uitest"
)

type imageHost struct {
	*uitest.Host
	mu             sync.Mutex
	sent, released uint32
	protocol       graphics.Protocol
	unknownCell    bool
	copied         string
}

func (h *imageHost) Graphics() graphics.Protocol   { return h.protocol }
func (h *imageHost) CellSize() (image.Point, bool) { return image.Pt(8, 16), !h.unknownCell }
func (h *imageHost) Transmit([]byte) (graphics.Image, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sent++
	return graphics.Image{ID: h.sent, Size: image.Pt(80, 32)}, nil
}

func (h *imageHost) ReleaseImage(graphics.Image) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.released++
	return nil
}

func TestShutdownCancelsWorker(t *testing.T) {
	host := uitest.New(t, uitest.Config{Width: 70, Height: 20})
	started, stopped := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- runScreen(t.Context(), host, func(ctx context.Context, _ string) (*mermaid.Image, error) {
			close(started)
			<-ctx.Done()
			close(stopped)
			return nil, context.Cause(ctx)
		})
	}()
	<-started
	host.Shows(t, "Preparing Mermaid")
	host.Type("q")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("worker outlived screen")
	}
}

func TestReplacementRejectsStaleResultAndReleasesAcceptedImages(t *testing.T) {
	host := &imageHost{protocol: graphics.Kitty, Host: uitest.New(t, uitest.Config{Width: 75, Height: 25})}
	first, releaseFirst := make(chan struct{}), make(chan struct{})
	var calls int
	var mu sync.Mutex
	done := make(chan error, 1)
	go func() {
		done <- runScreen(t.Context(), host, func(_ context.Context, _ string) (*mermaid.Image, error) {
			mu.Lock()
			calls++
			call := calls
			mu.Unlock()
			if call == 1 {
				close(first)
				<-releaseFirst
			}
			// An empty neutral value is sufficient here: this fake host owns transport.
			return new(mermaid.Image), nil
		})
	}()
	<-first
	host.Shows(t, "Preparing Mermaid")
	host.Type("r")
	host.Shows(t, "Mermaid as embedded content")
	host.Until(t, "first image upload", func() bool { host.mu.Lock(); defer host.mu.Unlock(); return host.sent == 1 })
	close(releaseFirst)
	host.Type("r")
	host.Shows(t, "Mermaid as embedded content")
	host.Until(t, "replacement image upload", func() bool { host.mu.Lock(); defer host.mu.Unlock(); return host.sent == 2 })
	host.Type("q")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if host.sent < 1 || host.sent != host.released {
		t.Fatalf("sent=%d released=%d", host.sent, host.released)
	}
}

func TestBackendDiagnosticIsVisible(t *testing.T) {
	host := uitest.New(t, uitest.Config{Width: 70, Height: 20})
	done := make(chan error, 1)
	go func() {
		done <- runScreen(t.Context(), host, func(context.Context, string) (*mermaid.Image, error) { return nil, errors.New("diagram syntax failed") })
	}()
	host.Shows(t, "diagram syntax failed")
	host.Type("q")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestClosedPreparationCannotAdvanceContentState(t *testing.T) {
	screen := &diagramScreen{closed: true, pending: true, source: "current source"}
	screen.accept(new(mermaid.Image), nil)
	if !screen.pending || screen.err != nil || screen.image.ID != 0 || screen.doc.Len() != 0 {
		t.Fatal("closed content accepted a preparation result")
	}
}

func (h *imageHost) Copy(value string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.copied = value
	return true
}

func TestUnsupportedTerminalsNeverReceiveImageData(t *testing.T) {
	for _, tt := range []struct {
		name     string
		protocol graphics.Protocol
		unknown  bool
	}{{"none", graphics.None, false}, {"sixel", graphics.Sixel, false}, {"geometry", graphics.Kitty, true}} {
		t.Run(tt.name, func(t *testing.T) {
			host := &imageHost{Host: uitest.New(t, uitest.Config{Width: 100, Height: 20}), protocol: tt.protocol, unknownCell: tt.unknown}
			done := make(chan error, 1)
			go func() {
				done <- runScreen(t.Context(), host, func(context.Context, string) (*mermaid.Image, error) { return new(mermaid.Image), nil })
			}()
			host.Shows(t, "Inline images unavailable")
			host.Shows(t, "flowchart TD")
			host.Send(input.Mouse{Pos: image.Pt(41, 0), Action: input.MouseDown, Button: input.ButtonLeft})
			host.Shows(t, "Source copied")
			host.Type("p")
			host.Shows(t, "Image path copied")
			host.Type("q")
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			host.mu.Lock()
			path, sent := host.copied, host.sent
			host.mu.Unlock()
			if sent != 0 {
				t.Fatal("unsupported terminal received image data")
			}
			if path == "" {
				t.Fatal("no exported path")
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("export did not survive exit: %v", err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExportPreservesBytes(t *testing.T) {
	data := []byte("owned image bytes")
	path, err := exportPNG(data)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if removeErr := os.Remove(path); removeErr != nil {
			t.Error(removeErr)
		}
	})
	got, err := os.ReadFile(path) //nolint:gosec // G304: path was freshly allocated by the export under test.
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("export changed image data")
	}
}

func TestImageOpenActionUsesAcceptedExport(t *testing.T) {
	host := uitest.New(t, uitest.Config{Width: 100, Height: 20})
	screen := &diagramScreen{source: flowchart, prepared: new(mermaid.Image)}
	opened := make(chan string, 1)
	screen.open = func(_ context.Context, path string) error { opened <- path; return nil }
	done := make(chan error, 1)
	go func() {
		app, err := program.NewScreen(program.ScreenConfig{Host: host, Build: func(r *program.Runtime) (scene.Element, error) {
			screen.runtime = r
			scope, scopeErr := r.NewScope(program.ScopeConfig{Context: t.Context()})
			if scopeErr != nil {
				return scene.Element{}, scopeErr
			}
			screen.scope, screen.lifetime = scope, scope
			screen.writer = host.Writer()
			screen.setDocument(nil)
			return screen.Element(), nil
		}})
		if err == nil {
			err = app.Run(t.Context())
		}
		done <- err
	}()
	host.Shows(t, "Open Image")
	host.Type("o")
	host.Shows(t, "Opened image")
	host.Type("q")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := screen.close(); err != nil {
		t.Fatal(err)
	}
	path := <-opened
	if path != screen.exported {
		t.Fatal("viewer opened a different generation")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}
