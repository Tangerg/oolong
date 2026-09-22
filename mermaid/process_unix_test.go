//go:build unix

package mermaid_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Tangerg/oolong/mermaid"
)

func TestUnresponsiveRendererCannotLeaveTheBrowserRunning(t *testing.T) {
	cfg := backendConfig(t)
	cfg.Timeout = 5 * time.Second
	renderer, err := mermaid.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ready := filepath.Join(t.TempDir(), "browser.pid")
	done := make(chan error, 1)
	go func() {
		_, renderErr := renderer.Render(ctx, "spin:"+ready)
		done <- renderErr
	}()
	var pid int
	deadline := time.Now().Add(3 * time.Second)
	for pid == 0 {
		//nolint:gosec // G304: fixture-owned readiness file.
		if data, readErr := os.ReadFile(ready); readErr == nil && len(data) > 0 {
			pid, err = strconv.Atoi(string(data))
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("backend not ready")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	err = <-done
	if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "did not finish") {
		t.Fatalf("shutdown=%v", err)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("browser %d still exists: %v", pid, err)
	}
}
