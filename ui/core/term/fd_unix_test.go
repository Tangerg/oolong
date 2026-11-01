//go:build unix

package term_test

import (
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/Tangerg/oolong/ui/core/term"
)

func TestRejectingANonterminalPreservesItsFileStatusFlags(t *testing.T) {
	in, out, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = in.Close(); _ = out.Close() }()
	raw, err := in.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	flags := func() int {
		t.Helper()
		var result int
		var cause error
		err := raw.Control(func(fd uintptr) { result, cause = unix.FcntlInt(fd, unix.F_GETFL, 0) })
		if err != nil || cause != nil {
			t.Fatal(errors.Join(err, cause))
		}
		return result
	}
	before := flags()
	if _, err := term.OpenOn(in, out, term.Config{}, nil); !errors.Is(err, term.ErrNotTerminal) {
		t.Fatalf("OpenOn = %v", err)
	}
	if after := flags(); before != after {
		t.Fatalf("file status flags changed from %#x to %#x", before, after)
	}
}
