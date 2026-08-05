//go:build unix

package term

import (
	"fmt"
	"os"
	"os/signal"

	"golang.org/x/sys/unix"
)

// Suspend stops this process the way Ctrl+Z does, and returns when it is continued.
//
// It does nothing to the terminal, which is what makes it a separate thing: a
// program that stopped while holding the alternate screen leaves the user looking
// at half an interface they cannot type into, so the terminal has to be given back
// first. [Terminal.Suspend] is the two together and is what a session wants.
//
// The stop signal is sent to this process and not handled, so the default action
// takes it: the shell that started this program takes the terminal back, prints its
// prompt, and this call has not returned. Continuing is what returns from it, which
// is why the continue signal is waited for rather than assumed — the kill returns
// as soon as the signal is queued, and going on from there would resume an
// interface while the process was still stopped.
func Suspend() error {
	cont := make(chan os.Signal, 1)
	signal.Notify(cont, unix.SIGCONT)
	defer signal.Stop(cont)

	if err := unix.Kill(unix.Getpid(), unix.SIGTSTP); err != nil {
		return fmt.Errorf("term: suspend: %w", err)
	}
	<-cont
	return nil
}
