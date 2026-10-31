//go:build !unix

package term

import "image"

func cellSize(int) (image.Point, bool) { return image.Point{}, false }
