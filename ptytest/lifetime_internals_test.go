package ptytest

import (
	"testing"
	"time"
)

// TestThePrimaryCanBeUnblocked states the property Close depends on.
//
// A read on a pty ends when the far end goes away, and the far end is whatever
// holds the replica open — which is not always the child. If the descriptor is not
// in the runtime's poller, nothing can end that read, and closing the file waits
// for it instead of stopping it. A deadline is the cheapest observation of the same
// capability: only a pollable descriptor accepts one.
func TestThePrimaryCanBeUnblocked(t *testing.T) {
	if !Supported() {
		t.Skip("no pty on this platform")
	}
	primary, replica, err := openPTY()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = replica.Close()
		_ = primary.Close()
	}()
	if err := primary.SetReadDeadline(time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("the pty primary cannot be unblocked: %v", err)
	}
	if err := primary.SetReadDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
}

// TestSizingKeepsThePrimaryUnblockable guards the way setSize reaches the
// descriptor. Taking it with Fd would hand the file back to blocking mode, which
// costs nothing visible until a Close somewhere has to wait for ever.
func TestSizingKeepsThePrimaryUnblockable(t *testing.T) {
	if !Supported() {
		t.Skip("no pty on this platform")
	}
	primary, replica, err := openPTY()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = replica.Close()
		_ = primary.Close()
	}()
	if err := setSize(primary, Size{Cols: 80, Rows: 24}); err != nil {
		t.Fatal(err)
	}
	if err := primary.SetReadDeadline(time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("sizing the pty made its primary unblockable: %v", err)
	}
}
