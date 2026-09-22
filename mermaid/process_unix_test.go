//go:build unix

package mermaid_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/Tangerg/oolong/mermaid"
)

func TestUnresponsiveRendererCannotLeaveTheBrowserRunning(t *testing.T) {
	cfg := backendConfig(t, 0)
	renderer, err := mermaid.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), cfg.Timeout)
	defer cancel()
	endpoint, ready := readinessServer(t)
	call := startRender(ctx, t, renderer, "spin:"+endpoint)
	data, err := waitReady(ctx, ready, call)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatal(err)
	}
	call.cancel()
	<-call.done
	if !errors.Is(call.err, context.Canceled) || strings.Contains(call.err.Error(), "did not finish") {
		t.Fatalf("shutdown=%v", call.err)
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("browser %d still exists: %v", pid, err)
	}
}
