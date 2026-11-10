package ssh_test

import (
	"errors"
	"testing"

	charmssh "charm.land/ssh"
	"github.com/Tangerg/oolong/extensions/ssh"
	"github.com/Tangerg/oolong/ui/core/grid"
	"github.com/Tangerg/oolong/ui/core/term"
)

func TestInvalidCursorRestoreDoesNotConsumeTerminalOpen(t *testing.T) {
	windows := make(chan charmssh.Window)
	close(windows)
	session := &fakeSession{
		ctx: newFakeContext(t.Context()), window: charmssh.Window{Width: 80, Height: 24},
		windows: windows, ptyOK: true,
	}
	var adapter ssh.Adapter
	server := new(charmssh.Server)
	if err := adapter.Install(server); err != nil {
		t.Fatal(err)
	}
	request, _ := session.requestedPTY()
	release, err := server.PtyHandler(session.ctx, session, request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if releaseErr := release(); releaseErr != nil {
			t.Error(releaseErr)
		}
	})
	terminal, err := adapter.Bind(session)
	if err != nil {
		t.Fatal(err)
	}
	invalid := term.Config{RestoreCursor: grid.CursorStyle{Shape: grid.CursorBar + 1}}
	if host, openErr := terminal.Open(invalid); !errors.Is(openErr, grid.ErrInvalidCursorShape) || host != nil {
		t.Fatalf("invalid cursor restore returned host=%v, err=%v", host, openErr)
	}
	if got := session.output.String(); got != "" {
		t.Fatalf("refused configuration wrote terminal modes: %q", got)
	}
	valid := term.Config{RestoreCursor: grid.CursorStyle{Shape: grid.CursorBar}}
	host, err := terminal.Open(valid)
	if err != nil {
		t.Fatalf("refused configuration consumed the open lifetime: %v", err)
	}
	if err = host.Close(); err != nil {
		t.Fatal(err)
	}
	modes, err := valid.Modes(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := session.output.String(), modes.Enter()+modes.Leave(); got != want {
		t.Fatalf("configured restore output = %q, want %q", got, want)
	}
}
