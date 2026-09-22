//go:build unix || windows

package mermaid

import (
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestBackendProcess(_ *testing.T) {
	index := slices.Index(os.Args, "--mermaid-test-backend")
	if index < 0 {
		return
	}
	args := os.Args[index+1:]
	if len(args) == 0 {
		os.Exit(2)
	}
	mode := args[0]
	switch mode {
	case "wait":
		time.Sleep(time.Minute)
	case "fail":
		_, _ = io.WriteString(os.Stderr, "intentional backend diagnostic")
		os.Exit(3)
	default:
		output := slices.Index(args, "--output")
		if output < 0 || output+1 >= len(args) {
			os.Exit(4)
		}
		file, err := os.Create(args[output+1]) //nolint:gosec // G703: test subprocess receives only its parent-owned output path.
		if err != nil {
			os.Exit(5)
		}
		if err := png.Encode(file, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
			os.Exit(6)
		}
		if err := file.Close(); err != nil {
			os.Exit(7)
		}
	}
	os.Exit(0)
}

func testRenderer(t *testing.T, mode string) *Renderer {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := New(Config{Executable: executable, Arguments: []string{"-test.run=^TestBackendProcess$", "--", "--mermaid-test-backend", mode}, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return renderer
}

func TestBackendSuccessFailureAndCancellation(t *testing.T) {
	result, err := testRenderer(t, "png").Render(t.Context(), "flowchart TD\n A-->B")
	if err != nil || result.Size() != image.Pt(3, 2) {
		t.Fatalf("result=%v error=%v", result, err)
	}
	if _, err := testRenderer(t, "fail").Render(t.Context(), "invalid"); err == nil || !strings.Contains(err.Error(), "intentional backend diagnostic") {
		t.Fatal(err)
	}
	started := time.Now()
	waiting := testRenderer(t, "wait")
	waiting.cfg.Timeout = 150 * time.Millisecond
	if _, err := waiting.Render(t.Context(), "graph TD"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("cancellation did not terminate backend")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := testRenderer(t, "png").Render(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestInputLimitsAndQueuedCancellation(t *testing.T) {
	renderer := testRenderer(t, "png")
	renderer.cfg.MaxSourceBytes = 3
	if _, err := renderer.Render(t.Context(), "long"); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	if _, err := renderer.Render(t.Context(), string([]byte{255})); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	renderer.slot <- struct{}{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := renderer.Render(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	<-renderer.slot
}

func TestOfficialCLI(t *testing.T) {
	executable := os.Getenv("OOLONG_MERMAID_CLI")
	if executable == "" {
		t.Skip("set OOLONG_MERMAID_CLI for installed official backend integration")
	}
	renderer, err := New(Config{Executable: executable, Browser: os.Getenv("OOLONG_MERMAID_BROWSER")})
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"flowchart TD\n A[Owner] --> B[Worker]\n B --> C[PNG]", "sequenceDiagram\n Alice->>Bob: Hello"} {
		result, err := renderer.Render(t.Context(), source)
		if err != nil {
			t.Fatal(err)
		}
		if result.Size().X <= 0 || result.Size().Y <= 0 {
			t.Fatal("empty diagram")
		}
	}
	if _, err := renderer.Render(t.Context(), "not a Mermaid diagram"); err == nil {
		t.Fatal("syntax error hidden")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	if _, err := renderer.Render(ctx, "flowchart TD\n A-->B"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancel error=%v", err)
	}
}
