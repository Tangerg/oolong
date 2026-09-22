//go:build unix || windows

package mermaid

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

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
