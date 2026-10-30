//go:build unix || windows

package mermaid_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/oolong/extensions/mermaid"
)

func TestOfficialCLI(t *testing.T) {
	executable := os.Getenv("OOLONG_MERMAID_CLI")
	if executable == "" {
		t.Skip("set OOLONG_MERMAID_CLI for installed official backend integration")
	}
	renderer, err := mermaid.New(mermaid.Config{Executable: executable, Browser: os.Getenv("OOLONG_MERMAID_BROWSER")})
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"flowchart TD\n A[Owner] --> B[Worker]\n B --> C[PNG]", "sequenceDiagram\n Alice->>Bob: Hello", `flowchart LR; A["$$x^2$$"] --> B["$$y^2$$"]`} {
		result, err := renderer.Render(t.Context(), source)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := png.Decode(bytes.NewReader(result.PNG()))
		if err != nil {
			t.Fatal(err)
		}
		if decoded.Bounds().Size() != result.Size() {
			t.Fatal("PNG dimensions changed")
		}
		visible := 0
		for y := decoded.Bounds().Min.Y; y < decoded.Bounds().Max.Y; y++ {
			for x := decoded.Bounds().Min.X; x < decoded.Bounds().Max.X; x++ {
				_, _, _, alpha := decoded.At(x, y).RGBA()
				if alpha != 0 {
					visible++
				}
			}
		}
		if visible < 100 {
			t.Fatalf("diagram has only %d visible pixels", visible)
		}
	}
	if _, err := renderer.Render(t.Context(), "not a Mermaid diagram"); err == nil {
		t.Fatal("syntax error hidden")
	}
	t.Run("cancel after browser request", func(t *testing.T) {
		requested := make(chan struct{}, 1)
		server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
			select {
			case requested <- struct{}{}:
			default:
			}
			<-request.Context().Done()
		}))
		t.Cleanup(server.Close)
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		source := fmt.Sprintf("flowchart TD\n A@{ img: %q, label: \"Waiting\" }", server.URL+"/diagram.png")
		call := startRender(ctx, t, renderer, source)
		select {
		case <-requested:
		case <-call.done:
			t.Fatalf("render ended before browser request: %v", call.err)
		case <-ctx.Done():
			t.Fatal(context.Cause(ctx))
		}
		call.cancel()
		<-call.done
		if !errors.Is(call.err, context.Canceled) {
			t.Fatalf("cancel error=%v", call.err)
		}
	})
	t.Run("browser disappeared", func(t *testing.T) {
		browser := filepath.Join(t.TempDir(), "missing-browser.exe")
		//nolint:gosec // G306: executable locator in a private fixture directory.
		if err := os.WriteFile(browser, nil, 0o700); err != nil {
			t.Fatal(err)
		}
		missing, err := mermaid.New(mermaid.Config{Executable: executable, Browser: browser})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(browser); err != nil {
			t.Fatal(err)
		}
		if _, err := missing.Render(t.Context(), "flowchart LR; A-->B"); err == nil || !strings.Contains(err.Error(), "ENOENT") {
			t.Fatalf("browser launch error was lost: %v", err)
		}
	})
}

func TestInstalledBackendPreservesThemePixels(t *testing.T) {
	seen := make(map[[32]byte]string)
	for _, theme := range []string{"default", "dark", "forest", "neutral", "base"} {
		t.Run(theme, func(t *testing.T) {
			pixels := renderInstalled(t, mermaid.Config{Theme: theme}, "flowchart LR; A[Owner] --> B[Worker]")
			digest := sha256.Sum256(pixels.Pix)
			if previous, exists := seen[digest]; exists {
				t.Fatalf("themes %q and %q rendered identical pixels", previous, theme)
			}
			seen[digest] = theme
		})
	}
}

func TestInstalledBackendPreservesFillAndTextColors(t *testing.T) {
	pixels := renderInstalled(t, mermaid.Config{}, "flowchart LR\n A[VISIBLE]\n classDef colored fill:#ff0000,color:#00ff00\n class A colored")
	var fill, text int
	for y := range pixels.Bounds().Dy() {
		for x := range pixels.Bounds().Dx() {
			switch pixels.NRGBAAt(x, y) {
			case (color.NRGBA{R: 255, A: 255}):
				fill++
			case (color.NRGBA{G: 255, A: 255}):
				text++
			}
		}
	}
	if fill < 100 || text < 10 {
		t.Fatalf("authored colors lost: fill pixels=%d, text pixels=%d", fill, text)
	}
}

func renderInstalled(t *testing.T, config mermaid.Config, source string) *image.NRGBA {
	t.Helper()
	config.Executable = os.Getenv("OOLONG_MERMAID_CLI")
	if config.Executable == "" {
		t.Skip("set OOLONG_MERMAID_CLI for installed backend integration")
	}
	config.Browser = os.Getenv("OOLONG_MERMAID_BROWSER")
	renderer, err := mermaid.New(config)
	if err != nil {
		t.Fatal(err)
	}
	result, err := renderer.Render(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(result.PNG()))
	if err != nil {
		t.Fatal(err)
	}
	pixels := image.NewNRGBA(image.Rectangle{Max: decoded.Bounds().Size()})
	draw.Draw(pixels, pixels.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	return pixels
}
