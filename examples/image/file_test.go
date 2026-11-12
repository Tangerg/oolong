package main

import (
	"bytes"
	"context"
	"errors"
	"io"
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

func TestPNGFileAndReaderPreserveCancellationCause(t *testing.T) {
	cause := errors.New("image request ended")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	if _, err := readPNG(ctx, filepath.Join(t.TempDir(), "absent")); !errors.Is(err, cause) {
		t.Errorf("cancelled file admission = %v", err)
	}
	source := bytes.NewReader([]byte("unread"))
	reader := contextReader{context: ctx, reader: source}
	if n, err := reader.Read(make([]byte, 8)); n != 0 || !errors.Is(err, cause) || source.Len() != len("unread") {
		t.Fatalf("cancelled file read = %d, %v; remaining=%d", n, err, source.Len())
	}
}

type fileReadFunc func([]byte) (int, error)

func (f fileReadFunc) Read(p []byte) (int, error) { return f(p) }

func TestFileReadPreservesAnIndependentFailureDuringCancellation(t *testing.T) {
	cause := errors.New("image request ended")
	readFailure := errors.New("filesystem read failed")
	for _, test := range []struct {
		readErr     error
		independent bool
	}{
		{readErr: nil},
		{readErr: io.EOF},
		{readErr: readFailure, independent: true},
		{readErr: errors.Join(io.EOF, readFailure), independent: true},
	} {
		ctx, cancel := context.WithCancelCause(t.Context())
		reader := contextReader{context: ctx, reader: fileReadFunc(func(p []byte) (int, error) {
			cancel(cause)
			return copy(p, "part"), test.readErr
		})}
		buffer := make([]byte, 8)
		n, err := reader.Read(buffer)
		if n != len("part") || !errors.Is(err, cause) {
			t.Errorf("read result %v lost cancellation: %d, %v", test.readErr, n, err)
		}
		if test.independent && !errors.Is(err, test.readErr) {
			t.Errorf("read result %v was hidden by cancellation: %v", test.readErr, err)
		}
		if !test.independent && err != cause { //nolint:errorlint // Task outcomes distinguish bare cancellation from wrapped diagnostics.
			t.Errorf("pure cancellation was wrapped as another failure: %v", err)
		}
	}
}
