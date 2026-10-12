//go:build !unix

package term

func windowPixels(int) (w, h int, ok bool) { return 0, 0, false }
