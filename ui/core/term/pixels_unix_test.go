//go:build unix

package term_test

import (
	"image"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/Tangerg/oolong/ui/core/term"
)

func TestCellSizeRequiresTwoPositivePixelDimensions(t *testing.T) {
	_, replica := pty(t)
	fd := int(replica.Fd())
	tty, err := term.OpenOn(replica, replica, term.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tty.Close() })
	for _, tc := range []struct {
		width uint16
		want  image.Point
		known bool
	}{{width: 1}, {width: 800, want: image.Pt(10, 20), known: true}} {
		size := &unix.Winsize{Row: 24, Col: 80, Xpixel: tc.width, Ypixel: 480}
		if err := unix.IoctlSetWinsize(fd, unix.TIOCSWINSZ, size); err != nil {
			t.Fatal(err)
		}
		if got, known := tty.CellSize(); got != tc.want || known != tc.known {
			t.Fatalf("terminal reports %+v: cell size %v, %v; want %v, %v", size, got, known, tc.want, tc.known)
		}
	}
}
