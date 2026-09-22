//go:build unix

package mermaid

import (
	"errors"
	"path/filepath"
	"syscall"
	"testing"
)

func TestSpecialOutputIsRejectedBeforeOpening(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "diagram.png")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readImage(t.Context(), fifo, 1024, 100); !errors.Is(err, ErrLimit) {
		t.Fatalf("FIFO: %v", err)
	}
	if _, err := readImage(t.Context(), t.TempDir(), 1024, 100); !errors.Is(err, ErrLimit) {
		t.Fatalf("directory: %v", err)
	}
}
