package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPNGFileAdmission(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "image.png")
	data := []byte("selected file bytes")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := readPNG(t.Context(), path); err != nil || !bytes.Equal(got, data) {
		t.Fatalf("read = %q, %v", got, err)
	}
	if _, err := readPNG(t.Context(), directory); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("directory admitted: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := readPNG(ctx, filepath.Join(directory, "absent")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request reached file admission: %v", err)
	}
	if err := os.Truncate(path, maxPNGFileBytes+1); err != nil {
		t.Fatal(err)
	}
	if _, err := readPNG(t.Context(), path); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized file admitted: %v", err)
	}
}

func TestFileReadChecksCancellationBetweenReads(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	source := bytes.NewReader([]byte("firstsecond"))
	reader := contextReader{context: ctx, reader: source}
	buffer := make([]byte, len("first"))
	if n, err := reader.Read(buffer); err != nil || string(buffer[:n]) != "first" {
		t.Fatalf("first read = %q, %v", buffer[:n], err)
	}
	cancel()
	if n, err := reader.Read(buffer); n != 0 || !errors.Is(err, context.Canceled) || source.Len() != len("second") {
		t.Fatalf("read after cancellation = %d, %v; source has %d bytes left", n, err, source.Len())
	}
}
