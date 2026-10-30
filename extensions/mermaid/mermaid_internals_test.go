package mermaid

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestOutputValidationAndOwnership(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "diagram.png")
	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, limits := range []struct{ bytes, pixels int64 }{{1, 100}, {10000, 11}} {
		if _, err := readImage(t.Context(), path, limits.bytes, limits.pixels); !errors.Is(err, ErrLimit) {
			t.Fatal(err)
		}
	}
	result, err := readImage(t.Context(), path, 10000, 12)
	if err != nil {
		t.Fatal(err)
	}
	first := result.PNG()
	first[0] = 0
	if result.PNG()[0] == 0 || result.Size() != image.Pt(4, 3) {
		t.Fatal("image does not own its PNG")
	}
	if err := os.WriteFile(path, []byte("not png"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readImage(t.Context(), path, 10000, 100); err == nil {
		t.Fatal("accepted invalid PNG")
	}
}

func TestDiagnosticsBoundMemoryWithoutBreakingWriterContract(t *testing.T) {
	output := &boundedOutput{limit: 3}
	if n, err := output.Write([]byte("abcdef")); n != 6 || err != nil {
		t.Fatalf("n=%d error=%v", n, err)
	}
	if n, err := output.Write([]byte("more")); n != 4 || err != nil || output.Len() != 3 {
		t.Fatalf("n=%d error=%v output=%q", n, err, output.String())
	}
}

func TestZeroRendererRejectsUse(t *testing.T) {
	var renderer Renderer
	if result, err := renderer.Render(t.Context(), "graph TD"); result != nil || err == nil {
		t.Fatalf("result=%v error=%v", result, err)
	}
}

func TestRenderOwnsBrowserProfileAndSecurityConfiguration(t *testing.T) {
	renderer := &Renderer{cfg: Config{Theme: "dark", MaxSourceBytes: 1024, MaxEdges: 10, Width: 100, Height: 100}}
	directory := t.TempDir()
	_, output, err := renderer.arguments(directory)
	if err != nil {
		t.Fatal(err)
	}
	if output != filepath.Join(directory, "diagram.png") {
		t.Fatal(output)
	}
	data, err := os.ReadFile(filepath.Join(directory, "config.json")) //nolint:gosec // G304: fixed name in a test-owned directory.
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Profile string `json:"profile"`
		Mermaid struct {
			SecurityLevel string `json:"securityLevel"`
			MaxTextSize   int    `json:"maxTextSize"`
			MaxEdges      int    `json:"maxEdges"`
		} `json:"mermaid"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if config.Profile != filepath.Join(directory, "browser") || config.Mermaid.SecurityLevel != "strict" || config.Mermaid.MaxTextSize != 1024 || config.Mermaid.MaxEdges != 10 {
		t.Fatalf("config=%+v", config)
	}
}

func TestImageValidationDoesNotAcceptCancelledWork(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 4, 3))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := readImage(ctx, path, 10000, 100)
	if result != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("result=%v err=%v", result, err)
	}
}
