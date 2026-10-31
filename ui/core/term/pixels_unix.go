//go:build unix

package term

import (
	"image"

	"golang.org/x/sys/unix"
)

// Derive pixel and cell dimensions from one kernel window measurement: two
// independent reads could combine the pixels before a resize with the rows after.
func cellSize(fd int) (image.Point, bool) {
	size, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil || size.Col == 0 || size.Row == 0 {
		return image.Point{}, false
	}
	cell := image.Pt(int(size.Xpixel/size.Col), int(size.Ypixel/size.Row))
	if cell.X == 0 || cell.Y == 0 {
		return image.Point{}, false
	}
	return cell, true
}
