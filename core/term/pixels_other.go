//go:build !unix

package term

// windowPixels reports that nothing said, which is what a platform whose window
// size is only ever counted in cells can honestly answer.
func windowPixels(int) (w, h int, ok bool) { return 0, 0, false }
