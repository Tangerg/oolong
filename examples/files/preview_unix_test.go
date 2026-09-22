//go:build unix

package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Tangerg/oolong/components/kit"
)

func TestPreviewRejectsFIFOWithoutWaitingForAWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan string, 1)
	go func() { done <- preview(entry{path: path}, kit.Theme{})[0].String() }()
	select {
	case message := <-done:
		if !strings.Contains(message, "regular file") {
			t.Fatal(message)
		}
	case <-time.After(time.Second):
		t.Fatal("preview blocked on FIFO")
	}
}

func TestPreviewLimitsASingleLongLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large")
	if err := os.WriteFile(path, []byte(strings.Repeat("a", 1<<20)), 0o600); err != nil {
		t.Fatal(err)
	}
	lines := preview(entry{path: path}, kit.Theme{})
	if len(lines) != 1 || len(lines[0].String()) != 64<<10 {
		t.Fatal("preview exceeded byte budget")
	}
}
