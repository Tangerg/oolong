//go:build unix

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPNGFileAdmissionNeverWaitsForAFIFOWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, cancelled := range []bool{true, false} {
		ctx, cancel := context.WithCancel(t.Context())
		if cancelled {
			cancel()
		}
		finished := make(chan error, 1)
		go func() {
			_, err := readPNG(ctx, path)
			finished <- err
		}()
		select {
		case err := <-finished:
			if cancelled && !errors.Is(err, context.Canceled) {
				t.Errorf("cancelled FIFO read = %v", err)
			} else if !cancelled && (err == nil || !strings.Contains(err.Error(), "regular file")) {
				t.Errorf("FIFO read = %v", err)
			}
		case <-time.After(time.Second):
			cancel()
			t.Fatal("file admission waited for a FIFO writer")
		}
		cancel()
	}
}

func TestPNGFileAdmissionRejectsASymlink(t *testing.T) {
	directory := t.TempDir()
	path, link := filepath.Join(directory, "image.png"), filepath.Join(directory, "link.png")
	if err := os.WriteFile(path, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readPNG(t.Context(), link); err == nil {
		t.Fatal("symlink was admitted")
	}
}
