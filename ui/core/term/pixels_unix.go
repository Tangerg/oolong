//go:build unix

package term

import "golang.org/x/sys/unix"

// windowPixels is the terminal window's size in pixels, and whether it said.
//
// It comes from the same place the size in cells does, which is the only way to
// learn it without asking the terminal a question and waiting for the answer: the
// window size the operating system keeps has two more fields in it, and a terminal
// that fills them in has told the truth about itself for free.
//
// Plenty of terminals leave them zero. That is what the false is for, and it is why
// nothing here guesses a cell size — a picture scaled by an invented number is a
// picture the wrong shape, which is worse than one that was not shown.
func windowPixels(fd int) (w, h int, ok bool) {
	size, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil || size.Xpixel == 0 || size.Ypixel == 0 {
		return 0, 0, false
	}
	return int(size.Xpixel), int(size.Ypixel), true
}
